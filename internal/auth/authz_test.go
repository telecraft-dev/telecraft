package auth

import (
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/ownership"
)

func testActor(t *testing.T, email string) Actor {
	t.Helper()
	users := writeUsers(t, goodUsers)
	actor, err := Resolve(Identity{Subject: "sub-1", Name: "", Email: email}, users, testTree())
	if err != nil {
		t.Fatalf("Resolve(%q): %v", email, err)
	}
	return actor
}

func TestResolveJoinsIdentityToOwnerAndTeam(t *testing.T) {
	actor := testActor(t, "jo@example.com")
	if actor.Owner != "gateway-owners" || actor.Team != "data-flow" {
		t.Fatalf("Resolve placed the actor at %+v", actor)
	}
	// The provider carried no name claim, so users.yaml fills it.
	if actor.Identity.Name != "Jo Author" {
		t.Fatalf("Resolve left the name %q", actor.Identity.Name)
	}
}

func TestResolveKeepsTheProviderNameClaim(t *testing.T) {
	users := writeUsers(t, goodUsers)
	actor, err := Resolve(Identity{Subject: "s", Name: "Josephine Author", Email: "jo@example.com"}, users, testTree())
	if err != nil {
		t.Fatal(err)
	}
	if actor.Identity.Name != "Josephine Author" {
		t.Fatalf("the provider's name claim authors the change, got %q", actor.Identity.Name)
	}
}

func TestResolveRefusesAnIdentityTheEstateDoesNotKnow(t *testing.T) {
	users := writeUsers(t, goodUsers)
	_, err := Resolve(Identity{Subject: "s", Email: "stranger@example.com"}, users, testTree())
	if err == nil || !strings.Contains(err.Error(), UsersFile) {
		t.Fatalf("Resolve = %v, want an unknown identity to fail closed, naming the seam", err)
	}
}

// Acceptance: the acting user's team membership determines which actions
// surfaces offer (issue #26): edit authority is the actor's team subtree,
// never a sibling or ancestor (ADR-0016, ADR-0017; authority points down
// the tree as in ADR-0021 and ADR-0037).
func TestCanEditIsTheActorsTeamSubtree(t *testing.T) {
	est := testEstate()
	actor := testActor(t, "jo@example.com") // data-flow

	cases := []struct {
		name string
		ref  ownership.Ref
		want bool
	}{
		{"an object their own team owns", ownership.Ref{Kind: ownership.KindBlueprint, ID: "data-flow/gateway-standard"}, true},
		{"an object a descendant team owns", ownership.Ref{Kind: ownership.KindBlueprint, ID: "edge/edge-standard"}, true},
		{"an object a sibling team owns", ownership.Ref{Kind: ownership.KindComponent, ID: "infosec/pii-redaction"}, false},
		{"an object an ancestor team owns", ownership.Ref{Kind: ownership.KindTier, ID: "platform/global"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := actor.CanEdit(est, tc.ref)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("CanEdit(%v) = %v, want %v", tc.ref, got, tc.want)
			}
		})
	}
}

func TestCanEditOnAMissingObjectIsAnErrorNeverAFalse(t *testing.T) {
	actor := testActor(t, "jo@example.com")
	_, err := actor.CanEdit(testEstate(), ownership.Ref{Kind: ownership.KindBlueprint, ID: "ghost"})
	if err == nil {
		t.Fatal("CanEdit answered a question about nothing")
	}
}

func TestActionableTeamsIsTheSubtreeRootFirst(t *testing.T) {
	actor := testActor(t, "jo@example.com")
	teams, err := actor.ActionableTeams(testTree())
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 2 || teams[0] != "data-flow" || teams[1] != "edge" {
		t.Fatalf("ActionableTeams = %v", teams)
	}
}

// ADR-0020 §6: activation is offered to operators, not to general console
// users, and the operator falls out of the same tree as every other
// permission. Activating changes judgement for the whole Estate, so the
// actors who may do it are the ones whose horizon is the whole Estate.
func TestOnlyAnActorAtARootOfTheTreeIsAnOperator(t *testing.T) {
	tree := testTree()
	cases := []struct {
		team ownership.TeamID
		want bool
	}{
		{"platform", true},   // a root: no parent above it
		{"data-flow", false}, // one below the root
		{"edge", false},
		{"infosec", false},
	}
	for _, tc := range cases {
		t.Run(string(tc.team), func(t *testing.T) {
			actor := Actor{Team: tc.team, Owner: "someone"}
			got, err := actor.Operator(tree)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("Operator() = %v for team %q, want %v", got, tc.team, tc.want)
			}
		})
	}
}

func TestAnActorOutsideTheTreeIsNotAnOperator(t *testing.T) {
	actor := Actor{Team: "nowhere", Owner: "someone"}
	got, err := actor.Operator(testTree())
	if err == nil {
		t.Fatal("an actor with no place in the tree resolved")
	}
	if got {
		t.Error("an actor with no place in the tree is an operator")
	}
}
