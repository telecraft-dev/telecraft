package ownership

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write drops one file into dir, creating parents as needed.
func write(t *testing.T, dir, name, body string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// loadErr is Load reduced to its error, for tests that only assert on failure.
func loadErr(t *testing.T, dir string) error {
	t.Helper()
	est, err := Load(dir)
	if err != nil && (len(est.Objects) != 0 || len(est.Tree.Teams) != 0) {
		t.Fatalf("Load failed but returned a non-empty estate: a failed load must fail closed")
	}
	return err
}

const goodTeams = `
teams:
  - id: platform
    name: Platform
    owners: [platform-observability]
`

const goodObject = `
- kind: tier
  id: edge
  owner: platform-observability
`

// loadFixture loads the acceptance fixture: a nested team tree plus authored
// objects across several kinds.
func loadFixture(t *testing.T) Estate {
	t.Helper()
	est, err := Load(filepath.Join("testdata", "estate"))
	if err != nil {
		t.Fatalf("the fixture estate does not load: %v", err)
	}
	return est
}

// Acceptance: an estate fixture with a nested team tree loads, and the tree
// reads back exactly as authored: parents, children, owner membership.
func TestEstateFixtureWithNestedTeamTreeLoads(t *testing.T) {
	est := loadFixture(t)

	if len(est.Tree.Teams) != 5 {
		t.Fatalf("fixture tree holds %d teams, want 5", len(est.Tree.Teams))
	}
	for team, parent := range map[TeamID]TeamID{
		"engineering": "",
		"platform":    "engineering",
		"data-flow":   "platform",
		"infosec":     "engineering",
		"product":     "engineering",
	} {
		got, ok := est.Tree.Teams[team]
		if !ok {
			t.Fatalf("team %q did not load", team)
		}
		if got.Parent != parent {
			t.Errorf("team %q has parent %q, want %q", team, got.Parent, parent)
		}
	}

	owner, ok := est.Tree.Owners["gateway-owners"]
	if !ok || owner.Team != "data-flow" {
		t.Errorf("owner gateway-owners = %+v, want membership of team data-flow", owner)
	}

	subtree, err := est.Tree.Subtree("platform")
	if err != nil {
		t.Fatal(err)
	}
	if len(subtree) != 2 || subtree[0] != "platform" || subtree[1] != "data-flow" {
		t.Errorf("Subtree(platform) = %v, want [platform data-flow]", subtree)
	}
	if all, _ := est.Tree.Subtree("engineering"); len(all) != 5 {
		t.Errorf("Subtree(engineering) covers %d teams, want all 5", len(all))
	}

	if len(est.Objects) != 7 {
		t.Fatalf("fixture estate holds %d authored objects, want 7", len(est.Objects))
	}
	split := est.Objects[Ref{Kind: KindTier, ID: "gateway-pci"}]
	if split.Owner != "pii-guardians" {
		t.Errorf("split tier gateway-pci owned by %q, want pii-guardians", split.Owner)
	}
	for ref, o := range est.Objects {
		if o.Owner == "" {
			t.Errorf("%s %q loaded without an owner", ref.Kind, ref.ID)
		}
	}
}

// Acceptance: an ownerless authored object fails validation at load:
// fail closed, never a finding that routes to nobody (REQ-015, ADR-0016).
func TestOwnerlessAuthoredObjectFailsValidation(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "teams.yaml", goodTeams)
	write(t, dir, "tiers.yaml", `
- kind: tier
  id: edge
`)
	err := loadErr(t, dir)
	if err == nil {
		t.Fatal("expected a load error for an ownerless authored object")
	}
	if !strings.Contains(err.Error(), "owner") || !strings.Contains(err.Error(), "tiers.yaml") {
		t.Errorf("error does not name the missing owner and the file: %v", err)
	}
}

// An unknown field must fail the load naming the file and the field, in the
// team tree and in object files alike.
func TestUnknownFieldFailsClosedWithFileAndField(t *testing.T) {
	t.Run("teams file", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "teams.yaml", `
teams:
  - id: platform
    lead: someone
`)
		write(t, dir, "tiers.yaml", goodObject)
		err := loadErr(t, dir)
		if err == nil || !strings.Contains(err.Error(), "teams.yaml") || !strings.Contains(err.Error(), "lead") {
			t.Fatalf("expected an error naming teams.yaml and the field, got %v", err)
		}
	})
	t.Run("object file", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "teams.yaml", goodTeams)
		write(t, dir, "tiers.yaml", `
- kind: tier
  id: edge
  owner: platform-observability
  onwer: typo
`)
		err := loadErr(t, dir)
		if err == nil || !strings.Contains(err.Error(), "tiers.yaml") || !strings.Contains(err.Error(), "onwer") {
			t.Fatalf("expected an error naming tiers.yaml and the field, got %v", err)
		}
	})
}

// An Owner belongs to exactly one Team; listed under two, every roll-up
// containing both would double-count their findings (ADR-0017).
func TestOwnerInTwoTeamsIsRejected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "teams.yaml", `
teams:
  - id: platform
    owners: [shared-owner]
  - id: infosec
    owners: [shared-owner]
`)
	write(t, dir, "tiers.yaml", `
- kind: tier
  id: edge
  owner: shared-owner
`)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "exactly one Team") {
		t.Fatalf("expected an owner-in-two-teams error, got %v", err)
	}
	if !strings.Contains(err.Error(), `"platform"`) || !strings.Contains(err.Error(), `"infosec"`) {
		t.Errorf("error does not name both teams: %v", err)
	}
}

// A duplicate team id is the only way to write multi-parent membership into
// a nested tree, and it is rejected for the same double-counting reason.
func TestDuplicateTeamIDIsRejected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "teams.yaml", `
teams:
  - id: platform
    teams:
      - id: shared
  - id: infosec
    teams:
      - id: shared
`)
	write(t, dir, "tiers.yaml", goodObject)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "at most one parent") {
		t.Fatalf("expected a duplicate-team error, got %v", err)
	}
}

func TestUnknownOwnerOnObjectIsRejected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "teams.yaml", goodTeams)
	write(t, dir, "tiers.yaml", `
- kind: tier
  id: edge
  owner: nobody-anyone-knows
`)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "nobody-anyone-knows") {
		t.Fatalf("expected an unknown-owner error, got %v", err)
	}
}

// Collectors are not ownable (ADR-0016): one authored as an object must be
// rejected with the remedy (split the Tier) in the message.
func TestCollectorAsAuthoredObjectIsRejected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "teams.yaml", goodTeams)
	write(t, dir, "collectors.yaml", `
- kind: collector
  id: host-42
  owner: platform-observability
`)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "split the Tier") {
		t.Fatalf("expected a collector-not-authored error naming the remedy, got %v", err)
	}
}

func TestUnknownObjectKindIsRejected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "teams.yaml", goodTeams)
	write(t, dir, "objects.yaml", `
- kind: dashboard
  id: overview
  owner: platform-observability
`)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), `unknown object kind "dashboard"`) {
		t.Fatalf("expected an unknown-kind error, got %v", err)
	}
}

func TestDuplicateObjectNamesBothFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "teams.yaml", goodTeams)
	write(t, dir, "a.yaml", goodObject)
	write(t, dir, "b.yaml", goodObject)
	err := loadErr(t, dir)
	if err == nil {
		t.Fatal("expected a load error for a duplicate object")
	}
	if !strings.Contains(err.Error(), "a.yaml") || !strings.Contains(err.Error(), "b.yaml") {
		t.Errorf("duplicate error does not name both files: %v", err)
	}
}

func TestMissingTeamsFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "tiers.yaml", goodObject)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "teams.yaml") {
		t.Fatalf("expected a missing-teams.yaml error, got %v", err)
	}
}

func TestEstateWithoutObjectsIsAnError(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "teams.yaml", goodTeams)
	if err := loadErr(t, dir); err == nil {
		t.Fatal("an estate with no authored objects must not load: there is nothing to route or roll up")
	}
}

func TestEmptyTeamsFileIsRejected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "teams.yaml", "teams: []\n")
	write(t, dir, "tiers.yaml", goodObject)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "no teams") {
		t.Fatalf("expected a no-teams error, got %v", err)
	}
}

func TestMalformedYAMLFailsClosedNamingTheFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "teams.yaml", goodTeams)
	write(t, dir, "broken.yaml", "{ this is: [not yaml")
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "broken.yaml") {
		t.Fatalf("expected an error naming the broken file, got %v", err)
	}
}

func TestMultipleYAMLDocumentsAreRejected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "teams.yaml", goodTeams)
	write(t, dir, "two.yaml", goodObject+"\n---\n- kind: tier\n  id: other\n")
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "one concern per file") {
		t.Fatalf("expected a multi-document error, got %v", err)
	}
}

func TestMissingDirectoryIsAnError(t *testing.T) {
	if err := loadErr(t, filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected an error for a missing estate directory")
	}
}

// Allow-lists, Grants and users live beside teams.yaml in the estate
// directory (ADR-0021 §5; ADR-0019); they are policy and membership, loaded
// by internal/allowlist and pkg/auth, and must not be parsed here as
// authored-object files.
func TestPolicyFilesBesideTeamsAreSkipped(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "teams.yaml", goodTeams)
	write(t, dir, "tiers.yaml", goodObject)
	write(t, dir, "allow-lists.yaml", "allow_lists:\n  - team: platform\n    owner: platform-observability\n    allow: [receiver/otlp]\n")
	write(t, dir, "grants.yaml", "grants: []\n")
	write(t, dir, "users.yaml", "users:\n  - email: jo@example.com\n    name: Jo\n    owner: platform-observability\n")
	est, err := Load(dir)
	if err != nil {
		t.Fatalf("an estate with policy files beside teams.yaml must load: %v", err)
	}
	if len(est.Objects) != 1 {
		t.Errorf("got %d objects, want just the tier: policy files are not authored objects", len(est.Objects))
	}
}

// LoadTeams is the seam for consumers that judge against teams alone.
func TestLoadTeamsReadsJustTheTree(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "teams.yaml", goodTeams)
	tree, err := LoadTeams(filepath.Join(dir, "teams.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tree.Teams["platform"]; !ok {
		t.Error("the tree does not hold the authored team")
	}
}
