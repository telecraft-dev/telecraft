package activation

import (
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/conformance"
	"github.com/telecraft-dev/telecraft/internal/ownership"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
)

// registry builds one Schema Registry version through the artefact the
// import pipeline writes, for the same reason the Catalogue fixture does.
func registry(t *testing.T, ref string, groups ...schemaregistry.Group) *schemaregistry.Registry {
	t.Helper()
	r := &schemaregistry.Registry{
		FormatVersion: schemaregistry.FormatVersion,
		Source:        schemaregistry.Source{Repository: "example.com/registry", Ref: ref},
		Manifest:      schemaregistry.Manifest{Name: "example"},
		Groups:        groups,
	}
	path, _, err := r.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := schemaregistry.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func attributes(id string, attrs ...schemaregistry.Attribute) schemaregistry.Group {
	return schemaregistry.Group{
		ID:         id,
		Kind:       schemaregistry.AttributeGroup,
		File:       "model/" + id + ".yaml",
		Brief:      "the " + id + " group",
		Attributes: attrs,
	}
}

func defines(id string, level schemaregistry.Level) schemaregistry.Attribute {
	return schemaregistry.Attribute{ID: id, Type: "string", Level: level, Brief: "the " + id + " attribute"}
}

func TestTheVersionDiffReportsAdditionsRemovalsAndTightenings(t *testing.T) {
	from := registry(t, "v1.4.0", attributes("db",
		defines("db.namespace", schemaregistry.Recommended),
		defines("db.system", schemaregistry.Required),
		defines("db.statement", schemaregistry.OptIn)))
	to := registry(t, "v1.5.0", attributes("db",
		defines("db.namespace", schemaregistry.Required),
		defines("db.system", schemaregistry.Required),
		defines("db.operation", schemaregistry.Recommended)))

	rep, err := RegistryImpact(RegistryInputs{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findChange(rep, AttributeAdded, "db.operation"); !ok {
		t.Errorf("no addition reported: %+v", rep.Changes)
	}
	if _, ok := findChange(rep, AttributeRemoved, "db.statement"); !ok {
		t.Errorf("no removal reported: %+v", rep.Changes)
	}
	tightened, ok := findChange(rep, LevelTightened, "db.namespace")
	if !ok {
		t.Fatalf("no tightening reported: %+v", rep.Changes)
	}
	if !strings.Contains(tightened.Detail, "from recommended to required") {
		t.Errorf("tightening reads %q", tightened.Detail)
	}
	// db.system did not move, so nothing is said about it.
	if _, ok := findChange(rep, LevelTightened, "db.system"); ok {
		t.Error("an unchanged level was reported as a tightening")
	}
}

// A loosening is a change, and it is not a tightening. The report says what
// happened rather than putting every move under one word.
func TestALooseningIsNotReportedAsATightening(t *testing.T) {
	from := registry(t, "v1.4.0", attributes("db", defines("db.namespace", schemaregistry.Required)))
	to := registry(t, "v1.5.0", attributes("db", defines("db.namespace", schemaregistry.Recommended)))

	rep, err := RegistryImpact(RegistryInputs{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findChange(rep, LevelTightened, "db.namespace"); ok {
		t.Errorf("a loosening was reported as a tightening: %+v", rep.Changes)
	}
}

func TestNewlyDeprecatedGroupsAndAttributesAreReported(t *testing.T) {
	from := registry(t, "v1.4.0", attributes("db", defines("db.namespace", schemaregistry.Required)))

	group := attributes("db", schemaregistry.Attribute{
		ID: "db.namespace", Type: "string", Level: schemaregistry.Required, Brief: "the db.namespace attribute",
		Deprecation: &schemaregistry.Deprecation{RenamedTo: "db.name"},
	})
	group.Deprecation = &schemaregistry.Deprecation{Reason: "renamed"}
	to := registry(t, "v1.5.0", group)

	rep, err := RegistryImpact(RegistryInputs{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	attr, ok := findChange(rep, Deprecated, "db.namespace")
	if !ok {
		t.Fatalf("the deprecated attribute was not reported: %+v", rep.Changes)
	}
	if !strings.Contains(attr.Detail, "renamed to db.name") {
		t.Errorf("the attribute's deprecation reads %q", attr.Detail)
	}
	if _, ok := findChange(rep, Deprecated, "db"); !ok {
		t.Errorf("the deprecated group was not reported: %+v", rep.Changes)
	}
}

// schemaRequirement is one loaded-shaped schema-conformance requirement.
func schemaRequirement(id string) requirements.Requirement {
	return requirements.Requirement{
		ID:     id,
		Title:  "database attributes",
		Owner:  "platform-lead",
		Schema: &requirements.SchemaAssertion{RegistryVersion: "v1.4.0"},
	}
}

func verdict(service, environment string, findings ...conformance.Finding) conformance.Verdict {
	return conformance.Verdict{
		Row:      conformance.Row{Service: service, Environment: environment},
		Findings: findings,
	}
}

func schemaFinding(id string, outcome conformance.Outcome, missing ...string) conformance.Finding {
	return conformance.Finding{
		Requirement: schemaRequirement(id),
		Outcome:     outcome,
		Grade:       ownership.Violation,
		Missing:     missing,
	}
}

// ADR-0034's Consequences: "N services newly fail required on db.namespace".
func TestTheEstateHalfNamesTheAttributeAndTheServices(t *testing.T) {
	from := registry(t, "v1.4.0", attributes("db", defines("db.namespace", schemaregistry.Recommended)))
	to := registry(t, "v1.5.0", attributes("db", defines("db.namespace", schemaregistry.Required)))

	rep, err := RegistryImpact(RegistryInputs{
		From: from,
		To:   to,
		Estate: &EstateReading{
			Before: []conformance.Verdict{
				verdict("checkout", "production", schemaFinding("db-attrs", conformance.Compliant)),
				verdict("orders", "production", schemaFinding("db-attrs", conformance.Compliant)),
				verdict("search", "production", schemaFinding("db-attrs", conformance.Compliant)),
			},
			After: []conformance.Verdict{
				verdict("checkout", "production", schemaFinding("db-attrs", conformance.Misconfigured, "db.namespace")),
				verdict("orders", "production", schemaFinding("db-attrs", conformance.Misconfigured, "db.namespace")),
				verdict("search", "production", schemaFinding("db-attrs", conformance.Compliant)),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	change, ok := findChange(rep, NewlyFailing, "db.namespace")
	if !ok {
		t.Fatalf("no newly failing Service reported: %+v", rep.Changes)
	}
	if len(change.Services) != 2 || change.Services[0] != "checkout" || change.Services[1] != "orders" {
		t.Errorf("newly failing Services are %q, want checkout and orders", change.Services)
	}
	if !strings.Contains(rep.Summary(), "2 Services newly fail a required attribute") {
		t.Errorf("summary is %q", rep.Summary())
	}
	line := change.Line()
	if !strings.Contains(line, "db.namespace") || !strings.Contains(line, "checkout, orders") {
		t.Errorf("the line reads %q", line)
	}
}

// A Service that was already failing has not been made worse by this
// activation, and an activation that reported it would read as breaking
// something it did not touch.
func TestAServiceThatWasAlreadyFailingIsNotNewlyFailing(t *testing.T) {
	from := registry(t, "v1.4.0", attributes("db", defines("db.namespace", schemaregistry.Required)))
	to := registry(t, "v1.5.0", attributes("db",
		defines("db.namespace", schemaregistry.Required),
		defines("db.system", schemaregistry.Recommended)))

	rep, err := RegistryImpact(RegistryInputs{
		From: from,
		To:   to,
		Estate: &EstateReading{
			Before: []conformance.Verdict{
				verdict("checkout", "production", schemaFinding("db-attrs", conformance.Misconfigured, "db.namespace")),
			},
			After: []conformance.Verdict{
				verdict("checkout", "production", schemaFinding("db-attrs", conformance.Misconfigured, "db.namespace")),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findChange(rep, NewlyFailing, "db.namespace"); ok {
		t.Errorf("an unchanged failure was reported as new: %+v", rep.Changes)
	}
}

// Without an estate reading the report says so. Reporting that no Service
// newly fails would be a claim nobody checked.
func TestNoEstateReadingIsSaidOutLoud(t *testing.T) {
	from := registry(t, "v1.4.0", attributes("db", defines("db.namespace", schemaregistry.Recommended)))
	to := registry(t, "v1.5.0", attributes("db", defines("db.namespace", schemaregistry.Required)))

	rep, err := RegistryImpact(RegistryInputs{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Unknown) != 1 || !strings.Contains(rep.Unknown[0], "which Services stop passing") {
		t.Errorf("the report is silent about its own silence: %q", rep.Unknown)
	}
	if rep.Count(NewlyFailing) != 0 {
		t.Error("a report with no estate reading claimed something about Services")
	}
}

// An improvement-grade finding getting worse is a real change, and it is not
// a Service that stops passing: the binary is violations alone.
func TestAnImprovementGradeFailureIsNotAServiceThatStopsPassing(t *testing.T) {
	from := registry(t, "v1.4.0", attributes("db", defines("db.namespace", schemaregistry.Recommended)))
	to := registry(t, "v1.5.0", attributes("db", defines("db.namespace", schemaregistry.Recommended)))

	advisory := schemaFinding("db-attrs", conformance.Misconfigured, "db.namespace")
	advisory.Grade = ownership.Advisory

	rep, err := RegistryImpact(RegistryInputs{
		From: from,
		To:   to,
		Estate: &EstateReading{
			Before: []conformance.Verdict{verdict("checkout", "production", schemaFinding("db-attrs", conformance.Compliant))},
			After:  []conformance.Verdict{verdict("checkout", "production", advisory)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Count(NewlyFailing) != 0 {
		t.Errorf("an improvement was counted as a Service that stops passing: %+v", rep.Changes)
	}
}
