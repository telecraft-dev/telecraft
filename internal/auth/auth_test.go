package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/ownership"
)

// The shared fixture tree:
//
//	platform (platform-observability)
//	├── data-flow (gateway-owners)
//	│   └── edge (edge-owners)
//	└── infosec (pii-guardians)
func testTree() ownership.Tree {
	teams := map[ownership.TeamID]ownership.Team{
		"platform":  {ID: "platform", Name: "Platform", Owners: []ownership.OwnerID{"platform-observability"}, Children: []ownership.TeamID{"data-flow", "infosec"}},
		"data-flow": {ID: "data-flow", Name: "Data flow", Parent: "platform", Owners: []ownership.OwnerID{"gateway-owners"}, Children: []ownership.TeamID{"edge"}},
		"edge":      {ID: "edge", Name: "Edge", Parent: "data-flow", Owners: []ownership.OwnerID{"edge-owners"}},
		"infosec":   {ID: "infosec", Name: "Infosec", Parent: "platform", Owners: []ownership.OwnerID{"pii-guardians"}},
	}
	owners := map[ownership.OwnerID]ownership.Owner{}
	for _, team := range teams {
		for _, o := range team.Owners {
			owners[o] = ownership.Owner{ID: o, Team: team.ID}
		}
	}
	return ownership.Tree{Teams: teams, Owners: owners}
}

func testEstate() ownership.Estate {
	objects := map[ownership.Ref]ownership.Object{}
	add := func(kind ownership.ObjectKind, id string, owner ownership.OwnerID) {
		objects[ownership.Ref{Kind: kind, ID: id}] = ownership.Object{Kind: kind, ID: id, Owner: owner}
	}
	add(ownership.KindBlueprint, "data-flow/gateway-standard", "gateway-owners")
	add(ownership.KindBlueprint, "edge/edge-standard", "edge-owners")
	add(ownership.KindComponent, "infosec/pii-redaction", "pii-guardians")
	add(ownership.KindTier, "platform/global", "platform-observability")
	return ownership.Estate{Tree: testTree(), Objects: objects}
}

// writeUsers writes a users.yaml and loads it against the fixture tree.
func writeUsers(t *testing.T, body string) Users {
	t.Helper()
	path := filepath.Join(t.TempDir(), UsersFile)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	users, err := LoadUsers(path, testTree())
	if err != nil {
		t.Fatalf("the fixture users file does not load: %v", err)
	}
	return users
}

// usersErr loads a users.yaml expected to fail, returning the error text.
func usersErr(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), UsersFile)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	users, err := LoadUsers(path, testTree())
	if err == nil {
		t.Fatalf("LoadUsers accepted an invalid file")
	}
	if len(users.byEmail) != 0 {
		t.Fatalf("LoadUsers failed but returned users — a failed load must fail closed")
	}
	return err.Error()
}

const goodUsers = `
users:
  - email: jo@example.com
    name: Jo Author
    owner: gateway-owners
  - email: sam@example.com
    name: Sam Guardian
    owner: pii-guardians
`

func TestIdentityAttributionMatchesTheForgeSeam(t *testing.T) {
	id := Identity{Subject: "abc", Name: "Jo Author", Email: "jo@example.com"}
	got := id.Attribution()
	if got.Name != "Jo Author" || got.Email != "jo@example.com" {
		t.Fatalf("Attribution() = %+v — the claims must author the change verbatim (ADR-0019 §3)", got)
	}
	if got.Handle != "" {
		t.Fatalf("Attribution() invented a forge handle %q — a handle is the forge integration's to add", got.Handle)
	}
}
