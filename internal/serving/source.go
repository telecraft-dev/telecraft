package serving

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Source is where the serving path gets its repo snapshot. Both
// implementations end in LoadSnapshot over a directory: the git dependency
// is git-the-tool, never git-the-service (ADR-0032 §3) — a local bare
// repository fully satisfies the transport floor, and a single binary plus
// a directory is a complete standalone instance.
type Source interface {
	// Snapshot fetches (where fetching applies) and compiles the estate at
	// its current head.
	Snapshot(ctx context.Context) (*Snapshot, error)
}

// DirSource serves an estate checkout already on disk — the standalone and
// local-development rung of ADR-0032 §3, and the air-gap posture
// (ADR-0019) in the same shape. Nothing is fetched; each snapshot re-reads
// the directory, so an external `git pull` is picked up on the next poll.
type DirSource struct {
	// Root is the estate checkout: the teams/ tree beside rendered/.
	Root string
}

// Snapshot compiles the directory as it stands. The head SHA is read from
// the surrounding git history when there is one; a plain directory serves
// fine without it — the artefacts carry their own commit stamps
// (ADR-0013).
func (d DirSource) Snapshot(ctx context.Context) (*Snapshot, error) {
	commit, err := git(ctx, d.Root, "rev-parse", "HEAD")
	if err != nil {
		commit = ""
	}
	return LoadSnapshot(d.Root, commit)
}

// GitSource fetches an estate repo with git-the-tool (ADR-0032 §3): one
// clone into Dir, then each snapshot fetches the remote HEAD and checks it
// out. The poll interval of the caller is the bounded staleness of
// ADR-0032 §1; a failed fetch surfaces as an error and the caller keeps
// serving the previous snapshot.
type GitSource struct {
	// URL is anything git clones: an SSH or HTTPS remote, or a local
	// `file:///…/estate.git` bare repository — the air-gap floor.
	URL string

	// Dir is where the clone lives. It is a cache of git, never a fork of
	// it: deleting it costs one re-clone.
	Dir string
}

// Snapshot brings Dir to the remote's current HEAD and compiles it.
func (g GitSource) Snapshot(ctx context.Context) (*Snapshot, error) {
	if _, err := os.Stat(filepath.Join(g.Dir, ".git")); err != nil {
		if _, err := git(ctx, "", "clone", "--quiet", g.URL, g.Dir); err != nil {
			return nil, err
		}
	} else {
		if _, err := git(ctx, g.Dir, "fetch", "--quiet", "origin", "HEAD"); err != nil {
			return nil, err
		}
		if _, err := git(ctx, g.Dir, "checkout", "--quiet", "--detach", "FETCH_HEAD"); err != nil {
			return nil, err
		}
	}
	commit, err := git(ctx, g.Dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	return LoadSnapshot(g.Dir, commit)
}

// git runs one git command, returning its trimmed stdout. Errors carry the
// command and its combined output — a failed fetch needs to say why.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
