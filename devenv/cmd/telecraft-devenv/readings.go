package main

import (
	"context"
	"sort"
	"time"

	"github.com/telecraft-dev/telecraft/internal/console"
	seam "github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// estateReader is the half of the EstateProvider seam this tool uses. It is
// named here rather than taken as seam.Provider so the composer can be
// tested against a fabricated reading with no server in the way.
type estateReader interface {
	Estate(context.Context) seam.Estate
}

// deliveryReader answers how one collector came by its configuration
// (REQ-041). It is a second reader rather than part of the estate one
// because the EstateProvider seam carries what a collector reports about
// its running state, never where that state came from. See delivery.go.
type deliveryReader interface {
	Path(identity map[string]string) string
}

// row is one (Service, Environment) the arrivals are read for. The pairs
// come from the authored conformance estate, which is the devenv's one
// declared reading (ADR-0052 §2).
type row struct {
	Service     string
	Environment string
}

// composer turns the two live seams into the readings file the snapshot
// builder loads. It decides nothing: every field it writes is something a
// seam returned, and the one thing it adds (how long a signal has been
// silent) is bookkeeping the production path gets from a Damper and a
// single-instant reading cannot have.
type composer struct {
	Collectors estateReader
	Telemetry  telemetry.Provider

	// Delivery is how each collector came by its configuration. It is
	// required: REQ-041's reading has two values and no third one for not
	// having looked, so a composer without it would be back to asserting
	// one path for every collector.
	Delivery deliveryReader

	// Rows and Tiers are what to read for, taken from the estate.
	Rows  []row
	Tiers []string

	// Attributes are the attribute names to measure coverage for: the
	// union of what the requirements library asks about, so a coverage
	// assertion is judged against a measurement rather than a blank.
	Attributes []string

	Window time.Duration
	Now    func() time.Time

	// silentSince remembers when each signal stopped arriving. A snapshot
	// is one instant, and no judgement raises a finding from one instant
	// (ADR-0035 §3), so a reading that could not say how long a gap had
	// lasted would leave every gap permanently dampened and no scenario
	// would ever become visible.
	silentSince map[silenceKey]time.Time

	// shortfallSince is the same clock for populations, and it is fed from
	// the last snapshot rather than from a count taken here: which
	// collector matches which Tier is the serving matcher's judgement, and
	// a second implementation of it in a development tool would be a
	// second thing to be wrong. One refresh of lag, at ten seconds a
	// refresh, against a grace window measured in minutes.
	shortfallSince map[string]time.Time
}

type silenceKey struct {
	service     string
	environment string
	signal      string
}

// compose takes one reading of everything.
func (c *composer) compose(ctx context.Context) console.Readings {
	now := c.Now()
	out := console.Readings{
		AsOf:   now,
		Window: requirements.Duration(c.Window),
	}

	est := c.Collectors.Estate(ctx)
	for _, col := range est.Collectors {
		out.Collectors = append(out.Collectors, collectorReading(col, est.AsOf, c.Delivery.Path(col.Identity)))
	}

	for _, r := range c.Rows {
		observed := c.Telemetry.Observe(ctx,
			telemetry.Service{Name: r.Service, Environment: r.Environment},
			c.Window, c.Attributes)
		out.Rows = append(out.Rows, console.RowReading{
			Service:     r.Service,
			Environment: r.Environment,
			Signals:     c.signalsOf(r, observed, now),
		})
	}

	for _, tier := range c.Tiers {
		self := c.Telemetry.ObserveSelf(ctx, tier, c.Window)
		reading := tierReading(tier, self)
		if since, ok := c.shortfallSince[tier]; ok {
			reading.ShortfallSince = since
		}
		out.Tiers = append(out.Tiers, reading)
	}

	return out
}

// observePopulations feeds the shortfall clock from the snapshot the last
// refresh produced. A Tier below its floor starts a clock; one back at or
// above it stops one.
//
// The judgement this feeds refuses to raise a toothed finding from a single
// instant (ADR-0035 §3), so without this every Tier would read ok however
// long it had been short, and `devenv scenario shrink` would show nothing.
func (c *composer) observePopulations(cards []console.CardFace, now time.Time) {
	if c.shortfallSince == nil {
		c.shortfallSince = map[string]time.Time{}
	}
	for _, card := range cards {
		if card.Population.Floor == nil || card.Population.Matched >= *card.Population.Floor {
			delete(c.shortfallSince, card.Tier)
			continue
		}
		if _, ok := c.shortfallSince[card.Tier]; !ok {
			c.shortfallSince[card.Tier] = now
		}
	}
}

// collectorReading projects one collector's seam reading.
//
// Every collector in this reading is on a live connection to the platform's
// own server, so its state is reporting. Its delivery path is not decided
// here: it is read off the same wire, from whether the collector declares
// it accepts remote config (delivery.go), because a collector reporting
// through its own opamp extension reports exactly like a served one and
// takes nothing the server sends.
func collectorReading(col seam.Collector, asOf time.Time, delivery string) console.CollectorReading {
	lastSeen := col.Effective.AsOf
	if lastSeen.IsZero() {
		lastSeen = asOf
	}
	return console.CollectorReading{
		ID:         collectorID(col.Identity),
		Attributes: col.Identity,
		State:      "reporting",
		Version:    col.Identity["service.version"],
		LastSeen:   lastSeen,
		Delivery:   delivery,
	}
}

// collectorID picks the readable name for a collector. It is presentation
// only: identity for matching is the attribute set (ADR-0013), and the
// fallback is the whole set rather than a truncation of it.
func collectorID(identity map[string]string) string {
	if id := identity["service.instance.id"]; id != "" {
		return id
	}
	return seam.Fingerprint(identity)
}

// signalsOf projects one row's arrival reading, one entry per signal the
// backend was asked about.
func (c *composer) signalsOf(r row, observed telemetry.Observed, now time.Time) map[string]console.SignalReading {
	out := make(map[string]console.SignalReading, len(observed.Signals))
	for kind, obs := range observed.Signals {
		known := obs.Known
		reading := console.SignalReading{
			Known:             &known,
			Cause:             obs.Cause,
			Present:           obs.Present,
			Volume:            obs.Volume,
			AttributeCoverage: obs.AttributeCoverage,
		}
		if since, ok := c.trackSilence(r, string(kind), obs, now); ok {
			reading.Since = since
		}
		out[string(kind)] = reading
	}
	return out
}

// trackSilence maintains the silence clock for one signal and reports when
// the current silence began. A signal that is arriving, or one the backend
// cannot see at all, has no silence to report: an unknown reading is not an
// observed gap (ADR-0008).
func (c *composer) trackSilence(r row, signal string, obs telemetry.SignalObservation, now time.Time) (time.Time, bool) {
	key := silenceKey{service: r.Service, environment: r.Environment, signal: signal}
	if c.silentSince == nil {
		c.silentSince = map[silenceKey]time.Time{}
	}
	if !obs.Known || obs.Present {
		delete(c.silentSince, key)
		return time.Time{}, false
	}
	if since, ok := c.silentSince[key]; ok {
		return since, true
	}
	c.silentSince[key] = now
	return now, true
}

// tierReading projects one Tier's self-telemetry reading. The component
// identities are carried verbatim, which is what lets the expectation
// engine judge claims against what the backend actually recorded rather
// than against a shape this tool chose (ADR-0039 §5).
func tierReading(tier string, self telemetry.SelfObserved) console.TierReading {
	out := console.TierReading{
		Tier:    tier,
		Signals: make(map[string]console.SignalReading, len(self.Signals)),
	}
	for kind, sig := range self.Signals {
		known := sig.Known
		out.Signals[string(kind)] = console.SignalReading{
			Known:   &known,
			Cause:   sig.Cause,
			Present: sig.Present,
			Volume:  sig.Volume,
		}
		for _, comp := range sig.Components {
			if len(comp.Attributes) > 0 {
				out.Components = append(out.Components, comp.Attributes)
			}
		}
	}
	sortComponents(out.Components)
	return out
}

// sortComponents orders the verbatim component identities so a reading
// taken twice over an unchanged estate is byte-identical, and a diff of two
// readings shows what changed rather than what a map iteration reordered.
func sortComponents(comps []map[string]string) {
	sort.Slice(comps, func(i, j int) bool {
		return seam.Fingerprint(comps[i]) < seam.Fingerprint(comps[j])
	})
}

// attributeNames is the union of every attribute name the library asks
// about coverage for, sorted. Asking for all of them on every row costs one
// aggregation each and keeps the reading independent of which requirement
// happens to apply to which Service.
func attributeNames(lib requirements.Library) []string {
	seen := map[string]bool{}
	for _, req := range lib.Sorted() {
		if req.Signal == nil {
			continue
		}
		for _, name := range req.Signal.RequiredAttributes {
			seen[name] = true
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
