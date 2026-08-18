// Self-telemetry reading (REQ-053, ADR-0039): the platform reads collector
// health from the adopter's backend through this same seam, like any other
// telemetry — never a privileged side channel. A reading is scoped to one
// Tier via the telecraft.tier resource stamp the renderer bakes into every
// artefact, and carries the commit stamps observed — the (Tier, SHA) pair
// that joins the reading back to the artefact that produced it (ADR-0039
// §5).
//
// The seam stays narrow here too: a provider reports the component-identity
// attributes verbatim, exactly as the backend recorded them. Interpreting
// them — legacy datapoint attributes, otelcol.component.* scope attributes,
// synthetic kinds, identity-dropping singletons — is the one platform-owned
// normaliser's job (ADR-0039 §3, internal/selftelemetry), never a
// provider's: two providers each guessing at join keys would be two
// normalisers.
package telemetry

import (
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
)

// SelfObserved is one Tier's self-telemetry reading. As with Observed,
// knowledge is per signal, degradation is data, and AsOf is always set.
type SelfObserved struct {
	AsOf   time.Time     `json:"as_of"`
	Window time.Duration `json:"window"`

	Signals map[requirements.SignalKind]SelfSignal `json:"signals"`
}

// Known reports whether every signal in the reading is known.
func (o SelfObserved) Known() bool {
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

// SelfSignal is one signal's self-telemetry reading for a Tier. When Known
// is false the observation fields are zero and mean nothing — absence of
// self-telemetry the provider cannot see is `Known: false` with a cause,
// never a failure and never an observed silence (ADR-0008; ADR-0039 §2:
// transport loss reads as unknown).
type SelfSignal struct {
	Known bool   `json:"known"`
	Cause string `json:"cause,omitempty"`

	Present bool  `json:"present"`
	Volume  int64 `json:"volume"`

	// Commits maps each telecraft.commit stamp observed in the window to
	// its record volume — the serving-SHA half of the join (ADR-0039 §5).
	Commits map[string]int64 `json:"commits,omitempty"`

	// Components are the distinct component-identity attribute combinations
	// observed, verbatim.
	Components []ComponentTelemetry `json:"components,omitempty"`

	// Truncated reports that the backend held more distinct component
	// identities or commit stamps than the provider read — reported, never
	// silent (ADR-0034 discipline).
	Truncated bool `json:"truncated,omitempty"`
}

// ComponentTelemetry is one component-identity attribute combination as the
// backend recorded it: the join-key attributes with their verbatim
// spellings — `receiver`/`scraper`/`processor`/`exporter` datapoint
// attributes on metrics, `otelcol.component.*` scope attributes on logs —
// and the record volume observed under that combination. An empty
// Attributes map is a real reading too: collector-level telemetry (process
// metrics, unattributed logs) carries no component identity.
type ComponentTelemetry struct {
	Attributes map[string]string `json:"attributes,omitempty"`
	Records    int64             `json:"records"`
}

// SelfSignals returns the signal kinds a self-telemetry reading covers:
// internal metrics and logs. Internal traces stay off in v1 — experimental
// upstream, and no claim consumes them (ADR-0039 §1) — so a reading that
// carried a traces key would be reporting on telemetry nothing emits.
func SelfSignals() []requirements.SignalKind {
	return []requirements.SignalKind{requirements.Logs, requirements.Metrics}
}

// SelfUnknown builds the fully degraded self-telemetry reading: every
// covered signal Known false with the same cause.
func SelfUnknown(asOf time.Time, window time.Duration, cause string) SelfObserved {
	obs := SelfObserved{
		AsOf:    asOf,
		Window:  window,
		Signals: map[requirements.SignalKind]SelfSignal{},
	}
	for _, kind := range SelfSignals() {
		obs.Signals[kind] = SelfSignal{Known: false, Cause: cause}
	}
	return obs
}
