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

// A finding marked Advice never decides a band: improvement and information
// findings ride alongside the verdict, and the band state, with every ratio
// and worst-severity roll-up built from face fields, is decided by
// violations alone (ADR-0034 §3). A violation beside the advice still
// decides it, so riding alongside hides nothing.
func TestBandForKeepsAdviceOutOfTheBinary(t *testing.T) {
	empty := Band{State: BandOK, WorstSeverity: SeverityNone}
	advice := Finding{Kind: "conformance", Severity: SeverityAdvisory, Dampening: "none", Advice: true}

	if band := bandFor([]Finding{advice}, "conformance", empty); band != empty {
		t.Errorf("band = %+v, want it untouched: an advisory advice finding moved the binary", band)
	}

	violation := Finding{Kind: "conformance", Severity: SeverityViolation, Dampening: "none", Summary: "s"}
	band := bandFor([]Finding{advice, violation}, "conformance", empty)
	if band.State != BandFinding || band.WorstSeverity != SeverityViolation {
		t.Errorf("band = %+v, want the violation to decide it", band)
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
