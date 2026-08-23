package serving

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// GitSource is git-the-tool over a plain local repository (ADR-0032 §3):
// clone once, then each poll fetches the remote HEAD, so a pushed commit is
// visible on the next snapshot, which is the whole freshness story.
func TestGitSourceFetchesHeadAndPicksUpNewCommits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git binary: GitSource is exercised where git-the-tool exists")
	}
	ctx := context.Background()
	origin, _ := fixtureEstate(t)
	rungit(t, origin, "init", "--quiet")
	rungit(t, origin, "add", ".")
	rungit(t, origin, "commit", "--quiet", "-m", "estate at first head")
	first := rungit(t, origin, "rev-parse", "HEAD")

	src := GitSource{URL: origin, Dir: t.TempDir() + "/clone"}
	snap, err := src.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Commit != first {
		t.Fatalf("snapshot head = %s, want %s", snap.Commit, first)
	}

	// A new head lands in the origin; the next poll serves it.
	writeFile(t, origin, "rendered/pipelines/gateway.yaml", "# changed at the new head\nreceivers: {}\n")
	rungit(t, origin, "commit", "--quiet", "-am", "estate at second head")
	second := rungit(t, origin, "rev-parse", "HEAD")

	snap, err = src.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Commit != second {
		t.Fatalf("snapshot head = %s, want the new head %s: bounded staleness is one fetch interval", snap.Commit, second) // ADR-0032
	}
	if m := snap.Match(gatewayAttrs()); string(m.Artefact) != "# changed at the new head\nreceivers: {}\n" {
		t.Errorf("the new head's artefact is not being served:\n%s", m.Artefact)
	}
}

// rungit runs git in dir with identity pinned, so the test needs no host
// git config.
func rungit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.com",
		"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
