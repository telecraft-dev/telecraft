// Package telemetry defines the TelemetryProvider seam: the reading of
// Observed state: did signal X arrive for Service Y in window W (ADR-0008).
//
// The seam is deliberately narrow. No query language, no index name, no
// product concept appears in it (REQ-023): the moment a backend's query
// syntax can pass through here, only one backend is ever really supported.
// The sanctioned extension is three primitives plus a grouping key
// (ADR-0009, ADR-0034 §4): AttributeNames, the attribute names in use for
// a Service, signal and window; DistinctValues, the value set one
// attribute carries, hard-capped; and GroupNames, the values of the
// grouping key a signal is grouped by, because semconv required-sets are
// per group. Together they unlock schema-conformance checking as pure
// string logic without widening the seam towards any vendor's API.
//
// All three are service-scoped by contract, and the fidelity rule runs
// both ways (ADR-0034 §4): a reading a Provider cannot scope to one
// Service is Known false with a stated cause, never an approximation
// passed off as the Service's own answer and never one Service's records
// reported under another's name. A wrong answer here is worse than no
// answer, because a wrong one is acted on.
//
// Not knowing is a normal state (ADR-0008). A Provider that cannot answer
// (unreachable backend, missing index, malformed response) reports the
// affected readings with Known false and a cause, and methods return no
// error: degradation is data in the reading, never a fabricated value and
// never a crash. Every reading carries AsOf, the instant it was taken, so a
// stale answer can never masquerade as a fresh one.
//
// The signal vocabulary is owned by internal/requirements (ADR-0009); the
// seam adopts it rather than minting a synonym (ADR-0001's rule, applied
// internally).
package telemetry

import (
	"context"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
)

// Service is the governed unit the reading is scoped to, identified by
// service.name (ADR-0015).
type Service struct {
	Name string

	// Environment narrows the reading to one Environment, matched on
	// deployment.environment.name (ADR-0023). Empty means the reading is not
	// narrowed. The verdict cross always narrows: verdicts never blend
	// across environments (ADR-0033), and an unscoped reading of a Service
	// running in several environments would let staging telemetry mask a
	// production outage of the same signal.
	Environment string
}

// Provider is the TelemetryProvider seam. Implementations live under
// internal/provider/ and are vendor-product-qualified there (ADR-0001);
// nothing vendor-shaped crosses this interface in either direction.
type Provider interface {
	// Name identifies the implementation for logs and stamps: the
	// vendor-product-qualified name as runtime data, never a type.
	Name() string

	// Observe reads what arrived for one Service over the trailing window:
	// presence and volume per signal, plus, for each requested attribute
	// name, the fraction of records in the window carrying it, measured in
	// the same round trip. attributes may be empty.
	Observe(ctx context.Context, service Service, window time.Duration, attributes []string) Observed

	// AttributeNames reads the set of attribute names in use for one
	// Service, signal and window, the sanctioned extension primitive
	// (REQ-023, ADR-0034 §4). An implementation that can only approximate
	// (e.g. by sampling records) must say so via Truncated, never silently.
	AttributeNames(ctx context.Context, service Service, kind requirements.SignalKind, window time.Duration) AttributeNames

	// DistinctValues reads the distinct values one attribute carries for
	// one Service, signal and window (ADR-0034 §4). The reading is
	// hard-capped at MaxDistinctValues and truncation is always reported,
	// so a clipped value set can never read as the whole one.
	//
	// Callers offer this only for attributes the Schema Registry declares
	// as enums. That constraint is caller-side rather than part of the
	// signature: the seam holds no registry, and a primitive that took one
	// would drag the vocabulary across the seam it exists to keep narrow.
	DistinctValues(ctx context.Context, service Service, kind requirements.SignalKind, attribute string, window time.Duration) DistinctValues

	// GroupNames reads the values of the grouping key one signal is
	// grouped by for one Service and window: span names for traces, metric
	// names for metrics, event names for logs (ADR-0034 §4). Presence per
	// name is the whole reading, because semconv states its required-sets
	// per group, so a conformance check cannot ask what is required until
	// it knows which groups arrived.
	GroupNames(ctx context.Context, service Service, kind requirements.SignalKind, window time.Duration) GroupNames

	// ObserveSelf reads the collector self-telemetry that arrived for one
	// Tier over the trailing window (REQ-053, ADR-0039): presence and
	// volume per internal signal, the commit stamps observed, and the raw
	// component-identity attribute combinations, verbatim. tier is the
	// team-qualified Tier id, matched on the telecraft.tier resource stamp
	// every rendered artefact bakes into its own telemetry (ADR-0039 §5).
	// This is the only way self-telemetry readings come in: the
	// platform reads them from the adopter's backend like any other
	// telemetry, never over a privileged side channel.
	ObserveSelf(ctx context.Context, tier string, window time.Duration) SelfObserved

	// LiveCheckFindings reads the finding records the live-check tap
	// emitted for one Service over the trailing window, verbatim, plus the
	// liveness leg: whether anything was sent to the tap in the window,
	// read from the teeing Tier's own self-telemetry counters (ADR-0034 §5
	// and §6, issue #159). The findings came home as ordinary log records
	// and are read back like any other telemetry, never over a privileged
	// side channel. A provider that cannot see either leg reports it Known
	// false with a cause, never an observed silence: a clean stream and a
	// dead tap are both silent, and only the liveness leg can tell them
	// apart.
	LiveCheckFindings(ctx context.Context, service Service, window time.Duration) LiveCheckFindings

	// Meter reads one Tier's pipeline-grain flow counters over the
	// trailing window (REQ-050, ADR-0040): per (Tier, signal) in and out
	// item counts, the per-exporter split a Hop's throughput reads, the
	// error-rate readings, and the Tier's incarnation count. Computed on
	// read, never stored, because the platform holds no time series, and history
	// is a range query against the adopter's backend at the adopter's
	// retention (ADR-0040 §5).
	//
	// An implementation whose backend cannot aggregate this way declares
	// the incapability by returning MeterUnknown with a cause (ADR-0036
	// pattern, ADR-0040 §6): metering never invents, and a reading nobody
	// can take is Known false, not a zero.
	Meter(ctx context.Context, tier string, window time.Duration) Metered
}

// Observed is one Service's reading across all signals. AsOf and Window
// describe the whole reading; knowledge is per signal, because degradation
// can be too: one signal's index missing says nothing about the others.
type Observed struct {
	// AsOf is the instant the reading was taken. Always set, including on
	// fully degraded readings: "we could not see, as of when" is still a
	// statement with a timestamp.
	AsOf   time.Time     `json:"as_of"`
	Window time.Duration `json:"window"`

	Signals map[requirements.SignalKind]SignalObservation `json:"signals"`
}

// Known reports whether every signal in the reading is known. A reading
// with no signals at all is not knowledge.
func (o Observed) Known() bool {
	if len(o.Signals) == 0 {
		return false
	}
	for _, s := range o.Signals {
		if !s.Known {
			return false
		}
	}
	return true
}

// SignalObservation is the reading for one signal. When Known is false the
// observation fields are zero and mean nothing. The Cause says why the
// provider could not see, and "we cannot see" is never rendered as "it is
// absent" (ADR-0008).
type SignalObservation struct {
	Known bool   `json:"known"`
	Cause string `json:"cause,omitempty"`

	Present bool  `json:"present"`
	Volume  int64 `json:"volume"`

	// Newest is the timestamp of the newest record that landed in the
	// window, the service-grain freshness base (ADR-0040 §4): the age of
	// the newest landed record per (Service, Environment, signal). Zero
	// when nothing landed, where the absence itself is the reading.
	Newest time.Time `json:"newest,omitempty"`

	// AttributeCoverage maps each requested attribute name to the fraction
	// of records in the window carrying it, in [0, 1]. Absent when no
	// attributes were requested or no records exist to measure, because coverage
	// over zero records is omitted, never fabricated as 0 or 1.
	AttributeCoverage map[string]float64 `json:"attribute_coverage,omitempty"`
}

// AttributeNames is the reading for the extension primitive: which
// attribute names are in use for one Service, signal and window.
type AttributeNames struct {
	Known bool   `json:"known"`
	Cause string `json:"cause,omitempty"`

	AsOf   time.Time     `json:"as_of"`
	Window time.Duration `json:"window"`

	// Names is sorted and de-duplicated.
	Names []string `json:"names,omitempty"`

	// Truncated reports that Names was derived from fewer records than the
	// window holds, so names may be missing. Truncation is always reported
	// (ADR-0034): a sampled union presented as complete would be a silent
	// approximation.
	Truncated      bool  `json:"truncated"`
	SampledRecords int64 `json:"sampled_records"`
	TotalRecords   int64 `json:"total_records"`
}

// MaxDistinctValues is the hard cap every DistinctValues reading is bound
// by (ADR-0034 §4). It is contractual rather than per implementation: an
// enum's value set is small by definition, so a reading that reaches the
// cap is either not an enum or is instrumented with an unbounded value,
// and in both cases the useful answer is the truncation rather than a
// longer list. A Provider never returns more values than this.
const MaxDistinctValues = 100

// MaxGroupNames is the hard cap every GroupNames reading is bound by. Span
// names are the classic unbounded dimension, and a group set that reaches
// the cap is itself worth surfacing, so the cap is reported as truncation
// rather than raised.
const MaxGroupNames = 500

// DistinctValues is the reading for the value-set primitive: which values
// one attribute carries for one Service, signal and window.
type DistinctValues struct {
	Known bool   `json:"known"`
	Cause string `json:"cause,omitempty"`

	AsOf   time.Time     `json:"as_of"`
	Window time.Duration `json:"window"`

	// Attribute is the attribute name the reading answers for, echoed back
	// so a reading can be read on its own.
	Attribute string `json:"attribute"`

	// Values is sorted and de-duplicated, and never longer than Cap.
	Values []string `json:"values,omitempty"`

	// Truncated reports that the window holds values Values does not, so
	// the set is a floor rather than the whole. Truncation is always
	// reported (ADR-0034 §4): a clipped set presented as complete would
	// turn a missing violation into a pass.
	Truncated bool `json:"truncated"`

	// Cap is the hard cap in force for this reading, at most
	// MaxDistinctValues. Carried so a reader can tell a set that fitted
	// from one that was cut, without knowing the Provider.
	Cap int `json:"cap"`
}

// GroupKey names the dimension a signal's records are grouped by. The
// vocabulary is semconv's own, adopted rather than re-minted, and it is
// backend-neutral: it says which dimension, never which field or index
// holds it.
type GroupKey string

const (
	// SpanName groups traces by span name.
	SpanName GroupKey = "span.name"
	// MetricName groups metrics by metric name.
	MetricName GroupKey = "metric.name"
	// EventName groups logs by event name.
	EventName GroupKey = "event.name"
)

// GroupKeyFor returns the grouping key one signal is grouped by. A kind
// with no grouping key returns the empty key, and a Provider asked for one
// reports Known false rather than guessing at a dimension.
func GroupKeyFor(kind requirements.SignalKind) GroupKey {
	switch kind {
	case requirements.Traces:
		return SpanName
	case requirements.Metrics:
		return MetricName
	case requirements.Logs:
		return EventName
	default:
		return ""
	}
}

// GroupNames is the reading for the grouping-key primitive: which spans,
// metrics or events arrived for one Service, signal and window. It mirrors
// AttributeNames field for field, because it is the same shape of answer
// about a different dimension.
type GroupNames struct {
	Known bool   `json:"known"`
	Cause string `json:"cause,omitempty"`

	AsOf   time.Time     `json:"as_of"`
	Window time.Duration `json:"window"`

	// Key is the dimension Names are values of, carried so a reading can
	// be read on its own.
	Key GroupKey `json:"key"`

	// Names is sorted and de-duplicated, and never longer than
	// MaxGroupNames.
	Names []string `json:"names,omitempty"`

	// Truncated reports that the window holds groups Names does not,
	// whether because the cap was reached or because the reading was
	// derived from a sample. Always reported, never silent.
	Truncated bool `json:"truncated"`

	// SampledRecords and TotalRecords describe a reading derived from a
	// sample of the window's records. Both zero means the reading was
	// aggregated over the whole window instead, where the only way to lose
	// a group is the cap, which Truncated already reports.
	SampledRecords int64 `json:"sampled_records"`
	TotalRecords   int64 `json:"total_records"`
}

// DistinctValuesUnknown builds the degraded value-set reading: Known false
// with a cause, and no values at all. Every reading carries AsOf, this one
// included: "we could not see, as of when" is still a statement with a
// timestamp.
func DistinctValuesUnknown(asOf time.Time, window time.Duration, attribute, cause string) DistinctValues {
	return DistinctValues{
		Known:     false,
		Cause:     cause,
		AsOf:      asOf,
		Window:    window,
		Attribute: attribute,
		Cap:       MaxDistinctValues,
	}
}

// GroupNamesUnknown builds the degraded grouping-key reading: Known false
// with a cause, and no names at all.
func GroupNamesUnknown(asOf time.Time, window time.Duration, kind requirements.SignalKind, cause string) GroupNames {
	return GroupNames{
		Known:  false,
		Cause:  cause,
		AsOf:   asOf,
		Window: window,
		Key:    GroupKeyFor(kind),
	}
}

// NotServiceScoped renders the cause a Provider states when its backend
// cannot narrow a primitive to one Service, the fidelity rule of ADR-0034
// §4 made concrete. The affected reading is Known false and carries this,
// so the checks it feeds are unknown rather than passing.
//
// The two failures it exists to forbid are the tempting ones. An
// index-scoped union answered as though it were the Service's own reading
// is a silent approximation, and a value another Service put in the index
// reported under this Service's name is a misattribution. Both read as
// knowledge, and both are wrong.
func NotServiceScoped(service Service, detail string) string {
	who := service.Name
	if service.Environment != "" {
		who += " in " + service.Environment
	}
	cause := "the backend cannot scope this reading to " + who
	if detail != "" {
		cause += ": " + detail
	}
	return cause
}

// Signals returns the signal kinds a reading covers, in stable order
// (ADR-0009: logs, metrics, traces; profiles deliberately absent).
func Signals() []requirements.SignalKind {
	return []requirements.SignalKind{requirements.Logs, requirements.Metrics, requirements.Traces}
}

// Unknown builds the fully degraded reading: every signal Known false with
// the same cause. It is what a Provider returns when the failure precedes
// any per-signal answer: an unreachable backend, an undecodable response.
func Unknown(asOf time.Time, window time.Duration, cause string) Observed {
	obs := Observed{
		AsOf:    asOf,
		Window:  window,
		Signals: map[requirements.SignalKind]SignalObservation{},
	}
	for _, kind := range Signals() {
		obs.Signals[kind] = SignalObservation{Known: false, Cause: cause}
	}
	return obs
}
