// Package readings takes one reading of the two live seams and composes
// the readings document the console snapshot builder loads: the collector
// estate through the EstateProvider seam (ADR-0008) and the telemetry
// arrivals through the TelemetryProvider one.
//
// It decides nothing. Every field it writes is something a seam returned,
// and the one thing it adds (how long a signal has been silent, how long a
// Tier has been short) is bookkeeping a single-instant reading cannot have
// and every judgement over it needs (ADR-0035 §3).
//
// The Instance server composes here on every refresh (ADR-0067), and the
// development environment writes the result to a file beside its snapshot
// (ADR-0052). One composition, so the two cannot read the seams
// differently.
package readings

import (
	"context"
	"sort"
	"time"

	"github.com/telecraft-dev/telecraft/internal/console"
	seam "github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// EstateReader is the half of the EstateProvider seam a composition uses.
// It is named here rather than taken as seam.Provider so the Composer can
// be tested against a fabricated reading with no server in the way.
type EstateReader interface {
	Estate(context.Context) seam.Estate
}

// DeliveryReader answers how one collector came by its configuration
// (REQ-041). It is a second reader rather than part of the estate one
// because the EstateProvider seam carries what a collector reports about
// its running state, never where that state came from. See delivery.go.
type DeliveryReader interface {
	Path(identity map[string]string) string
}

// Row is one (Service, Environment) the arrivals are read for. The pairs
// come from the authored conformance estate.
type Row struct {
	Service     string
	Environment string
}

// Composer turns the two live seams into one readings document. A Composer
// is reused across refreshes: the silence and shortfall clocks it keeps are
// what let a judgement date a gap, and they die with the process like every
// other in-memory holding (ADR-0032).
type Composer struct {
	Collectors EstateReader

	// Telemetry is the arrivals seam. Nil takes no arrival reading at all,
	// which is the shape of an Instance with no telemetry backend
	// configured yet: the rows and Tiers come back undeclared, and the
	// console renders them as not known with the cause said out loud
	// rather than as nothing arriving (ADR-0008).
	Telemetry telemetry.Provider

	// Delivery is how each collector came by its configuration. It is
	// required: REQ-041's reading has two values and no third one for not
	// having looked, so a Composer without it would be back to asserting
	// one path for every collector.
	Delivery DeliveryReader

	// Rows and Tiers are what to read for, taken from the estate.
	Rows  []Row
	Tiers []string

	// Attributes are the attribute names to measure coverage for: the
	// union of what the requirements library asks about, so a coverage
	// assertion is judged against a measurement rather than a blank.
	Attributes []string

	// SchemaSignals are the signals a schema-conformance requirement in
	// the library is judged on, and so the signals the attribute-name
	// reading is taken for (ADR-0034 §4). Empty takes none: a library that
	// references no Schema Registry has nothing to judge the names
	// against, and an aggregation per signal per Row would buy a reading
	// nobody reads.
	SchemaSignals []requirements.SignalKind

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

// Compose takes one reading of everything.
func (c *Composer) Compose(ctx context.Context) console.Readings {
	now := c.Now()
	out := console.Readings{
		AsOf:   now,
		Window: requirements.Duration(c.Window),
	}

	est := c.Collectors.Estate(ctx)
	for _, col := range est.Collectors {
		out.Collectors = append(out.Collectors, collectorReading(col, est.AsOf, c.Delivery.Path(col.Identity)))
	}

	if c.Telemetry == nil {
		return out
	}

	for _, r := range c.Rows {
		svc := telemetry.Service{Name: r.Service, Environment: r.Environment}
		observed := c.Telemetry.Observe(ctx, svc, c.Window, c.Attributes)
		names := make(map[requirements.SignalKind]telemetry.AttributeNames, len(c.SchemaSignals))
		for _, kind := range c.SchemaSignals {
			names[kind] = c.Telemetry.AttributeNames(ctx, svc, kind, c.Window)
		}
		out.Rows = append(out.Rows, console.RowReading{
			Service:     r.Service,
			Environment: r.Environment,
			Signals:     c.signalsOf(r, observed, names, now),
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

// ObservePopulations feeds the shortfall clock from the documents the last
// refresh produced. A Tier below its floor starts a clock; one back at or
// above it stops one.
//
// The judgement this feeds refuses to raise a toothed finding from a single
// instant (ADR-0035 §3), so without this every Tier would read ok however
// long it had been short, and no shortfall would ever become visible.
func (c *Composer) ObservePopulations(cards []console.CardFace, now time.Time) {
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
		ID:         CollectorID(col.Identity),
		Attributes: col.Identity,
		State:      "reporting",
		Version:    col.Identity["service.version"],
		LastSeen:   lastSeen,
		Delivery:   delivery,
	}
}

// CollectorID picks the readable name for a collector. It is presentation
// only: identity for matching is the attribute set (ADR-0013), and the
// fallback is the whole set rather than a truncation of it.
func CollectorID(identity map[string]string) string {
	if id := identity["service.instance.id"]; id != "" {
		return id
	}
	return seam.Fingerprint(identity)
}

// signalsOf projects one Row's arrival reading, one entry per signal the
// backend was asked about.
//
// The attribute-name reading rides along where one was taken and the
// backend could answer it (ADR-0034 §4). A reading the backend could not
// give is left undeclared rather than written as an empty name set: the
// console plays an undeclared reading back as Known false with a cause, and
// an empty set would be read by a schema verdict as attributes nobody sets.
func (c *Composer) signalsOf(r Row, observed telemetry.Observed, names map[requirements.SignalKind]telemetry.AttributeNames, now time.Time) map[string]console.SignalReading {
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
		if n, asked := names[kind]; asked && n.Known {
			reading.AttributeNames = &console.AttributeNamesReading{
				Names:          n.Names,
				Truncated:      n.Truncated,
				SampledRecords: n.SampledRecords,
				TotalRecords:   n.TotalRecords,
			}
		}
		if since, ok := c.trackSilence(r, string(kind), obs, now); ok {
			reading.Since = since
		}
		out[string(kind)] = reading
	}
	return out
}

// SchemaSignals is every signal a schema-conformance requirement in the
// library is judged on, in the seam's stable signal order. The reading is
// taken for those signals on every Row rather than per Environment: which
// requirement applies where is the evaluator's business, and a reading that
// depended on it would be a second copy of that judgement.
func SchemaSignals(lib requirements.Library) []requirements.SignalKind {
	var out []requirements.SignalKind
	for _, kind := range telemetry.Signals() {
		for _, req := range lib.Sorted() {
			if req.Schema != nil && req.Schema.Covers(kind) {
				out = append(out, kind)
				break
			}
		}
	}
	return out
}

// trackSilence maintains the silence clock for one signal and reports when
// the current silence began. A signal that is arriving, or one the backend
// cannot see at all, has no silence to report: an unknown reading is not an
// observed gap (ADR-0008).
func (c *Composer) trackSilence(r Row, signal string, obs telemetry.SignalObservation, now time.Time) (time.Time, bool) {
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

// AttributeNames is the union of every attribute name the library asks
// about coverage for, sorted. Asking for all of them on every Row costs one
// aggregation each and keeps the reading independent of which requirement
// happens to apply to which Service.
func AttributeNames(lib requirements.Library) []string {
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
