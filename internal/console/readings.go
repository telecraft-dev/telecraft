package console

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// Readings is the estate's declared runtime evidence: the readings a
// repository cannot hold. In production the collector collector estate arrives through
// the EstateProvider seam (ADR-0008) and the arrivals, the self-telemetry
// and the flow through the TelemetryProvider seam (ADR-0008, ADR-0039,
// ADR-0040); a static snapshot has neither backend, so the estate declares
// them here and this package plays them back through the same seams.
// Everything judged from them is judged by the product's own evaluators.
//
// Loading is strict and fails closed, matching every other authored file in
// the estate: an unknown field or a malformed document is an error naming
// the file, never a snapshot silently missing a collector.
type Readings struct {
	// AsOf is the instant every reading was taken, and the instant the
	// snapshot's judgements are made at. Mandatory: a reading without a
	// timestamp cannot have its freshness computed (ADR-0036 §2).
	AsOf time.Time `yaml:"as_of"`

	// Window is the trailing window the arrival readings cover; zero takes
	// the requirements library's longest window.
	Window requirements.Duration `yaml:"window"`

	// Collectors is the collector estate: every collector the estate reading can see,
	// with the identifying attributes Tier selectors match on.
	Collectors []CollectorReading `yaml:"collectors"`

	// Rows is the arrival reading per (Service, Environment): the Observed
	// leg of the verdict cross (ADR-0004).
	Rows []RowReading `yaml:"rows"`

	// Tiers is the self-telemetry reading per Tier (ADR-0039).
	Tiers []TierReading `yaml:"tiers"`
}

// CollectorReading is one collector as the estate reading sees it.
type CollectorReading struct {
	// ID names the collector in list surfaces. It is presentation only.
	// Identity for matching is Attributes (ADR-0013).
	ID string `yaml:"id"`

	// Attributes are the reported identifying attributes: what Tier
	// selectors and Rollout cohorts match on.
	Attributes map[string]string `yaml:"attributes"`

	// State is reporting, stale or never_seen: the flat list's state
	// column, rendered last-known-plus-age beside LastSeen (ADR-0040).
	State string `yaml:"state"`

	// Version is the collector build the reading reports.
	Version string `yaml:"version"`

	// LastSeen is the last-known reading time.
	LastSeen time.Time `yaml:"last_seen"`

	// Delivery is how this collector receives config: served over OpAMP or
	// git-delivered (REQ-041). A visible property of the collector
	// (ADR-0007), aggregated to Tier grain for the canvas.
	Delivery string `yaml:"delivery"`

	// RunningSHA is the commit stamp the collector reports running; empty
	// takes the snapshot's head, which is the settled steady state.
	RunningSHA string `yaml:"running_sha"`

	// AppliedAt is when the running artefact went APPLIED; zero is treated
	// as settled, so a Tier is not held pending forever (ADR-0038 §4b).
	AppliedAt time.Time `yaml:"applied_at"`
}

// RowReading is one (Service, Environment) arrival reading.
type RowReading struct {
	// Service is the Service's service.name, matching the conformance
	// estate's rows (ADR-0015).
	Service string `yaml:"service"`

	Environment string `yaml:"environment"`

	// Signals is the per-signal reading. A signal absent from the map is
	// Known false with a stated cause: not knowing is a normal state and
	// is reported as itself (ADR-0008), never as absence.
	Signals map[string]SignalReading `yaml:"signals"`
}

// SignalReading is one signal's arrival reading.
type SignalReading struct {
	// Known distinguishes "the backend cannot say" from "nothing arrived".
	// Absent defaults to known, the ordinary case in a declared reading.
	Known *bool  `yaml:"known"`
	Cause string `yaml:"cause"`

	Present bool  `yaml:"present"`
	Volume  int64 `yaml:"volume"`

	// AttributeCoverage is the fraction of records carrying each named
	// attribute, in [0, 1].
	AttributeCoverage map[string]float64 `yaml:"attribute_coverage"`

	// Groups is the grouping key's values in the window: the span, metric
	// or event names the records carried (ADR-0034 §4). Absent is Known
	// false with a cause, and a declared empty list is the observed
	// absence: an estate that has said nothing about groups has said
	// nothing, and rendering that as "no groups arrived" would fabricate
	// the reading a conformance check is about to judge.
	Groups *[]string `yaml:"groups"`

	// Values is the distinct value set per attribute name, declared for
	// the enum attributes a conformance check asks about (ADR-0034 §4). An
	// attribute absent from the map is Known false with a cause, for the
	// same reason: an empty set is what a check reads as "the enum was
	// never violated".
	Values map[string][]string `yaml:"values"`

	// Since is when this signal stopped arriving. The judgement never
	// raises a finding from a single instant (ADR-0035 §3) and a snapshot
	// is a single instant, so persistence is declared here exactly as a
	// Damper would have tracked it. Absent means the silence starts now,
	// which reads as dampened, the honest answer for a gap nobody has
	// watched yet.
	Since time.Time `yaml:"since"`
}

// TierReading is one Tier's self-telemetry reading (ADR-0039).
type TierReading struct {
	Tier string `yaml:"tier"`

	// Signals is the per-signal self-telemetry reading; the covered
	// signals are metrics and logs (internal traces stay off in v1).
	Signals map[string]SignalReading `yaml:"signals"`

	// Emitting declares which of the Tier's instantiated components report
	// their own telemetry: `all`, `none`, or a list of rendered component
	// ids exactly as the artefact spells them. The generator renders the
	// declaration into the join-key attribute combinations R-4 pins
	// (internal/selftelemetry interprets them), so the claims are judged
	// by the real normaliser against a real-shaped reading.
	Emitting []string `yaml:"emitting"`

	// Silent withholds named components from an `all` declaration, the
	// way to an honest expectation red on one component.
	Silent []string `yaml:"silent"`

	// Components carries verbatim component-identity attribute
	// combinations, for a reading authored exactly as a backend recorded
	// it. Merged with whatever Emitting renders.
	Components []map[string]string `yaml:"components"`

	// EverSeen reports whether any collector has ever matched this Tier.
	// Absent means "as the collector estate reads now": a Tier with collectors has
	// been seen, one without never has. Declaring it false on a populated
	// Tier is meaningless and declaring it true on an empty one is the
	// dropped-to-zero case: under-populated, never never_seen (ADR-0030).
	EverSeen *bool `yaml:"ever_seen"`

	// FirstWatched is when the platform started watching this Tier, the
	// age base for the never_seen stale-config signal (ADR-0035 §7).
	FirstWatched time.Time `yaml:"first_watched"`

	// ShortfallSince is when the current sub-floor condition began. The
	// judgement never raises a toothed finding from a single instant
	// (ADR-0035 §3), and a snapshot is a single instant, so persistence
	// is declared here, exactly as a Damper would have tracked it.
	ShortfallSince time.Time `yaml:"shortfall_since"`

	// Attributes are extra component-identity attribute combinations the
	// reading carries beyond the pipeline components: collector-level
	// telemetry and synthetic graph nodes. Tolerated, matching nothing.
	Attributes []map[string]string `yaml:"attributes"`

	// Flow is the Tier's pipeline-grain metering reading. Absent is the
	// default and stays unknown: a Tier that declares no flow keeps saying
	// it cannot see, which is a statement, not a zero (ADR-0008).
	Flow *FlowReading `yaml:"flow"`
}

// FlowReading is one Tier's declared pipeline-grain flow reading, the
// metering half of the seam (ADR-0040), as the estate declares it.
//
// This is the same predicament the collector estate and the arrivals are
// in, answered the same way. ADR-0040 §1 already settles where
// pipeline-grain metering comes from: collector self-telemetry, per
// (Tier, signal), in = receiver-accepted and out = per-exporter sent. That
// is precisely the grain the `tiers:` section above already declares.
// This adds one more field of a seam the file declares two others of, and
// widens nothing. A snapshot has no backend to derive metering from, so
// the estate declares the reading and this package plays it back through
// the Provider seam, exactly as it does for the arrivals: the evaluators
// and the card contract cannot tell a declared reading from a live one.
//
// The declaration mirrors telemetry.Metered field for field on purpose. A
// declared reading the meter could not have produced is not a reading, and
// the mirror is what holds the two to the same shape.
type FlowReading struct {
	// Signals is the per-signal reading. A signal absent from the map is
	// Known false with a stated cause: a declaration covering one lane
	// and not another degrades per lane, never wholesale (ADR-0008).
	Signals map[string]FlowSignalReading `yaml:"signals"`

	// Incarnations is the Tier-wide restart-rate reading (ADR-0040 §4). It
	// sits beside the signals rather than inside them because a restart
	// takes the whole collector process with it.
	Incarnations *FlowIncarnations `yaml:"incarnations"`
}

// FlowSignalReading is one signal's declared flow, mirroring
// telemetry.MeteredSignal.
type FlowSignalReading struct {
	// Known distinguishes "the backend cannot say" from "nothing flowed".
	// Absent defaults to known, the ordinary case in a declared reading.
	Known *bool  `yaml:"known"`
	Cause string `yaml:"cause"`

	// In is receiver-accepted items over the window and Out exporter-sent
	// items, summed across instances. Items are the unit (ADR-0040 §2):
	// there is no byte field to declare because no byte counter exists on
	// these surfaces, and an estimated one would be an invention.
	In  int64 `yaml:"in"`
	Out int64 `yaml:"out"`

	// Exporters maps each exporter's rendered id (`type` or `type/name`,
	// exactly as the Blueprint spells it) to its own sent-item count. A
	// Hop's throughput is its feeding exporter's out-rate (ADR-0040 §1),
	// and this is where that rate would be read; Out is their sum.
	Exporters map[string]int64 `yaml:"exporters"`

	// The error-rate readings: the only reds metering itself sources
	// (ADR-0040 §3). In-minus-out is not among them and never becomes one.
	Refused       int64 `yaml:"refused"`
	SendFailed    int64 `yaml:"send_failed"`
	EnqueueFailed int64 `yaml:"enqueue_failed"`

	// Newest is the timestamp of the newest self-telemetry datapoint the
	// counters were read from, the pipeline-grain freshness base
	// (ADR-0040 §4). Absent declares a known-empty window: nothing
	// reported, which the card renders as silent rather than as unknown.
	Newest time.Time `yaml:"newest"`

	// Truncated declares that more exporters or instances existed than the
	// reading summed, so the figures are a floor. Reported, never silent.
	Truncated bool `yaml:"truncated"`
}

// FlowIncarnations is the declared restart-rate reading, mirroring
// telemetry.Incarnations.
type FlowIncarnations struct {
	Known *bool  `yaml:"known"`
	Cause string `yaml:"cause"`

	// Count is how many distinct collector process incarnations reported
	// in the window. It counts process starts, never health: a Tier that
	// scaled out and one that crash-looped both raise it, and telling them
	// apart belongs to the claims, not the meter.
	Count int `yaml:"count"`

	Truncated bool `yaml:"truncated"`
}

// emitsAll reports the `all` shorthand.
func (t TierReading) emitsAll() bool {
	for _, e := range t.Emitting {
		if e == "all" {
			return true
		}
	}
	return false
}

// silentSet indexes the withheld component ids.
func (t TierReading) silentSet() map[string]bool {
	out := map[string]bool{}
	for _, s := range t.Silent {
		out[s] = true
	}
	return out
}

// flowProblems validates one Tier's declared flow. The rule it enforces
// throughout is a single one: the declaration must be a reading the meter
// could have taken. A figure the counters cannot produce (a negative
// count off a monotonic counter, an exporter split that does not add up to
// the out-rate it was summed from, a datapoint newer than the instant the
// reading was taken) is an authoring mistake, and a snapshot built over
// it would put a number on a card that no backend would ever have
// returned. Fails closed, like every other authored file in the estate.
func (t TierReading) flowProblems(asOf time.Time) []string {
	if t.Flow == nil {
		return nil
	}
	var problems []string
	names := make([]string, 0, len(t.Flow.Signals))
	for name := range t.Flow.Signals {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		sig := t.Flow.Signals[name]
		where := fmt.Sprintf("tier %s flow %s", t.Tier, name)
		if !requirements.SignalKind(name).Valid() {
			problems = append(problems, fmt.Sprintf("%s is not a signal: use logs, metrics, or traces", where))
			continue
		}

		figures := map[string]int64{
			"in": sig.In, "out": sig.Out, "refused": sig.Refused,
			"send_failed": sig.SendFailed, "enqueue_failed": sig.EnqueueFailed,
		}
		for _, field := range []string{"in", "out", "refused", "send_failed", "enqueue_failed"} {
			if figures[field] < 0 {
				problems = append(problems, fmt.Sprintf("%s declares %s %d, but a count is never negative", where, field, figures[field]))
			}
		}

		// An unknown carrying figures says two things at once. Known false
		// means the counters could not be read, and then every count is
		// zero and means nothing (ADR-0040 §6).
		if sig.Known != nil && !*sig.Known {
			counted := sig.In != 0 || sig.Out != 0 || sig.Refused != 0 ||
				sig.SendFailed != 0 || sig.EnqueueFailed != 0 || len(sig.Exporters) > 0
			if counted {
				problems = append(problems, where+" is marked unknown but carries figures: an unknown reading has no counts")
			}
			if !sig.Newest.IsZero() {
				problems = append(problems, where+" is marked unknown but carries a newest timestamp")
			}
		}

		// Out is the sum of the per-exporter splits by construction: the
		// meter reads the split and sums it server-side. A declaration
		// whose parts do not add up to its whole would make the card and
		// the exporter rates disagree about the same window.
		if len(sig.Exporters) > 0 {
			ids := make([]string, 0, len(sig.Exporters))
			for id := range sig.Exporters {
				ids = append(ids, id)
			}
			sort.Strings(ids)

			var sum int64
			for _, id := range ids {
				n := sig.Exporters[id]
				sum += n
				if strings.TrimSpace(id) == "" {
					problems = append(problems, where+" declares an exporter with no id: use the rendered `type` or `type/name`")
				}
				if n < 0 {
					problems = append(problems, fmt.Sprintf("%s declares exporter %q sending %d items, but a count is never negative", where, id, n))
				}
			}
			if sum != sig.Out {
				problems = append(problems, fmt.Sprintf("%s declares an exporter split summing to %d, but out is %d: out must equal the sum of the exporters' counts", where, sum, sig.Out))
			}
		}

		if !sig.Newest.IsZero() && !asOf.IsZero() && sig.Newest.After(asOf) {
			problems = append(problems, fmt.Sprintf("%s declares a newest datapoint at %s, which is after the as_of the reading was taken at",
				where, sig.Newest.UTC().Format(time.RFC3339)))
		}
	}

	inc := t.Flow.Incarnations
	if inc == nil {
		return problems
	}
	if inc.Count < 0 {
		problems = append(problems, fmt.Sprintf("tier %s flow declares %d incarnations, but a count of process starts is never negative", t.Tier, inc.Count))
	}
	if inc.Known != nil && !*inc.Known && inc.Count != 0 {
		problems = append(problems, fmt.Sprintf("tier %s flow marks its incarnation count unknown but declares %d: an unknown reading has no count", t.Tier, inc.Count))
	}
	return problems
}

// LoadReadings reads and validates one readings file.
func LoadReadings(path string) (Readings, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Readings{}, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return Readings{}, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return Readings{}, fmt.Errorf("%s: empty file. Declare the collector estate and its telemetry arrivals here", path)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var r Readings
	if err := dec.Decode(&r); err != nil {
		return Readings{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return Readings{}, fmt.Errorf("%s: more than one YAML document in the file. Keep one document per file", path)
	}

	var problems []string
	if r.AsOf.IsZero() {
		problems = append(problems, "as_of is missing: every reading carries the instant it was taken")
	}
	seen := map[string]string{}
	for i, c := range r.Collectors {
		ctx := fmt.Sprintf("collector %d", i)
		if c.ID != "" {
			ctx = fmt.Sprintf("collector %q", c.ID)
		}
		if c.ID == "" {
			problems = append(problems, ctx+" has no id")
		} else if prev, dup := seen[c.ID]; dup {
			problems = append(problems, fmt.Sprintf("%s appears twice (%s)", ctx, prev))
		} else {
			seen[c.ID] = ctx
		}
		if len(c.Attributes) == 0 {
			problems = append(problems, ctx+" reports no identifying attributes, so no Tier can match it")
		}
		switch c.State {
		case "reporting", "stale", "never_seen":
		case "":
			problems = append(problems, ctx+" has no state: use reporting, stale, or never_seen")
		default:
			problems = append(problems, fmt.Sprintf("%s has state %q: use reporting, stale, or never_seen", ctx, c.State))
		}
		switch c.Delivery {
		case "served", "git":
		case "":
			problems = append(problems, ctx+" declares no delivery path: use served or git")
		default:
			problems = append(problems, fmt.Sprintf("%s has delivery %q: use served or git", ctx, c.Delivery))
		}
	}
	for _, row := range r.Rows {
		if row.Service == "" || row.Environment == "" {
			problems = append(problems, "a row reading names no service or no environment: each row is one Service in one Environment")
		}
		for name := range row.Signals {
			if !requirements.SignalKind(name).Valid() {
				problems = append(problems, fmt.Sprintf("row %s/%s reads signal %q, which is not a signal: use logs, metrics, or traces", row.Service, row.Environment, name))
			}
		}
	}
	for _, tier := range r.Tiers {
		if tier.Tier == "" {
			problems = append(problems, "a tier reading names no tier")
		}
		for name := range tier.Signals {
			if !requirements.SignalKind(name).Valid() {
				problems = append(problems, fmt.Sprintf("tier %s reads signal %q, which is not a signal: use logs, metrics, or traces", tier.Tier, name))
			}
		}
		problems = append(problems, tier.flowProblems(r.AsOf)...)
	}
	if len(problems) > 0 {
		return Readings{}, fmt.Errorf("invalid readings in %s:\n  - %s", path, strings.Join(problems, "\n  - "))
	}
	return r, nil
}

// row finds the arrival reading for one row.
func (r Readings) row(service, environment string) (RowReading, bool) {
	for _, row := range r.Rows {
		if row.Service == service && row.Environment == environment {
			return row, true
		}
	}
	return RowReading{}, false
}

// tier finds the self-telemetry reading for one Tier.
func (r Readings) tier(id string) (TierReading, bool) {
	for _, t := range r.Tiers {
		if t.Tier == id {
			return t, true
		}
	}
	return TierReading{}, false
}

// observed converts one row reading to the seam's shape.
func (s SignalReading) observed(attributes []string) telemetry.SignalObservation {
	if s.Known != nil && !*s.Known {
		cause := s.Cause
		if cause == "" {
			cause = "the declared reading marks this signal unknown"
		}
		return telemetry.SignalObservation{Known: false, Cause: cause}
	}
	obs := telemetry.SignalObservation{Known: true, Present: s.Present, Volume: s.Volume}
	if len(attributes) > 0 && len(s.AttributeCoverage) > 0 {
		obs.AttributeCoverage = map[string]float64{}
		for _, a := range attributes {
			if v, ok := s.AttributeCoverage[a]; ok {
				obs.AttributeCoverage[a] = v
			}
		}
	}
	return obs
}

// selfSignal converts one tier reading's signal to the seam's shape.
func (s SignalReading) selfSignal(components []telemetry.ComponentTelemetry) telemetry.SelfSignal {
	if s.Known != nil && !*s.Known {
		cause := s.Cause
		if cause == "" {
			cause = "the declared reading marks this signal unknown"
		}
		return telemetry.SelfSignal{Known: false, Cause: cause}
	}
	return telemetry.SelfSignal{
		Known:      true,
		Present:    s.Present,
		Volume:     s.Volume,
		Components: components,
	}
}

// metered converts one declared signal flow to the seam's shape.
func (f FlowSignalReading) metered() telemetry.MeteredSignal {
	if f.Known != nil && !*f.Known {
		cause := f.Cause
		if cause == "" {
			cause = "the declared reading marks this signal's flow unknown"
		}
		return telemetry.MeteredSignal{Known: false, Cause: cause}
	}
	sig := telemetry.MeteredSignal{
		Known:         true,
		In:            f.In,
		Out:           f.Out,
		Refused:       f.Refused,
		SendFailed:    f.SendFailed,
		EnqueueFailed: f.EnqueueFailed,
		Newest:        f.Newest,
		Truncated:     f.Truncated,
	}
	for id, n := range f.Exporters {
		if sig.Exporters == nil {
			sig.Exporters = map[string]int64{}
		}
		sig.Exporters[id] = n
	}
	return sig
}

// incarnations converts the declared restart rate to the seam's shape. An
// estate that declares flow but no incarnation count gets an unknown
// churn reading beside known volume rows: knowledge is per reading, and
// the card says so rather than showing a restful zero (ADR-0008).
func (f FlowReading) incarnations(tier string) telemetry.Incarnations {
	if f.Incarnations == nil {
		return telemetry.Incarnations{
			Known: false,
			Cause: "the estate's readings file declares no incarnation count for " + tier +
				", so the restart rate is unknown",
		}
	}
	inc := *f.Incarnations
	if inc.Known != nil && !*inc.Known {
		cause := inc.Cause
		if cause == "" {
			cause = "the declared reading marks this Tier's incarnation count unknown"
		}
		return telemetry.Incarnations{Known: false, Cause: cause}
	}
	return telemetry.Incarnations{Known: true, Count: inc.Count, Truncated: inc.Truncated}
}

// missing is the reading for a row or signal the estate declared nothing
// for: Known false with a cause, never a fabricated absence (ADR-0008).
func missing(what string) telemetry.SignalObservation {
	return telemetry.SignalObservation{
		Known: false,
		Cause: "the estate's readings file declares no reading for " + what,
	}
}

// provider plays the declared readings back through the TelemetryProvider
// seam, so the evaluators judging them cannot tell a declared reading from
// a live one. That is the point: the judgement is the product's.
type provider struct {
	readings Readings
	window   time.Duration

	// components is the self-telemetry component identity per Tier,
	// rendered from the Tier readings' Emitting declarations.
	components map[string][]telemetry.ComponentTelemetry
}

// The playback answers the whole seam, metering included: a partial
// implementation would mean the snapshot judged declared readings through
// a different door than a live console does.
var _ telemetry.Provider = (*provider)(nil)

// Name identifies the reading's origin in stamps and logs. It is the
// estate's own declaration, not a backend, and says so.
func (p *provider) Name() string { return "telecraft/declared-readings" }

func (p *provider) Observe(_ context.Context, service telemetry.Service, window time.Duration, attributes []string) telemetry.Observed {
	obs := telemetry.Observed{
		AsOf:    p.readings.AsOf,
		Window:  window,
		Signals: map[requirements.SignalKind]telemetry.SignalObservation{},
	}
	row, ok := p.readings.row(service.Name, service.Environment)
	for _, kind := range telemetry.Signals() {
		if !ok {
			obs.Signals[kind] = missing(fmt.Sprintf("%s in %s", service.Name, service.Environment))
			continue
		}
		sig, declared := row.Signals[string(kind)]
		if !declared {
			obs.Signals[kind] = missing(fmt.Sprintf("%s %s in %s", kind, service.Name, service.Environment))
			continue
		}
		obs.Signals[kind] = sig.observed(attributes)
	}
	return obs
}

func (p *provider) AttributeNames(_ context.Context, service telemetry.Service, kind requirements.SignalKind, window time.Duration) telemetry.AttributeNames {
	row, ok := p.readings.row(service.Name, service.Environment)
	if !ok {
		return telemetry.AttributeNames{
			Known:  false,
			Cause:  missing(service.Name).Cause,
			AsOf:   p.readings.AsOf,
			Window: window,
		}
	}
	sig, declared := row.Signals[string(kind)]
	if !declared {
		return telemetry.AttributeNames{
			Known:  false,
			Cause:  missing(string(kind) + " for " + service.Name).Cause,
			AsOf:   p.readings.AsOf,
			Window: window,
		}
	}
	names := make([]string, 0, len(sig.AttributeCoverage))
	for a := range sig.AttributeCoverage {
		names = append(names, a)
	}
	sort.Strings(names)
	return telemetry.AttributeNames{
		Known:  true,
		AsOf:   p.readings.AsOf,
		Window: window,
		Names:  names,
	}
}

// unnamedService is why an unnamed Service is refused rather than answered.
// The declaration is keyed on service.name, and an unnamed ask would either
// match nothing or, in a live implementation, match the whole backend. The
// two must never differ, so the playback refuses it the same way the live
// provider does (ADR-0034 §4).
const unnamedService = "no service.name was given, so the reading would not be one Service's"

// signal resolves one (Service, Environment, signal) declaration, or the
// cause the reading is Known false for. It is the door the two ADR-0034 §4
// primitives come through, so an undeclared row and an undeclared signal
// read the same whichever primitive asked.
func (p *provider) signal(service telemetry.Service, kind requirements.SignalKind) (SignalReading, string) {
	row, ok := p.readings.row(service.Name, service.Environment)
	if !ok {
		return SignalReading{}, missing(service.Name).Cause
	}
	sig, declared := row.Signals[string(kind)]
	if !declared {
		return SignalReading{}, missing(string(kind) + " for " + service.Name).Cause
	}
	return sig, ""
}

// DistinctValues plays back the declared value set for one attribute. A
// snapshot has no backend to aggregate, so an attribute the estate has not
// declared values for is Known false with the cause said out loud, never an
// empty set: the difference between "this enum was never violated" and "we
// never looked" is the whole of ADR-0034 §4's fidelity rule.
func (p *provider) DistinctValues(_ context.Context, service telemetry.Service, kind requirements.SignalKind, attribute string, window time.Duration) telemetry.DistinctValues {
	unknown := func(cause string) telemetry.DistinctValues {
		return telemetry.DistinctValuesUnknown(p.readings.AsOf, window, attribute, cause)
	}
	if attribute == "" {
		return unknown("no attribute was named: a value set of nothing is not a reading")
	}
	if service.Name == "" {
		return unknown(telemetry.NotServiceScoped(service, unnamedService))
	}
	sig, cause := p.signal(service, kind)
	if cause != "" {
		return unknown(cause)
	}
	values, declared := sig.Values[attribute]
	if !declared {
		return unknown("the estate's readings file declares no distinct values for " +
			attribute + " on " + string(kind) + " for " + service.Name)
	}

	reading := telemetry.DistinctValues{
		Known:     true,
		AsOf:      p.readings.AsOf,
		Window:    window,
		Attribute: attribute,
		Cap:       telemetry.MaxDistinctValues,
	}
	reading.Values, reading.Truncated = capped(values, telemetry.MaxDistinctValues)
	return reading
}

// GroupNames plays back the declared group set for one signal. A declared
// reading is complete by construction, so it is never Truncated unless the
// estate declared more groups than the seam's cap holds, which is itself
// worth surfacing.
func (p *provider) GroupNames(_ context.Context, service telemetry.Service, kind requirements.SignalKind, window time.Duration) telemetry.GroupNames {
	unknown := func(cause string) telemetry.GroupNames {
		return telemetry.GroupNamesUnknown(p.readings.AsOf, window, kind, cause)
	}
	if telemetry.GroupKeyFor(kind) == "" {
		return unknown(string(kind) + " has no grouping key")
	}
	if service.Name == "" {
		return unknown(telemetry.NotServiceScoped(service, unnamedService))
	}
	sig, cause := p.signal(service, kind)
	if cause != "" {
		return unknown(cause)
	}
	if sig.Groups == nil {
		return unknown("the estate's readings file declares no " + string(kind) +
			" group names for " + service.Name)
	}

	reading := telemetry.GroupNames{
		Known:  true,
		AsOf:   p.readings.AsOf,
		Window: window,
		Key:    telemetry.GroupKeyFor(kind),
	}
	reading.Names, reading.Truncated = capped(*sig.Groups, telemetry.MaxGroupNames)
	return reading
}

// capped sorts and de-duplicates a declared set and clips it to the seam's
// hard cap, reporting the clip rather than swallowing it.
func capped(declared []string, limit int) ([]string, bool) {
	seen := map[string]bool{}
	out := make([]string, 0, len(declared))
	for _, v := range declared {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	if len(out) > limit {
		return out[:limit], true
	}
	return out, false
}

func (p *provider) ObserveSelf(_ context.Context, tier string, window time.Duration) telemetry.SelfObserved {
	out := telemetry.SelfObserved{
		AsOf:    p.readings.AsOf,
		Window:  window,
		Signals: map[requirements.SignalKind]telemetry.SelfSignal{},
	}
	reading, ok := p.readings.tier(tier)
	if !ok {
		return telemetry.SelfUnknown(p.readings.AsOf, window,
			"the estate's readings file declares no self-telemetry reading for "+tier)
	}
	for _, kind := range telemetry.SelfSignals() {
		sig, declared := reading.Signals[string(kind)]
		if !declared {
			out.Signals[kind] = telemetry.SelfSignal{
				Known: false,
				Cause: "the estate's readings file declares no " + string(kind) + " self-telemetry for " + tier,
			}
			continue
		}
		out.Signals[kind] = sig.selfSignal(p.components[tier])
	}
	return out
}

// Meter plays the declared flow reading back through the metering seam
// (ADR-0040). An estate that declares nothing keeps the honest neutral it
// has always had: every signal Known false with the cause said out loud,
// because a snapshot has no backend to derive metering from, and "we
// cannot see the counters" is never rendered as "nothing flowed"
// (ADR-0008, ADR-0040 §6).
//
// Degradation is per reading, not per Tier. A Tier that declares metrics
// flow and nothing else carries a metrics row with figures beside a logs
// row that says why it is empty, which is the whole reason knowledge is
// held per signal at this seam.
func (p *provider) Meter(_ context.Context, tier string, window time.Duration) telemetry.Metered {
	reading, ok := p.readings.tier(tier)
	if !ok || reading.Flow == nil {
		return telemetry.MeterUnknown(p.readings.AsOf, window, flowCause)
	}

	m := telemetry.Metered{
		AsOf:    p.readings.AsOf,
		Window:  window,
		Signals: map[requirements.SignalKind]telemetry.MeteredSignal{},
	}
	for _, kind := range telemetry.Signals() {
		sig, declared := reading.Flow.Signals[string(kind)]
		if !declared {
			m.Signals[kind] = telemetry.MeteredSignal{
				Known: false,
				Cause: "the estate's readings file declares no " + string(kind) + " flow for " + tier,
			}
			continue
		}
		m.Signals[kind] = sig.metered()
	}
	m.Incarnations = reading.Flow.incarnations(tier)
	return m
}
