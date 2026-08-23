package serving

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Artefact choice is most-specific-wins over satisfied selectors, with an
// equal-specificity tie resolving to the first Tier in id order. Any
// deterministic rule keeps replicas agreeing (ADR-0032 §2); this one is
// also stable under re-ordering of the authored files.
func TestMatchPicksMostSpecificSelectorDeterministically(t *testing.T) {
	snap := &Snapshot{
		entries: []entry{
			{tier: "a/broad", selector: map[string]string{"tier": "gateway"}, artefact: []byte("broad")},
			{tier: "a/narrow", selector: map[string]string{"tier": "gateway", "region": "eu"}, artefact: []byte("narrow")},
			{tier: "b/broad-too", selector: map[string]string{"env": "production"}, artefact: []byte("broad-too")},
		},
		unmatched: []byte("unmatched"),
	}

	cases := []struct {
		name  string
		attrs map[string]string
		tier  string
	}{
		{"most specific wins", map[string]string{"tier": "gateway", "region": "eu", "env": "production"}, "a/narrow"},
		{"partial selector never matches", map[string]string{"region": "eu"}, ""},
		{"single pair", map[string]string{"tier": "gateway"}, "a/broad"},
		{"equal specificity ties to first id", map[string]string{"tier": "gateway", "env": "production"}, "a/broad"},
		{"no attributes", nil, ""},
	}
	for _, c := range cases {
		m := snap.Match(c.attrs)
		if m.Tier != c.tier {
			t.Errorf("%s: matched %q, want %q", c.name, m.Tier, c.tier)
		}
		if c.tier == "" && (!m.Unmatched || string(m.Artefact) != "unmatched") {
			t.Errorf("%s: an unmatched collector must receive the Unmatched artefact, got %q", c.name, m.Artefact)
		}
		if len(m.Artefact) == 0 {
			t.Errorf("%s: a Match carried no artefact. No decision may end empty", c.name) // ADR-0010 rule 6
		}
	}
}

// The compiled index holds exactly the selector-carrying Tiers: a Tier
// without a selector is reachable only by the git-delivered path
// (REQ-041), so it has no line to match against.
func TestSnapshotIndexesOnlySelectorCarryingTiers(t *testing.T) {
	root, _ := fixtureEstate(t)
	snap, err := LoadSnapshot(root, fixtureCommit)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.entries) != 2 {
		t.Fatalf("index holds %d entries, want 2: the selectorless batch Tier must not appear", len(snap.entries))
	}
	if snap.Commit != fixtureCommit {
		t.Errorf("snapshot head = %q, want %q", snap.Commit, fixtureCommit)
	}
}

// A snapshot with no Unmatched artefact cannot honour ADR-0030, so it
// refuses to load: the server keeps serving the previous head instead of
// inventing behaviour for unmatched collectors.
func TestSnapshotFailsClosedWithoutUnmatchedArtefact(t *testing.T) {
	root, _ := fixtureEstate(t)
	if err := os.Remove(filepath.Join(root, "rendered", "_estate", "unmatched.yaml")); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSnapshot(root, fixtureCommit)
	if err == nil || !strings.Contains(err.Error(), "re-render") {
		t.Fatalf("a snapshot without the Unmatched artefact loaded: %v", err)
	}
}

// An empty rendered artefact is refused at the snapshot, before it can
// ever become an empty config map on the wire (ADR-0010 rule 6).
func TestSnapshotFailsClosedOnEmptyArtefact(t *testing.T) {
	root, _ := fixtureEstate(t)
	writeFile(t, root, "rendered/pipelines/gateway.yaml", "\n")
	_, err := LoadSnapshot(root, fixtureCommit)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("a snapshot with an empty artefact loaded: %v", err)
	}
}

// DirSource is the standalone rung (ADR-0032 §3): a plain directory with
// no git history around it still serves: the head is simply unnamed, and
// identity travels in the artefacts (ADR-0013).
func TestDirSourceServesAPlainDirectory(t *testing.T) {
	root, _ := fixtureEstate(t)
	snap, err := DirSource{Root: root}.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Commit != "" {
		t.Errorf("snapshot head = %q, want empty for a directory outside git", snap.Commit)
	}
	if m := snap.Match(gatewayAttrs()); m.Tier != "pipelines/gateway" {
		t.Errorf("matched %q, want pipelines/gateway", m.Tier)
	}
}
