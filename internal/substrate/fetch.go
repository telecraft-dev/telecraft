package substrate

import (
	"fmt"
	"os/exec"
	"strings"
)

// Fetch materialises the source tree for one pinned ref into dir: a sparse,
// depth-1 git checkout of only the files the substrate asked for, which for
// every substrate so far is a few megabytes rather than the whole
// repository. It returns the commit the ref resolved to, which the artefact
// records so every version is reproducible and auditable (ADR-0020).
//
// The ref may be a tag or a branch: git resolves either, and the recorded
// commit is what pins it. A tag is the normal case, because an artefact
// version is a pinned thing, and a repository holding a tag and a branch of
// the same name can say which it means by passing the full
// "refs/tags/<name>" form.
//
// Fetching happens at import time only, on an operator's machine, with the
// result carried onward as the artefact; the platform itself never fetches
// at runtime (ADR-0019), and no fetched tree is vendored into this
// repository.
func Fetch(repoURL, ref, dir string, files []string) (commit string, err error) {
	if len(files) == 0 {
		return "", fmt.Errorf("no sparse-checkout patterns: a fetch that takes the whole repository is not what this pipeline is for")
	}
	steps := [][]string{
		{"init", "--quiet", "."},
		{"remote", "add", "origin", repoURL},
		append([]string{"sparse-checkout", "set", "--no-cone"}, files...),
		{"fetch", "--quiet", "--depth", "1", "origin", ref},
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
