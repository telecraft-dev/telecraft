package conformance

import (
	"strings"

	"github.com/telecraft-dev/telecraft/internal/requirements"
)

// Effective is one row's Effective reading: the collector's own reported
// running config — OpAMP's EffectiveConfig, adopted verbatim (ADR-0004,
// names per ADR-0015). Never what an applier holds, never what a ConfigMap
// contains; one definition for served and foreign collectors alike.
//
// The reading carries pipelines with component order preserved, not a flat
// component list (ADR-0004): `has_receiver: [filelog]` wired only into a
// traces pipeline is exactly the broken_pipeline case the product exists to
// catch, and a flat list could not represent it. Config assertions carry no
// pipeline scope yet, so a bare assertion matches any pipeline — the
// documented migration path.
type Effective struct {
	// Known keeps "we cannot see this collector's config" distinct from "it
	// reports an empty config" (ADR-0008). When false, Cause says why and
	// Pipelines means nothing.
	Known bool
	Cause string

	Pipelines []Pipeline
}

// Pipeline is one otelcol pipeline as the collector reports it, components
// in configured order.
type Pipeline struct {
	// Name is the otelcol pipeline id: the signal, optionally qualified —
	// "logs", "traces/backend".
	Name string `yaml:"name"`

	Receivers  []string `yaml:"receivers"`
	Processors []string `yaml:"processors"`
	Exporters  []string `yaml:"exporters"`
}

// SatisfiesConfig judges this reading against one config assertion — the
// same checker the cross uses on the Effective reading, exposed so the
// drift detection can run it against the Intended one (ADR-0004: "same
// checker as the declared config assertion, run against the intended
// YAML"). The reading must be Known; an unavailable reading has nothing to
// judge and belongs to the cross's evaluability rules, not this method.
func (e Effective) SatisfiesConfig(a *requirements.ConfigAssertion) (bool, []string) {
	ok, _, detail := checkConfig(a, e)
	return ok, detail
}

// componentsOf unions one component slot across every pipeline — the
// any-pipeline default for bare config assertions (ADR-0004).
func (e Effective) componentsOf(slot func(Pipeline) []string) []string {
	var out []string
	for _, p := range e.Pipelines {
		out = append(out, slot(p)...)
	}
	return out
}

// anyPresent reports whether any wanted component type appears in have.
//
// Collector components are named "type" or "type/name", and a requirement
// asks for a type. Matching on the type prefix means "otlp/onramp" satisfies
// a requirement for "otlp", which is what anyone writing the requirement
// meant.
func anyPresent(want, have []string) bool {
	for _, w := range want {
		for _, h := range have {
			if h == w || strings.HasPrefix(h, w+"/") {
				return true
			}
		}
	}
	return false
}
