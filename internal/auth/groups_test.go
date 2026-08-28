package auth

import (
	"reflect"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/ownership"
)

// The fixture mapping: two groups, and one of them shared with a group
// authored later, so precedence has something to decide.
func testGroups() Groups {
	return Groups{
		{Group: "platform-engineering", Owner: "gateway-owners"},
		{Group: "security", Owner: "pii-guardians"},
	}
}

// The first authored rule whose group the identity carries wins, so an
// operator decides precedence by the order they write the rules in.
func TestGroupsResolveInAuthoredOrder(t *testing.T) {
	groups := testGroups()

	cases := map[string]struct {
		asserted []string
		want     ownership.OwnerID
		found    bool
	}{
		"one group":                       {[]string{"security"}, "pii-guardians", true},
		"two groups, the first rule wins": {[]string{"security", "platform-engineering"}, "gateway-owners", true},
		"a group nobody mapped":           {[]string{"everyone"}, "", false},
		"no groups at all":                {nil, "", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			owner, found := groups.Owner(tc.asserted)
			if owner != tc.want || found != tc.found {
				t.Fatalf("Owner(%v) = %q, %v; want %q, %v", tc.asserted, owner, found, tc.want, tc.found)
			}
		})
	}

	// Reversing the rules reverses the answer, which is the whole of what
	// authored order means.
	reversed := Groups{testGroups()[1], testGroups()[0]}
	if owner, _ := reversed.Owner([]string{"security", "platform-engineering"}); owner != "pii-guardians" {
		t.Errorf("reversing the rules did not reverse the answer: %q", owner)
	}
}

// Only the groups the estate names travel any further, and narrowing them
// cannot change who anybody resolves to.
func TestGroupsNarrowToWhatTheEstateNames(t *testing.T) {
	groups := testGroups()
	asserted := []string{"everyone", "security", "a-thousand-others", "platform-engineering"}

	known := groups.Known(asserted)
	if !reflect.DeepEqual(known, []string{"security", "platform-engineering"}) {
		t.Fatalf("Known = %v", known)
	}
	full, _ := groups.Owner(asserted)
	narrowed, _ := groups.Owner(known)
	if full != narrowed {
		t.Errorf("narrowing changed the answer: %q then %q", full, narrowed)
	}
	empty := Groups{}
	if got := empty.Known(asserted); got != nil {
		t.Errorf("a mapping that names nothing carried %v", got)
	}
	if got := groups.Known(nil); got != nil {
		t.Errorf("an identity with no groups carried %v", got)
	}
}

// users.yaml wins wherever it names the email: it is the explicit,
// reviewed statement about one human, and a bulk rule never overrides one.
func TestResolvePrefersTheNamedUserToTheGroupRule(t *testing.T) {
	users := writeUsers(t, goodUsers)
	// jo@example.com is a gateway owner in users.yaml; the mapping would
	// place them in infosec instead.
	groups := Groups{{Group: "security", Owner: "pii-guardians"}}

	actor, err := Resolve(Identity{Subject: "s", Email: "jo@example.com", Groups: []string{"security"}}, users, groups, testTree())
	if err != nil {
		t.Fatal(err)
	}
	if actor.Owner != "gateway-owners" || actor.Team != "data-flow" {
		t.Fatalf("the group rule overrode the named user: %+v", actor)
	}
}

// A human nobody wrote down is placed by the groups their provider
// asserts, and the address they signed in with authors their changes.
func TestResolvePlacesAnUnnamedHumanByTheirGroups(t *testing.T) {
	users := writeUsers(t, goodUsers)
	groups := testGroups()

	actor, err := Resolve(Identity{Subject: "s", Email: "new@example.com", Groups: []string{"everyone", "security"}}, users, groups, testTree())
	if err != nil {
		t.Fatal(err)
	}
	if actor.Owner != "pii-guardians" || actor.Team != "infosec" {
		t.Fatalf("Resolve placed the actor at %+v", actor)
	}
	if actor.Identity.Name != "new@example.com" {
		t.Errorf("nobody named them, so the address authors their changes; got %q", actor.Identity.Name)
	}
	// The session this actor is signed into carries only the groups the
	// estate named.
	if !reflect.DeepEqual(actor.Identity.Groups, []string{"security"}) {
		t.Errorf("the actor carries %v", actor.Identity.Groups)
	}
	// The provider's name claim still wins where it has one.
	named, err := Resolve(Identity{Subject: "s", Name: "New Person", Email: "new@example.com", Groups: []string{"security"}}, users, groups, testTree())
	if err != nil {
		t.Fatal(err)
	}
	if named.Identity.Name != "New Person" {
		t.Errorf("the provider's name claim was dropped: %q", named.Identity.Name)
	}
}

// The mapping resolves membership; it never widens it. A group nobody
// mapped places nobody, and the refusal names both places an operator
// could fix it.
func TestResolveFailsClosedWhenNoGroupIsMapped(t *testing.T) {
	users := writeUsers(t, goodUsers)

	_, err := Resolve(Identity{Subject: "s", Email: "new@example.com", Groups: []string{"everyone"}}, users, testGroups(), testTree())
	if err == nil {
		t.Fatal("an unmapped group signed somebody in")
	}
	for _, want := range []string{UsersFile, ProvidersFile} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %s:\n%v", want, err)
		}
	}

	// With no mapping authored at all, the refusal is the one it has
	// always been: groups nobody asked for are nobody's business.
	_, err = Resolve(Identity{Subject: "s", Email: "new@example.com", Groups: []string{"security"}}, users, nil, testTree())
	if err == nil || strings.Contains(err.Error(), ProvidersFile) {
		t.Errorf("an estate with no mapping answered %v", err)
	}
}

// A mapping is validated against the tree it resolves into, and every
// problem is named rather than the first.
func TestGroupsCheckNamesEveryProblem(t *testing.T) {
	problems := Groups{
		{Group: "", Owner: "gateway-owners"},
		{Group: "security", Owner: ""},
		{Group: "platform-engineering", Owner: "nobody"},
		{Group: "security", Owner: "pii-guardians"},
	}.check(testTree())

	want := []string{"names no group", "names no owner", "not in the team tree", "appears twice"}
	if len(problems) != len(want) {
		t.Fatalf("check found %d problems: %v", len(problems), problems)
	}
	for i, w := range want {
		if !strings.Contains(problems[i], w) {
			t.Errorf("problem %d does not say %q: %q", i, w, problems[i])
		}
	}
	if got := testGroups().check(testTree()); len(got) != 0 {
		t.Errorf("a sound mapping reported %v", got)
	}
}
