package console

import (
	"testing"

	"github.com/telecraft-dev/telecraft/internal/conformance"
	"github.com/telecraft-dev/telecraft/internal/requirements"
)

// A schema-conformance finding writes its own remediation out of the Schema
// Registry, and that is the one a reader gets. Falling back to the authored
// line here would hand back the generic text the registry-derived one exists
// to replace (ADR-0034 §7).
func TestConformanceRemediationPrefersTheEvaluatorsOwn(t *testing.T) {
	derived := `Emit the attributes the Schema Registry demands at required: span group span.db.client demands "db.namespace" (string).`
	f := conformance.Finding{
		Requirement: requirements.Requirement{Remediation: "adopt the semantic conventions"},
		Remediation: derived,
	}
	if got := conformanceRemediation(f); got != derived {
		t.Errorf("remediation = %q, want the registry-derived line", got)
	}
}

// Every other requirement kind asserts one fixed thing, so its author can
// say what closes it. A finding without remediation is a complaint, so the
// authored line is what stops the drawer showing one.
func TestConformanceRemediationFallsBackToTheAuthoredLine(t *testing.T) {
	f := conformance.Finding{
		Requirement: requirements.Requirement{Remediation: "add an otlp receiver to the gateway"},
	}
	if got := conformanceRemediation(f); got != "add an otlp receiver to the gateway" {
		t.Errorf("remediation = %q, want the requirement's authored line", got)
	}
}
