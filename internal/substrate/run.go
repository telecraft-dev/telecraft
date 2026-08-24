package substrate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Options is one import run. Every substrate takes the same ones, because
// the three transports of ADR-0020 §5 are the same three for all of them:
// fetch the repository, import a tree that arrived some other way, or
// re-import what is already on disk.
type Options struct {
	// Repo is the clone URL to fetch from. Ignored when Tree is set.
	Repo string

	// Ref is the tag or branch to import at, and the version the artefact
	// is named for. Required.
	Ref string

	// Tree imports an existing checkout instead of fetching: the offline
	// path, for a tree carried across an air gap (ADR-0019). Repo is
	// still required, because it is the provenance the artefact records.
	Tree string

	// Path is the content root within the checkout, for a repository that
	// keeps the substrate in a subdirectory. Empty means the whole tree.
	Path string

	// Out is the directory the versioned artefact is written to.
	Out string

	// Stdout takes the report, Stderr the progress. Both default to the
	// process's own.
	Stdout, Stderr io.Writer
}

// Result is what one import did.
type Result struct {
	// Path is the artefact file the import wrote or found already correct.
	Path string

	// Changed is false when the file already held exactly these bytes, so
	// re-importing the same ref reports the no-op rather than a write.
	Changed bool

	// Source is what the artefact records: repository, ref, and the commit
	// the ref resolved to.
	Source Source
}

// Run is the import pipeline: materialise the tree at a pinned ref, build
// the substrate's artefact from it, print the coverage report, and write the
// artefact beside the versions already installed.
//
// It fails closed at every step. A tree that does not materialise, a build
// that finds anything malformed, or an artefact that does not encode all
// stop the run, and nothing is written.
func Run(sub Substrate, opts Options) (Result, error) {
	stdout, stderr := opts.Stdout, opts.Stderr
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}

	if opts.Ref == "" {
		return Result{}, fmt.Errorf("no ref: each %s version is imported at one pinned ref", sub.Name())
	}
	if opts.Repo == "" {
		// Required even on the offline path: the repository is the
		// provenance the artefact records, and a tree that arrived across
		// an air gap still came from somewhere (ADR-0020).
		return Result{}, fmt.Errorf("no repository: every %s version records the repository it came from", sub.Name())
	}

	root := opts.Tree
	var commit string
	if root == "" {
		tmp, err := os.MkdirTemp("", "telecraft-import-*")
		if err != nil {
			return Result{}, err
		}
		defer os.RemoveAll(tmp)
		fmt.Fprintf(stderr, "fetching %s at %s (sparse, depth 1)...\n", opts.Repo, opts.Ref)
		if commit, err = Fetch(opts.Repo, opts.Ref, tmp, sub.Files()); err != nil {
			return Result{}, err
		}
		root = tmp
	} else {
		var err error
		if commit, err = Commit(root); err != nil {
			// A tree copied without its .git (an air-gap transfer) still
			// imports; the artefact then records the ref alone.
			fmt.Fprintf(stderr, "note: %s is not a git checkout, so the artefact will record no source commit\n", root)
			commit = ""
		}
	}

	src := Source{Repository: Identity(opts.Repo), Ref: opts.Ref, Commit: commit}
	artefact, coverage, err := sub.Build(filepath.Join(root, opts.Path), src)
	if err != nil {
		return Result{}, err
	}

	fmt.Fprintf(stdout, "%s import of %s at %s", sub.Name(), src.Repository, src.Ref)
	if commit != "" {
		fmt.Fprintf(stdout, " (%s)", commit[:min(12, len(commit))])
	}
	fmt.Fprintln(stdout)
	fmt.Fprint(stdout, coverage)

	path, changed, err := Write(artefact, opts.Out, sub.Prefix())
	if err != nil {
		return Result{}, err
	}
	if changed {
		fmt.Fprintf(stdout, "wrote %s (%s)\n", path, artefact.Summary())
	} else {
		fmt.Fprintf(stdout, "%s already holds this import, so there is nothing to do\n", path)
	}
	return Result{Path: path, Changed: changed, Source: src}, nil
}
