package catalogue

import (
	"fmt"
	"os/exec"
	"strings"
)

// Fetch materialises the source tree for one pinned release tag into dir: a
// sparse, depth-1 git checkout of only the metadata.yaml and go.mod files:
// a few megabytes, not the whole repository. It returns the commit the tag
// resolved to, which the artefact records so every Catalogue is reproducible
// and auditable (ADR-0020).
//
// Fetching happens at import time only, on an operator's machine with the
// result carried onward as the artefact; the platform itself never fetches
// at runtime (ADR-0019), and the upstream tree is never vendored into this
// repository.
func Fetch(repoURL, tag, dir string) (commit string, err error) {
	steps := [][]string{
		{"init", "--quiet", "."},
		{"remote", "add", "origin", repoURL},
		{"sparse-checkout", "set", "--no-cone", "**/metadata.yaml", "**/go.mod"},
		{"fetch", "--quiet", "--depth", "1", "origin", "refs/tags/" + tag},
		{"checkout", "--quiet", "FETCH_HEAD"},
	}
	for _, args := range steps {
		if out, err := git(dir, args...); err != nil {
			return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	// Peel explicitly: for an annotated tag, FETCH_HEAD names the tag
	// object, and the auditable fact the artefact records is the commit.
	out, err := git(dir, "rev-parse", "FETCH_HEAD^{commit}")
	if err != nil {
		return "", fmt.Errorf("git rev-parse FETCH_HEAD: %v: %s", err, out)
	}
	return strings.TrimSpace(out), nil
}

// Commit resolves the HEAD commit of an existing checkout, for imports run
// against a pre-fetched tree.
func Commit(dir string) (string, error) {
	out, err := git(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD in %s: %v: %s", dir, err, out)
	}
	return strings.TrimSpace(out), nil
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
