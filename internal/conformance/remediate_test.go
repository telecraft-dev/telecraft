package conformance

import (
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/ownership"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
)

// authoredRemediation is what schemaRequirement puts on the requirement: the
// generic line a schema requirement's author can write, and the line these
// tests exist to prove the finding does not settle for.
const authoredRemediation = "instrument the missing attributes"

// contains fails with the whole text, because a remediation is prose and a
// diff of one word is unreadable without it.
func contains(t *testing.T, remediation, want, why string) {
	t.Helper()
	if !strings.Contains(remediation, want) {
		t.Errorf("the remediation does not name %s (%s):\n%s", want, why, remediation)
	}
}

// The registry is where remediation comes from, not the requirement file: a
// schema requirement names a scope, so what closes the gap is whatever the
// registry version declares for it (ADR-0034 §7).
func TestSchemaRemediationIsRegistryDerived(t *testing.T) {
	ev := schemaEvidence(t, requirements.Traces,
		"db.namespace", "db.operation.name", "db.system.name", "server.port")
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}), ev)

	f := findingAt(t, v, schemaregistry.Required)
	if f.Remediation == "" {
		t.Fatal("the required finding carries no remediation, so a reader is left with the authored line")
	}
	if f.Remediation == authoredRemediation {
		t.Fatal("the required finding fell back to the authored line, which cannot name what the registry demanded")
	}

	contains(t, f.Remediation, "span group span.db.client", "the group that demanded the attribute is where a level is changed")
	contains(t, f.Remediation, `"enterprise.criticality_tier"`, "the missing attribute")
	contains(t, f.Remediation, "(enum)", "the type the registry declares the attribute at")
	contains(t, f.Remediation, "required", "the level the registry demands it at")
	contains(t, f.Remediation, "instrumentation change", "the fix is instrumentation, not collector config")
}

// An attribute this registry only references is defined in a registry it
// imports from, which is not in the adopter's tree. Saying so is the honest
// answer; inventing a type would not be.
func TestSchemaRemediationSaysWhenTheTypeIsNotDeclaredHere(t *testing.T) {
	ev := schemaEvidence(t, requirements.Traces,
		"db.namespace", "db.operation.name", "db.system.name", "enterprise.criticality_tier")
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}), ev)

	f := findingAt(t, v, schemaregistry.Required)
	contains(t, f.Remediation, `"server.address"`, "the missing attribute")
	contains(t, f.Remediation, "type not declared in this registry version", "the type lives in a dependency registry")
}

// Upstream already wrote the migration instruction, so the finding hands it
// over rather than paraphrasing it (ADR-0034 §7).
func TestSchemaRemediationCarriesTheDeprecationNote(t *testing.T) {
	ev := schemaEvidence(t, requirements.Traces, "db.namespace")
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"registry.db"}}), ev)

	f := findingAt(t, v, schemaregistry.Recommended)
	contains(t, f.Remediation, `marks "db.connection_string" deprecated`, "the attribute the registry deprecated")
	contains(t, f.Remediation, "(obsoleted)", "the machine-readable reason")
	contains(t, f.Remediation, `renamed to "db.namespace"`, "the machine-readable migration target")
	contains(t, f.Remediation, "Carries credentials", "upstream's own note")
}

// The prose form of the notice reads the same way as the structured one:
// both land in Note, and neither is dropped.
func TestSchemaRemediationCarriesAProseDeprecationNote(t *testing.T) {
	ev := schemaEvidence(t, requirements.Traces, "enterprise.criticality_tier")
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Namespaces: []string{"enterprise"}}), ev)

	f := findingAt(t, v, schemaregistry.Recommended)
	contains(t, f.Remediation, `marks "enterprise.owner_email" deprecated`, "the deprecated attribute")
	contains(t, f.Remediation, "Replaced by the ownership tree", "upstream's own note")
}

// A gap on a resource or entity group may be enrichable at collection, and
// remediation may say so. What it must not do is split the finding in two or
// send it somewhere else: one finding, one owner (ADR-0034 §7).
func TestSchemaEnrichableGapSuggestsWithoutSplittingOrRerouting(t *testing.T) {
	ev := schemaEvidence(t, requirements.Traces, "enterprise.cost_centre")
	req := schemaRequirement(requirements.Scope{Groups: []string{"entity.service"}})
	v := evaluateSchema(t, req, ev)

	var required []Finding
	for _, f := range v.Findings {
		if f.Weight() == ownership.Violation {
			required = append(required, f)
		}
	}
	if len(required) != 1 {
		t.Fatalf("violation-grade findings = %d, want the one finding the miss produced: %v", len(required), details(v))
	}

	f := required[0]
	contains(t, f.Remediation, `"service.name"`, "the enrichable attribute")
	contains(t, f.Remediation, "k8sattributes", "the processor that could add it at collection")
	contains(t, f.Remediation, "does not move this finding", "enrichment is a suggestion and never a reroute")
	contains(t, f.Remediation, "instrumentation change", "the fix still belongs to the service")

	if f.Requirement.ID != req.ID {
		t.Errorf("the finding is attached to %q, want the requirement that produced it", f.Requirement.ID)
	}
}

// opt_in is an offer nobody took up. There is no gap, so there is nothing to
// enrich and nobody to send the work to.
func TestSchemaOptInRemediationSendsNobody(t *testing.T) {
	ev := schemaEvidence(t, requirements.Traces, "service.name")
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"entity.service"}}), ev)

	f := findingAt(t, v, schemaregistry.OptIn)
	contains(t, f.Remediation, "opt_in", "the level the registry offers it at")
	if strings.Contains(f.Remediation, "instrumentation change") {
		t.Errorf("an opt_in finding names an owner for work nobody asked for:\n%s", f.Remediation)
	}
	if strings.Contains(f.Remediation, "k8sattributes") {
		t.Errorf("an opt_in finding suggests enrichment for a non-gap:\n%s", f.Remediation)
	}
}

// A finding nobody could judge still carries a fix, and the fix is about the
// evaluation rather than the instrumentation: nothing here says the
// telemetry is the wrong shape.
func TestSchemaUnknownRemediationNamesTheEvaluation(t *testing.T) {
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}), Evidence{})
	f := findingAt(t, v, schemaregistry.Required)

	if f.Outcome != Unknown {
		t.Fatalf("outcome = %q, want %q when no registry version is available", f.Outcome, Unknown)
	}
	contains(t, f.Remediation, "Import Schema Registry version", "the fix is to install the version the requirement pins")
	if strings.Contains(f.Remediation, "instrumentation change") {
		t.Errorf("an unjudged requirement asks for instrumentation on evidence nobody saw:\n%s", f.Remediation)
	}
}

// A signal that never arrived has no shape to judge, so the fix is delivery.
func TestSchemaNotDeliveredRemediationNamesDelivery(t *testing.T) {
	ev := schemaEvidence(t, requirements.Traces)
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}), ev)

	f := findingAt(t, v, schemaregistry.Required)
	if f.Outcome != NotDelivered {
		t.Fatalf("outcome = %q, want %q when nothing arrived", f.Outcome, NotDelivered)
	}
	contains(t, f.Remediation, "Nothing arrived", "the reading, not the shape, is what failed")
}

// A conforming scope has nothing to fix, so it says nothing. A surface that
// needs a line falls back to the requirement's authored one.
func TestSchemaConformingFindingCarriesNoRemediation(t *testing.T) {
	ev := schemaEvidence(t, requirements.Traces, conformingSpan()...)
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}), ev)

	f := findingAt(t, v, schemaregistry.Required)
	if f.Outcome != Compliant {
		t.Fatalf("outcome = %q, want %q on a conforming scope", f.Outcome, Compliant)
	}
	if f.Remediation != "" {
		t.Errorf("a conforming finding carries a fix for a gap that is not there:\n%s", f.Remediation)
	}
}

// The cross's own findings keep the authored remediation: the requirement
// asserts one fixed thing, so its author can say what closes it.
func TestCrossFindingsWriteNoRemediation(t *testing.T) {
	req := requirements.Requirement{
		ID:          "traces-arrive",
		Version:     1,
		Level:       requirements.Required,
		Owner:       "platform-observability",
		Signal:      &requirements.SignalAssertion{Kind: requirements.Traces, Present: true, Window: requirements.Duration(schemaWindow)},
		Remediation: authoredRemediation,
	}
	v := evaluateSchema(t, req, Evidence{})

	if len(v.Findings) != 1 {
		t.Fatalf("findings = %d, want the one the cross produces", len(v.Findings))
	}
	if v.Findings[0].Remediation != "" {
		t.Errorf("a cross finding wrote its own remediation:\n%s", v.Findings[0].Remediation)
	}
}

// The demand carries what the finding has to say, resolved through a
// reference: a reference restates neither the type nor the deprecation, so
// both are read from the definition the registry holds.
func TestDemandResolvesTheDeclarationThroughAReference(t *testing.T) {
	reg := registry(t)
	byName := map[string]demand{}
	for _, d := range demandsOf(reg, requirements.Scope{Groups: []string{"span.db.client", "registry.db"}}) {
		byName[d.Attribute] = d
	}

	// db.system.name is referenced by span.db.client at required and
	// defined by registry.db as an enum. The level comes from the
	// reference, the type from the definition.
	d, ok := byName["db.system.name"]
	if !ok {
		t.Fatal("the scope demands db.system.name and the demand set does not carry it")
	}
	if d.Level != schemaregistry.Required {
		t.Errorf("level = %q, want the level the reference tightened to", d.Level)
	}
	if d.Type != "enum" {
		t.Errorf("type = %q, want the type the definition declares", d.Type)
	}
	if d.GroupKind != schemaregistry.Span {
		t.Errorf("group kind = %q, want the kind of the group that demanded it", d.GroupKind)
	}

	dep, ok := byName["db.connection_string"]
	if !ok {
		t.Fatal("the scope demands db.connection_string and the demand set does not carry it")
	}
	if dep.Deprecation == nil {
		t.Fatal("the demand drops the deprecation notice the registry declares")
	}
	if dep.Deprecation.RenamedTo != "db.namespace" {
		t.Errorf("renamed_to = %q, want the migration target the registry declares", dep.Deprecation.RenamedTo)
	}
}
