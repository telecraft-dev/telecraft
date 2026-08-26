package conformance

import (
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/ownership"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
)

// activeRef is the ref the drifted fixture version stands at: one ahead of
// the snapshotRef the requirements here pin.
const activeRef = "v1.5.0"

// driftedRegistry is the fixture registry a version on: span.db.client
// additionally demands enterprise.owner_email at required. The attribute's
// definition carries the fixture's own deprecation notice, so a finding
// naming it has a migration note to carry (ADR-0034 §7).
func driftedRegistry(t *testing.T) *schemaregistry.Registry {
	t.Helper()
	reg := registry(t)
	reg.Source.Ref = activeRef
	for i, g := range reg.Groups {
		if g.ID != "span.db.client" {
			continue
		}
		reg.Groups[i].Attributes = append(reg.Groups[i].Attributes, schemaregistry.Attribute{
			Ref:   "enterprise.owner_email",
			Level: schemaregistry.Required,
		})
		return reg
	}
	t.Fatal("the fixture registry has no span.db.client group")
	return nil
}

// driftEvidence is schemaEvidence with the drifted version standing active:
// filed under its own ref, named as the active one.
func driftEvidence(t *testing.T, inUse ...string) Evidence {
	t.Helper()
	ev := schemaEvidence(t, requirements.Traces, inUse...)
	ev.Schema.Versions[activeRef] = driftedRegistry(t)
	ev.Schema.ActiveVersion = activeRef
	return ev
}

// The clause itself: a scope passing the version its requirement pins and
// failing the active one is library_drift with the registry facet, no new
// finding kind and no new outcome (ADR-0034 §2, ADR-0026 §7).
func TestSchemaDriftIsRaisedWherePinPassesAndActiveFails(t *testing.T) {
	v := evaluateSchema(t, schemaRequirement(dbSpans()), driftEvidence(t, conformingSpan()...))

	f, found := findingFor(v, "db-spans-conform")
	if !found {
		t.Fatalf("no violation-grade finding in %v", details(v))
	}
	if f.Outcome != LibraryDrift {
		t.Fatalf("outcome = %q, want %q (detail: %v)", f.Outcome, LibraryDrift, f.Detail)
	}
	if f.Facet != FacetRegistry {
		t.Errorf("facet = %q, want %q", f.Facet, FacetRegistry)
	}
	if f.Weight() != ownership.Violation || !f.Failing() {
		t.Errorf("grade = %q, failing = %v: drift is counted in the roll-ups", f.Weight(), f.Failing())
	}
	if v.Worst() != LibraryDrift {
		t.Errorf("Worst() = %q, want %q", v.Worst(), LibraryDrift)
	}
	if !detailNames(f, snapshotRef) || !detailNames(f, activeRef) {
		t.Errorf("the detail does not name both versions: %v", f.Detail)
	}
	if !detailNames(f, "enterprise.owner_email") {
		t.Errorf("the detail does not name the attribute the active version demands: %v", f.Detail)
	}
	if len(f.Missing) != 1 || f.Missing[0] != "enterprise.owner_email" {
		t.Errorf("Missing = %v, want the newly demanded attribute", f.Missing)
	}
}

// The remediation is the component-facet precedent in section 7's shape: the
// registry-derived gap, the upstream migration note where the registry
// carries one, and the pin move, in the order a reader can act on them.
func TestSchemaDriftRemediationNamesTheNewerVersionAndTheMigrationNote(t *testing.T) {
	v := evaluateSchema(t, schemaRequirement(dbSpans()), driftEvidence(t, conformingSpan()...))

	f, found := findingFor(v, "db-spans-conform")
	if !found {
		t.Fatalf("no violation-grade finding in %v", details(v))
	}
	for _, want := range []string{
		"enterprise.owner_email",
		"deprecated",
		"Replaced by the ownership tree",
		"pin from Schema Registry version " + snapshotRef + " to " + activeRef,
	} {
		if !strings.Contains(f.Remediation, want) {
			t.Errorf("remediation does not carry %q: %q", want, f.Remediation)
		}
	}
	if !strings.Contains(f.Remediation, "instrumentation change") {
		t.Errorf("remediation does not keep the fix with the party who can make it: %q", f.Remediation)
	}
}

// A tracking reference never raises the registry facet: it is judged against
// the active version already, so there is no pin to fall behind (ADR-0026
// §1). Failing it is the ordinary failure.
func TestATrackingReferenceNeverRaisesTheRegistryFacet(t *testing.T) {
	req := schemaRequirement(dbSpans())
	req.Schema.RegistryVersion = ""
	req.Schema.Track = requirements.TrackHead

	ev := driftEvidence(t, conformingSpan()...)
	ev.Schema.Versions[requirements.TrackHead] = ev.Schema.Versions[activeRef]

	v := evaluateSchema(t, req, ev)
	f, found := findingFor(v, "db-spans-conform")
	if !found {
		t.Fatalf("no violation-grade finding in %v", details(v))
	}
	if f.Outcome != Misconfigured {
		t.Errorf("outcome = %q, want %q: a tracking reference failing the active version never complied", f.Outcome, Misconfigured)
	}
	if f.Facet != "" {
		t.Errorf("facet = %q, want none", f.Facet)
	}
}

// A Service failing its pin keeps the existing diagnosis, unchanged: fails
// both is the ordinary failure, never drift (ADR-0026 §6).
func TestAServiceFailingItsPinKeepsTheExistingDiagnosis(t *testing.T) {
	// server.address is demanded at required by the pinned version too, so
	// this scope fails both versions.
	inUse := []string{"db.namespace", "db.operation.name", "db.system.name", "enterprise.criticality_tier", "server.port"}
	v := evaluateSchema(t, schemaRequirement(dbSpans()), driftEvidence(t, inUse...))

	f, found := findingFor(v, "db-spans-conform")
	if !found {
		t.Fatalf("no violation-grade finding in %v", details(v))
	}
	if f.Outcome != Misconfigured {
		t.Errorf("outcome = %q, want %q", f.Outcome, Misconfigured)
	}
	if f.Facet != "" {
		t.Errorf("facet = %q, want none: failing the pin is not drift", f.Facet)
	}
}

// Passing both versions is compliant: nothing has fallen behind anything.
func TestSchemaDriftPassesBothVersionsStaysCompliant(t *testing.T) {
	inUse := append(conformingSpan(), "enterprise.owner_email")
	v := evaluateSchema(t, schemaRequirement(dbSpans()), driftEvidence(t, inUse...))

	f, found := findingFor(v, "db-spans-conform")
	if !found {
		t.Fatalf("no violation-grade finding in %v", details(v))
	}
	if f.Outcome != Compliant || f.Facet != "" {
		t.Errorf("finding = (%q, facet %q), want (compliant, none)", f.Outcome, f.Facet)
	}
}

// Told no active version, the drift arm judges nothing: which version is
// active is an activation decision, never this package's guess.
func TestSchemaDriftIsNotJudgedWithoutADesignation(t *testing.T) {
	v := evaluateSchema(t, schemaRequirement(dbSpans()), schemaEvidence(t, requirements.Traces, conformingSpan()...))

	f, found := findingFor(v, "db-spans-conform")
	if !found {
		t.Fatalf("no violation-grade finding in %v", details(v))
	}
	if f.Outcome != Compliant || f.Facet != "" {
		t.Errorf("finding = (%q, facet %q), want (compliant, none)", f.Outcome, f.Facet)
	}
}

// A pin standing at the active version has nothing to fall behind.
func TestSchemaDriftIsNotJudgedWhereThePinIsTheActiveVersion(t *testing.T) {
	ev := schemaEvidence(t, requirements.Traces, conformingSpan()...)
	ev.Schema.ActiveVersion = snapshotRef

	v := evaluateSchema(t, schemaRequirement(dbSpans()), ev)
	f, found := findingFor(v, "db-spans-conform")
	if !found {
		t.Fatalf("no violation-grade finding in %v", details(v))
	}
	if f.Outcome != Compliant || f.Facet != "" {
		t.Errorf("finding = (%q, facet %q), want (compliant, none)", f.Outcome, f.Facet)
	}
}

// An active verdict nobody could prove raises nothing: a truncated reading
// cannot tell the newly demanded attribute absent, so the active judgement
// is unknown and calling it drift would manufacture a red out of a blind
// spot (ADR-0008). The pinned verdict stands.
func TestSchemaDriftUnprovableActiveFailureRaisesNothing(t *testing.T) {
	ev := driftEvidence(t, conformingSpan()...)
	key := SchemaReading{Kind: requirements.Traces, Window: schemaWindow}
	reading := ev.Schema.Names[key]
	reading.Truncated, reading.SampledRecords, reading.TotalRecords = true, 500, 90000
	ev.Schema.Names[key] = reading

	v := evaluateSchema(t, schemaRequirement(dbSpans()), ev)
	f, found := findingFor(v, "db-spans-conform")
	if !found {
		t.Fatalf("no violation-grade finding in %v", details(v))
	}
	if f.Outcome != Compliant || f.Facet != "" {
		t.Errorf("finding = (%q, facet %q), want (compliant, none): presence proved the pin met, and absence off a truncated reading proves nothing", f.Outcome, f.Facet)
	}
}

// The improvement and information findings ride alongside the drift verdict
// exactly as they ride alongside a compliant one: the drift arm rewrites the
// scored verdict and touches nothing else.
func TestSchemaDriftLeavesTheAdviceFindingsAlone(t *testing.T) {
	// server.port, the group's one recommended attribute, is not in use, so
	// a recommended finding rides alongside whatever the verdict says.
	inUse := []string{"db.namespace", "db.operation.name", "db.system.name", "enterprise.criticality_tier", "server.address"}
	v := evaluateSchema(t, schemaRequirement(dbSpans()), driftEvidence(t, inUse...))

	f, found := findingFor(v, "db-spans-conform")
	if !found {
		t.Fatalf("no violation-grade finding in %v", details(v))
	}
	if f.Outcome != LibraryDrift {
		t.Fatalf("outcome = %q, want %q (detail: %v)", f.Outcome, LibraryDrift, f.Detail)
	}
	advice := findingAt(t, v, schemaregistry.Recommended)
	if advice.Facet != "" || advice.Outcome == LibraryDrift {
		t.Errorf("the recommended finding was rewritten: (%q, facet %q)", advice.Outcome, advice.Facet)
	}
	s := v.Score()
	if s.Failing != 1 || s.Advisory == 0 {
		t.Errorf("score = %+v, want the drift verdict counted once with the advice beside it", s)
	}
}
