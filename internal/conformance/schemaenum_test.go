package conformance

import (
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
	"github.com/telecraft-dev/telecraft/pkg/ownership"
)

// dbSpans is the scope every test here judges: span.db.client, which demands
// the two enum attributes the fixture registry declares members for.
func dbSpans() requirements.Scope {
	return requirements.Scope{Groups: []string{"span.db.client"}}
}

// withValues replaces the value-set reading for one attribute on the traces
// leg, so a test can say what the telemetry carried.
func withValues(ev Evidence, attribute string, reading telemetry.DistinctValues) Evidence {
	ev.Schema.Values[SchemaValueReading{Kind: requirements.Traces, Window: schemaWindow, Attribute: attribute}] = reading
	return ev
}

// An attribute carrying a value the registry does not declare is a breach.
// Before this check existed the attribute was present, and present read as
// clean: the presence check asks whether the name is there and never what it
// holds (ADR-0034 §4).
func TestSchemaUndeclaredEnumValueIsAViolation(t *testing.T) {
	ev := withValues(schemaEvidence(t, requirements.Traces, conformingSpan()...), "db.system.name",
		telemetry.DistinctValues{
			Known: true, Window: schemaWindow, Attribute: "db.system.name",
			Values: []string{"cassandra", "postgresql"}, Cap: telemetry.MaxDistinctValues,
		})

	v := evaluateSchema(t, schemaRequirement(dbSpans()), ev)
	f := findingAt(t, v, schemaregistry.Required)

	if f.Outcome != Misconfigured {
		t.Errorf("outcome = %q, want %q: the telemetry arrived and carries a value nobody declared (detail: %v)", f.Outcome, Misconfigured, f.Detail)
	}
	if f.Weight() != ownership.Violation || !f.Failing() {
		t.Errorf("grade = %q, failing = %v: db.system.name is demanded at required, so its breach is a violation", f.Weight(), f.Failing())
	}
	if !detailNames(f, "cassandra") {
		t.Errorf("the detail does not name the undeclared value: %v", f.Detail)
	}
	if !detailNames(f, "mariadb") {
		t.Errorf("the detail does not name what the registry does declare: %v", f.Detail)
	}
	if !strings.Contains(f.Remediation, "registry group registry.db") {
		t.Errorf("the remediation does not send the reader to the group that declares the members: %q", f.Remediation)
	}
	if v.Worst() != Misconfigured {
		t.Errorf("Worst() = %q, want %q", v.Worst(), Misconfigured)
	}
}

// A value the reading returned is in the telemetry whether or not the reading
// was whole, so a breach found in a truncated reading stands: presence is
// proof in the value dimension as in the name dimension.
func TestSchemaUndeclaredEnumValueInATruncatedReadingStillCounts(t *testing.T) {
	ev := withValues(schemaEvidence(t, requirements.Traces, conformingSpan()...), "db.system.name",
		telemetry.DistinctValues{
			Known: true, Window: schemaWindow, Attribute: "db.system.name",
			Values: []string{"cassandra"}, Truncated: true, Cap: telemetry.MaxDistinctValues,
		})

	f := findingAt(t, evaluateSchema(t, schemaRequirement(dbSpans()), ev), schemaregistry.Required)
	if f.Outcome != Misconfigured {
		t.Errorf("outcome = %q, want %q: a clipped reading still found the value (detail: %v)", f.Outcome, Misconfigured, f.Detail)
	}
	for _, d := range f.Detail {
		if strings.Contains(d, "clipped set cannot prove") {
			t.Errorf("the truncation is reported as the story when a breach was found: %q", d)
		}
	}
}

// The other direction, which the name check does not have: a clipped value
// set has its violations exactly where it stopped looking, so a reading that
// found nothing undeclared has proved nothing. Unknown, never a pass.
func TestSchemaTruncatedCleanEnumReadingIsUnknown(t *testing.T) {
	ev := withValues(schemaEvidence(t, requirements.Traces, conformingSpan()...), "db.system.name",
		telemetry.DistinctValues{
			Known: true, Window: schemaWindow, Attribute: "db.system.name",
			Values: []string{"postgresql"}, Truncated: true, Cap: telemetry.MaxDistinctValues,
		})

	f := findingAt(t, evaluateSchema(t, schemaRequirement(dbSpans()), ev), schemaregistry.Required)
	if f.Outcome != Unknown {
		t.Errorf("outcome = %q, want %q: a clipped set cannot prove the values it names are the only ones (detail: %v)", f.Outcome, Unknown, f.Detail)
	}
	if f.Outcome.Passing() {
		t.Error("a clipped value set passes the enum check")
	}
	if !detailNames(f, "truncated") {
		t.Errorf("the detail does not report the truncation: %v", f.Detail)
	}
	if f.Remediation == "" {
		t.Error("an unknown verdict with no fix is a complaint")
	}
}

// A value set nobody read is unknown too. An enum attribute in use whose
// values nobody looked at must not read as an enum nobody violated.
func TestSchemaUnreadValueSetIsUnknown(t *testing.T) {
	ev := schemaEvidence(t, requirements.Traces, conformingSpan()...)
	delete(ev.Schema.Values, SchemaValueReading{Kind: requirements.Traces, Window: schemaWindow, Attribute: "db.system.name"})

	f := findingAt(t, evaluateSchema(t, schemaRequirement(dbSpans()), ev), schemaregistry.Required)
	if f.Outcome != Unknown {
		t.Errorf("outcome = %q, want %q (detail: %v)", f.Outcome, Unknown, f.Detail)
	}
	if !detailNames(f, "no value-set reading covers") {
		t.Errorf("the detail does not name the reading it wanted: %v", f.Detail)
	}
}

// A provider that could not take the reading keeps its cause, exactly as an
// unreadable attribute-name reading does (ADR-0008).
func TestSchemaUnreadableValueSetKeepsItsCause(t *testing.T) {
	ev := withValues(schemaEvidence(t, requirements.Traces, conformingSpan()...), "db.system.name",
		telemetry.DistinctValuesUnknown(time.Now(), schemaWindow, "db.system.name",
			telemetry.NotServiceScoped(telemetry.Service{Name: "checkout", Environment: "production"}, "the index holds no service dimension")))

	f := findingAt(t, evaluateSchema(t, schemaRequirement(dbSpans()), ev), schemaregistry.Required)
	if f.Outcome != Unknown {
		t.Errorf("outcome = %q, want %q", f.Outcome, Unknown)
	}
	if !detailNames(f, "cannot scope this reading") {
		t.Errorf("the provider's cause is not preserved: %v", f.Detail)
	}
}

// The level mapping is reused rather than extended (#157, ADR-0034 §3): an
// undeclared value on an attribute the registry only recommends is an
// improvement, and improvements never move the binary.
func TestSchemaEnumBreachTakesTheAttributesDeclaredLevel(t *testing.T) {
	// registry.db declares db.system.name with no level, so the namespace
	// scope alone demands it at the default, recommended.
	req := schemaRequirement(requirements.Scope{Groups: []string{"registry.db"}})
	ev := withValues(schemaEvidence(t, requirements.Traces, "db.namespace", "db.operation.name", "db.system.name", "db.connection_string"),
		"db.system.name", telemetry.DistinctValues{
			Known: true, Window: schemaWindow, Attribute: "db.system.name",
			Values: []string{"cassandra"}, Cap: telemetry.MaxDistinctValues,
		})

	v := evaluateSchema(t, req, ev)
	f := findingAt(t, v, schemaregistry.Recommended)
	if f.Weight() != ownership.Advisory {
		t.Errorf("grade = %q, want %q: the attribute is demanded at recommended", f.Weight(), ownership.Advisory)
	}
	if f.Failing() {
		t.Error("an improvement-grade enum breach fails the row, so the binary is not violations alone")
	}
	if !detailNames(f, "cassandra") {
		t.Errorf("the detail does not name the undeclared value: %v", f.Detail)
	}
	if v.Worst() != Compliant {
		t.Errorf("Worst() = %q, want %q: a recommended breach never darkens the badge", v.Worst(), Compliant)
	}
	// Coverage stays presence coverage: every recommended attribute is in
	// use, and an enum breach is a different fact about one of them.
	// Folding it into the ratio would make "3 of 4" mean two things.
	if got, ok := f.CoverageRatio(); !ok || got != 1 {
		t.Errorf("coverage = (%v, %v), want (1, true): every recommended attribute is in use", got, ok)
	}
}

// The check is offered only for attributes the registry declares as enums,
// which is the caller-side constraint ADR-0034 §4 puts on the primitive, and
// only for the ones the names reading found in use. Everything else buys no
// round trip.
func TestSchemaValuePlanIsEnumsInUseOnly(t *testing.T) {
	lib := fixtureLibrary(t)
	names := map[SchemaReading][]string{}
	for _, key := range SchemaReadings(lib, "production") {
		names[key] = conformingSpan()
	}
	inUse := func(key SchemaReading, attribute string) bool {
		for _, n := range names[key] {
			if n == attribute {
				return true
			}
		}
		return false
	}

	plan := SchemaValueReadings(lib, "production", inUse)
	if len(plan) == 0 {
		t.Fatal("the fixture library plans no value readings, so this proves nothing")
	}
	for _, key := range plan {
		if key.Attribute != "db.system.name" && key.Attribute != "enterprise.criticality_tier" {
			t.Errorf("planned a value reading for %v, which the registry does not declare as an enum", key)
		}
	}

	// Nothing in use buys nothing: the presence gate is what bounds the
	// cost of one round trip per enum attribute per covered signal.
	if got := SchemaValueReadings(lib, "production", func(SchemaReading, string) bool { return false }); len(got) != 0 {
		t.Errorf("planned %v for a Service that sets none of them", got)
	}
}
