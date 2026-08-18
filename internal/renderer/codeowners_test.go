package renderer

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/ownership"
)

// The generated projection derives from the tree, not the directory shape
// (ADR-0027 §1): a child team's paths carry its own owners plus its
// ancestors', nearest first, over both the authored subtree and the
// rendered artefacts.
func TestCodeOwnersIncludesAncestors(t *testing.T) {
	tree, err := ownership.LoadTeams(filepath.Join("testdata", "estate", ownership.TeamsFile))
	if err != nil {
		t.Fatal(err)
	}
	got := string(CodeOwners(tree))

	for _, want := range []string{
		"/teams/data-flow/ @gateway-owners @platform-observability",
		"/rendered/data-flow/ @gateway-owners @platform-observability",
		"/teams/infosec/ @pii-guardians",
		"/rendered/infosec/ @pii-guardians",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("CODEOWNERS lacks %q:\n%s", want, got)
		}
	}

	// engineering has no owner anywhere on its chain: a line assigning
	// review to nobody must not be emitted.
	if strings.Contains(got, "/teams/engineering/") {
		t.Error("an ownerless team got a CODEOWNERS line assigning review to nobody")
	}
}

func TestCodeOwnersIsDeterministic(t *testing.T) {
	tree, err := ownership.LoadTeams(filepath.Join("testdata", "estate", ownership.TeamsFile))
	if err != nil {
		t.Fatal(err)
	}
	if a, b := string(CodeOwners(tree)), string(CodeOwners(tree)); a != b {
		t.Error("two projections of the same tree differ")
	}
}

// The Unmatched artefact under rendered/_estate/ is root-team-owned by
// convention (ADR-0030). The fixture tree's root (engineering) is
// ownerless, so no line renders there — the scratch tree's root carries
// owners and gets the line.
func TestCodeOwnersAssignsEstateArtefactsToTheRootTeam(t *testing.T) {
	fixtureTree, err := ownership.LoadTeams(filepath.Join("testdata", "estate", ownership.TeamsFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(CodeOwners(fixtureTree)); strings.Contains(got, "/rendered/_estate/") {
		t.Errorf("an ownerless root team got a /rendered/_estate/ line assigning review to nobody:\n%s", got)
	}

	root := t.TempDir()
	writeFile(t, root, ownership.TeamsFile, scratchTeams)
	tree, err := ownership.LoadTeams(filepath.Join(root, ownership.TeamsFile))
	if err != nil {
		t.Fatal(err)
	}
	got := string(CodeOwners(tree))
	if want := "/rendered/_estate/ @org-lead"; !strings.Contains(got, want) {
		t.Errorf("CODEOWNERS lacks %q — the root team owns the estate-level governance artefacts (ADR-0030):\n%s", want, got)
	}
}
