// Package telemetry defines the TelemetryProvider seam: the reading of
// Observed state — did signal X arrive for Service Y in window W (ADR-0008).
//
// The seam is deliberately narrow. No query language, no index name, no
// product concept appears in it (REQ-023): the moment a backend's query
// syntax can pass through here, only one backend is ever really supported.
// The sanctioned extension primitive is AttributeNames — the set of
// attribute names in use for a Service, signal and window (ADR-0009,
// ADR-0034) — which unlocks schema-conformance checking as pure string
// logic without widening the seam towards any vendor's API.
//
// Not knowing is a normal state (ADR-0008). A Provider that cannot answer —
// unreachable backend, missing index, malformed response — reports the
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
	// Name identifies the implementation for logs and stamps — the
	// vendor-product-qualified name as runtime data, never a type.
	Name() string

	// Observe reads what arrived for one Service over the trailing window:
	// presence and volume per signal, plus — for each requested attribute
	// name — the fraction of records in the window carrying it, measured in
	// the same round trip. attributes may be empty.
	Observe(ctx context.Context, service Service, window time.Duration, attributes []string) Observed

	// AttributeNames reads the set of attribute names in use for one
	// Service, signal and window — the sanctioned extension primitive
	// (REQ-023, ADR-0034 §4). An implementation that can only approximate
	// (e.g. by sampling records) must say so via Truncated, never silently.
	AttributeNames(ctx context.Context, service Service, kind requirements.SignalKind, window time.Duration) AttributeNames
}

// Observed is one Service's reading across all signals. AsOf and Window
// describe the whole reading; knowledge is per signal, because degradation
// can be too — one signal's index missing says nothing about the others.
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
// observation fields are zero and mean nothing — the Cause says why the
// provider could not see, and "we cannot see" is never rendered as "it is
// absent" (ADR-0008).
type SignalObservation struct {
	Known bool   `json:"known"`
	Cause string `json:"cause,omitempty"`

	Present bool  `json:"present"`
	Volume  int64 `json:"volume"`

	// AttributeCoverage maps each requested attribute name to the fraction
	// of records in the window carrying it, in [0, 1]. Absent when no
	// attributes were requested or no records exist to measure — coverage
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

// Signals returns the signal kinds a reading covers, in stable order
// (ADR-0009: logs, metrics, traces — profiles deliberately absent).
func Signals() []requirements.SignalKind {
	return []requirements.SignalKind{requirements.Logs, requirements.Metrics, requirements.Traces}
}

// Unknown builds the fully degraded reading: every signal Known false with
// the same cause. It is what a Provider returns when the failure precedes
// any per-signal answer — an unreachable backend, an undecodable response.
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
