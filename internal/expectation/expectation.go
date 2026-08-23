// Package expectation is the Expectation engine (REQ-051, ADR-0038):
// machinery behind existing verdicts, never new vocabulary. From the
// Intended config at a commit SHA it derives Claims (checkable assertions
// about what telemetry should arrive) and judges arrivals against them,
// so that green means "the config worked", never merely "the config
// applied".
//
// The engine is machinery, not vocabulary (ADR-0038 §1). Data claims are
// the computation that decides the Observed leg of ADR-0004's cross: a
// Service's "expected traces never landed" *is* not_delivered, and the
// Expectation is why the evaluator knew to look. Pipeline claims join the
// Tier-attached finding family of ADR-0030/0035. No eighth outcome, no
// fourth reading: surfaces show outcomes and findings *sourced from*
// expectations.
//
// Three claim kinds, derived literal-only (ADR-0038 §2): arrival (per
// (Service, Environment, signal): the signal should land, derived from
// the Service's Paths through the rendered pipelines), enrichment (per
// (Service, Environment): attributes the rendered config explicitly,
// literally inserts should be present on landed telemetry), and
// self-telemetry (per Tier: each instantiated component should emit its
// own telemetry under R-4's join keys). The principle: the engine claims
// only what it can read off the artefact at a SHA, never what it believes
// about component semantics. Anything requiring knowledge of a component's
// runtime behaviour (k8sattributes, resourcedetection) yields no claim, and
// therefore unknown, never red. The curated behaviour-model layer is the
// named post-v1 seam (OQ-18), refused here: a false expectation-red would
// poison trust in the whole band.
//
// Derivation runs at evaluation time as a pure function of the artefact
// (ADR-0038 §3): no expectations file is ever committed, because a materialised
// copy is a drift surface against the artefact it restates. Memoisation
// (Memo) is in-memory, keyed by SHA, and confirmed loseable. The
// render-in-PR check displays the expectation diff (Diff) impact-report
// style: computed twice, stored never.
package expectation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/selftelemetry"
)

// Kind is one of the three claim kinds (ADR-0038 §2). The vocabulary is
// closed in v1: behaviour-model claims are the refused OQ-18 seam.
type Kind string

const (
	// Arrival: the signal should land for the Service in the Environment,
	// derived from the Service's Paths through the rendered pipelines.
	// Feeds not_delivered, the Observed leg of the cross (ADR-0004).
	Arrival Kind = "arrival"

	// Enrichment: an attribute the rendered config explicitly, literally
	// inserts or upserts should be present on landed telemetry. Only
	// static resource/attributes/transform actions with constant values
	// derive one; component runtime behaviour never does.
	Enrichment Kind = "enrichment"

	// SelfTelemetry: an instantiated component should emit its own
	// telemetry under R-4's join keys: the pipeline claim family, judged
	// per Tier.
	SelfTelemetry Kind = "self_telemetry"
)

// Shape is the self-telemetry join shape a pipeline claim expects. R-4's
// caveats are modelled as expected shapes, never as failures (ADR-0038
// §2c).
type Shape string

const (
	// ShapeIdentified: the component reports its rendered id in its join
	// keys, exactly as the YAML spells it.
	ShapeIdentified Shape = "identified"

	// ShapeUnidentified: an identity-dropping singleton (R-4 §5.2): the
	// component deliberately reports only its kind, and the upstream RFC
	// blesses the pattern. Expecting the id would model absence as
	// failure.
	ShapeUnidentified Shape = "unidentified"
)

// Claim is the unit of an Expectation: one checkable assertion derived
// from the Intended config at a commit SHA. Data claims (arrival,
// enrichment) key per (Service, Environment); pipeline claims
// (self-telemetry) key per Tier.
type Claim struct {
	Kind Kind `json:"kind"`

	// SHA is the commit the claim derives from. Claims are judged against
	// the artefact the collector reports running, the stamped SHA, never
	// head (ADR-0038 §4a).
	SHA string `json:"sha"`

	// Service and Environment key a data claim; empty on pipeline claims.
	// Service is the team-qualified Service id.
	Service     string `json:"service,omitempty"`
	Environment string `json:"environment,omitempty"`

	// Signal is the signal a data claim is about.
	Signal requirements.SignalKind `json:"signal,omitempty"`

	// Attribute and Value carry an enrichment claim's literal insertion.
	// Presence of the attribute is what the seam can check; the value is
	// the human-facing statement of what the config writes.
	Attribute string `json:"attribute,omitempty"`
	Value     string `json:"value,omitempty"`

	// Tier keys a pipeline claim (team-qualified Tier id); on data claims,
	// Tiers lists the Tiers whose artefacts back the claim, the context a
	// Service-attached finding attaches, because helper metrics are
	// component totals and the engine cannot honestly localise where on
	// the Path the data died (ADR-0038 §5b).
	Tier  string   `json:"tier,omitempty"`
	Tiers []string `json:"tiers,omitempty"`

	// Component is a pipeline claim's rendered component id (`type` or
	// `type/name`, exactly as the YAML spells it); ComponentKind its
	// self-telemetry kind; Shape the join shape R-4 says to expect.
	Component     string             `json:"component,omitempty"`
	ComponentKind selftelemetry.Kind `json:"component_kind,omitempty"`
	Shape         Shape              `json:"shape,omitempty"`
}

// Key is the claim's stable identity: what memoisation, dampening and
// diffing key on. Two derivations of the same artefact produce the same
// keys in the same order.
func (c Claim) Key() string {
	switch c.Kind {
	case Arrival:
		return fmt.Sprintf("arrival|%s|%s|%s", c.Service, c.Environment, c.Signal)
	case Enrichment:
		return fmt.Sprintf("enrichment|%s|%s|%s|%s", c.Service, c.Environment, c.Signal, c.Attribute)
	default:
		return fmt.Sprintf("self_telemetry|%s|%s|%s", c.Tier, c.ComponentKind, c.Component)
	}
}

// String renders the claim as the impact-report line the render-in-PR
// check displays (ADR-0038 §3, ADR-0020 precedent).
func (c Claim) String() string {
	switch c.Kind {
	case Arrival:
		return fmt.Sprintf("arrival claim: %s should land for %s in %s (via %s)",
			c.Signal, c.Service, c.Environment, strings.Join(c.Tiers, ", "))
	case Enrichment:
		return fmt.Sprintf("enrichment claim: %s records for %s in %s should carry %s=%q (via %s)",
			c.Signal, c.Service, c.Environment, c.Attribute, c.Value, strings.Join(c.Tiers, ", "))
	default:
		return fmt.Sprintf("self-telemetry claim: %s %s on %s should emit its own telemetry",
			c.ComponentKind, c.Component, c.Tier)
	}
}

// Set is one derived Expectation: every claim the Intended config at one
// SHA makes, in stable Key order. A Set is derived, never authored, and
// never persisted (ADR-0038 §3), so treat it as read-only.
type Set struct {
	SHA    string  `json:"sha"`
	Claims []Claim `json:"claims"`
}

// ForTier returns the pipeline claims attached to one Tier, in stable
// order.
func (s Set) ForTier(tier string) []Claim {
	var out []Claim
	for _, c := range s.Claims {
		if c.Kind == SelfTelemetry && c.Tier == tier {
			out = append(out, c)
		}
	}
	return out
}

// ForRow returns the data claims for one (Service, Environment) row, in
// stable order, the evaluation unit of ADR-0033.
func (s Set) ForRow(service, environment string) []Claim {
	var out []Claim
	for _, c := range s.Claims {
		if c.Kind != SelfTelemetry && c.Service == service && c.Environment == environment {
			out = append(out, c)
		}
	}
	return out
}

// RowAttributes returns the enrichment attribute names claimed for one
// row and signal, sorted: what the caller passes to a
// TelemetryProvider's Observe so the reading measures the claims'
// coverage in the same round trip.
func (s Set) RowAttributes(service, environment string, kind requirements.SignalKind) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range s.ForRow(service, environment) {
		if c.Kind == Enrichment && c.Signal == kind && !seen[c.Attribute] {
			seen[c.Attribute] = true
			out = append(out, c.Attribute)
		}
	}
	sort.Strings(out)
	return out
}

// Delta is the expectation diff between two derivations, what the
// render-in-PR check displays ("this change adds an arrival claim for
// traces"), computed twice, stored never (ADR-0038 §3).
type Delta struct {
	Added   []Claim
	Removed []Claim
}

// Empty reports a diff with nothing in it.
func (d Delta) Empty() bool { return len(d.Added) == 0 && len(d.Removed) == 0 }

// Diff compares two derived Sets by claim identity. Values changing under
// a stable key (an enrichment claim's literal value edited in place)
// surface as a remove-add pair, because the claim asserted before is not
// the claim asserted after.
func Diff(before, after Set) Delta {
	old := map[string]Claim{}
	for _, c := range before.Claims {
		old[identity(c)] = c
	}
	var d Delta
	seen := map[string]bool{}
	for _, c := range after.Claims {
		id := identity(c)
		seen[id] = true
		if _, ok := old[id]; !ok {
			d.Added = append(d.Added, c)
		}
	}
	for _, c := range before.Claims {
		if !seen[identity(c)] {
			d.Removed = append(d.Removed, c)
		}
	}
	return d
}

// identity is claim identity for diffing: the Key plus the asserted value
// and shape, without the SHA, because the SHA is which derivation, not what is
// claimed.
func identity(c Claim) string {
	return c.Key() + "|" + c.Value + "|" + string(c.Shape)
}
