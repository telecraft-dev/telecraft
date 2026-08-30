package console

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The document set of an estate that authors no Tier (ADR-0086 §3): the
// team tree it was created with, and empty everything else, projected by
// the same code a full build projects with.
func TestBuildNewEstateProjectsTheTeamTreeAndNothingElse(t *testing.T) {
	root := t.TempDir()
	teams := "teams:\n" +
		"  - id: engineering\n" +
		"    name: Engineering\n" +
		"    owners: [a]\n"
	if err := os.WriteFile(filepath.Join(root, "teams.yaml"), []byte(teams), 0o644); err != nil {
		t.Fatal(err)
	}

	taken := Readings{AsOf: time.Now().UTC()}
	bundle, err := BuildNewEstate(Inputs{Root: root, Taken: &taken})
	if err != nil {
		t.Fatalf("BuildNewEstate: %v", err)
	}

	if bundle.Estate.Teams.ID != "engineering" {
		t.Errorf("the team tree is %+v, want the one team the estate holds", bundle.Estate.Teams)
	}
	if got := len(bundle.Estate.Owners); got != 1 {
		t.Errorf("%d owners, want the one the tree names", got)
	}
	for name, n := range map[string]int{
		"cards":        len(bundle.Estate.Cards),
		"collectors":   len(bundle.Estate.Collectors),
		"rollouts":     len(bundle.Estate.Rollouts),
		"services":     len(bundle.Estate.Services),
		"blueprints":   len(bundle.Estate.Blueprints),
		"catalogue":    len(bundle.Estate.Catalogue),
		"requirements": len(bundle.Estate.Requirements),
		"environments": len(bundle.Estate.Environments),
		"drawers":      len(bundle.Estate.Drawers),
		"hops":         len(bundle.Estate.Topology.Hops),
		"paths":        len(bundle.Estate.Topology.Paths),
	} {
		if n != 0 {
			t.Errorf("a new estate reports %d %s, want none", n, name)
		}
	}
}

// An estate with no team tree at all is not a new estate, it is a
// directory. The refusal names the file, because the reader has pointed
// the server somewhere.
func TestBuildNewEstateRefusesAnEstateWithNoTeamTree(t *testing.T) {
	taken := Readings{AsOf: time.Now().UTC()}
	_, err := BuildNewEstate(Inputs{Root: t.TempDir(), Taken: &taken})
	if err == nil {
		t.Fatal("a directory with no teams.yaml built a document set anyway")
	}
}
