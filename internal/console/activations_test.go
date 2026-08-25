package console_test

import (
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/console"
)

func substrate(t *testing.T, b console.Bundle, kind string) console.SubstrateDoc {
	t.Helper()
	for _, s := range b.Activations.Substrates {
		if s.Kind == kind {
			return s
		}
	}
	t.Fatalf("no %q substrate in the snapshot", kind)
	return console.SubstrateDoc{}
}

// The active version is the estate's own designation, not a version the
// snapshot picked (ADR-0020 §9).
func TestTheSnapshotCarriesTheEstatesDesignation(t *testing.T) {
	b := build(t)
	cat := substrate(t, b, "catalogue")
	if cat.Active != "v1.0.0" {
		t.Errorf("active Catalogue is %q, want the designated v1.0.0", cat.Active)
	}
	if cat.Name != "Catalogue" {
		t.Errorf("substrate is named %q", cat.Name)
	}
	if len(cat.History) != 1 || cat.History[0].By != "engineering-lead" {
		t.Errorf("the audit trail is %+v", cat.History)
	}
	if cat.History[0].At != "2026-08-01T09:00:00Z" {
		t.Errorf("the activation time is %q", cat.History[0].At)
	}
}

// Every installed version that is not active is on offer, each with the
// report the decision would be taken on (ADR-0020 §6).
func TestEveryRetainedVersionIsOfferedWithItsImpactReport(t *testing.T) {
	cat := substrate(t, build(t), "catalogue")
	if len(cat.Candidates) != 1 {
		t.Fatalf("candidates are %+v, want the one retained version that is not active", cat.Candidates)
	}
	candidate := cat.Candidates[0]
	if candidate.Version != "v1.1.0" {
		t.Fatalf("candidate is %q", candidate.Version)
	}
	if candidate.Blocked != "" {
		t.Fatalf("the report could not be computed: %s", candidate.Blocked)
	}
	if !strings.Contains(candidate.Summary, "1 component in use is removed") {
		t.Errorf("summary is %q", candidate.Summary)
	}
	var named bool
	for _, line := range candidate.Lines {
		if strings.Contains(line, "processor/transform is removed") && strings.Contains(line, "data-flow/gateway-standard") {
			named = true
		}
	}
	if !named {
		t.Errorf("no line names the removal and the Blueprint it lands on: %q", candidate.Lines)
	}
}

// The active version is never offered as something to activate.
func TestTheActiveVersionIsNotACandidate(t *testing.T) {
	for _, c := range substrate(t, build(t), "catalogue").Candidates {
		if c.Version == "v1.0.0" {
			t.Error("the active version was offered as a candidate")
		}
	}
}

// A substrate the estate has never imported has nothing to designate and
// nothing to offer, which is a normal state and not an empty error.
func TestASubstrateWithNothingImportedIsEmptyRatherThanMissing(t *testing.T) {
	reg := substrate(t, build(t), "schema_registry")
	if reg.Active != "" {
		t.Errorf("a Schema Registry version is active in an estate that imported none: %q", reg.Active)
	}
	if len(reg.Candidates) != 0 || len(reg.History) != 0 {
		t.Errorf("the substrate carries %+v", reg)
	}
}

// ADR-0020 §6: activation is offered to operators, not to general console
// users. The fixture user sits below the root of the tree.
func TestAUserBelowTheRootIsNotOfferedActivation(t *testing.T) {
	if build(t).Estate.Me.Operator {
		t.Error("a user one team below the root is an operator")
	}
}

func TestAUserAtTheRootIsAnOperator(t *testing.T) {
	in := fixtureInputs()
	in.User.Team = "engineering"
	bundle, err := console.Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.Estate.Me.Operator {
		t.Error("a user at the root of the tree is not an operator")
	}
}

// The designation is what says which version judges authoring, so a caller
// that names no artefact gets the estate's answer rather than a guess.
func TestTheActiveArtefactComesFromTheDesignation(t *testing.T) {
	in := fixtureInputs()
	in.Active = ""
	bundle, err := console.Build(in)
	if err != nil {
		t.Fatalf("building against the estate's designation: %v", err)
	}
	if bundle.Catalogues.Active != "v1.0.0" {
		t.Errorf("judged against %q, want the designated v1.0.0", bundle.Catalogues.Active)
	}
}
