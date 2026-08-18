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
