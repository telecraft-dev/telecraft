// Command catalogue-import runs the Catalogue import pipeline (REQ-010,
// ADR-0020): it fetches opentelemetry-collector-contrib at a pinned release
// tag, walks every metadata.yaml, and writes one atomic, versioned Catalogue
// artefact plus a coverage report of what was found, excluded and missing.
//
// Usage:
//
//	go run ./cmd/catalogue-import -tag v0.158.0
//
// writes catalogues/catalogue-v0.158.0.json and prints the coverage report.
// Re-running against the same tag is idempotent: the artefact is
// byte-identical and left untouched. A different tag writes a new artefact
// beside the old one; existing Catalogue versions are retained, never
// replaced (ADR-0020 §9).
//
// The fetch is a sparse, depth-1 checkout of only metadata.yaml and go.mod
// (a few MB) into a temporary directory that is removed afterwards; the
// upstream tree is never vendored into this repository. An already-fetched
// tree, for example one carried across an air gap, can be imported
// offline with -source:
//
//	go run ./cmd/catalogue-import -tag v0.158.0 -source /path/to/contrib
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/catalogue"
)

const defaultRepo = "https://github.com/open-telemetry/opentelemetry-collector-contrib"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "catalogue-import:", err)
		os.Exit(1)
	}
}

func run() error {
	tag := flag.String("tag", "", "collector release tag to import, for example v0.158.0 (required)")
	out := flag.String("out", "catalogues", "directory the versioned artefact is written to")
	repo := flag.String("repo", defaultRepo, "source repository URL")
	source := flag.String("source", "", "import an existing checkout instead of fetching (offline path)")
	flag.Parse()

	if *tag == "" {
		return fmt.Errorf("-tag is required: each Catalogue is versioned against one collector release tag")
	}

	root := *source
	var commit string
	if root == "" {
		tmp, err := os.MkdirTemp("", "catalogue-import-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		fmt.Fprintf(os.Stderr, "fetching %s at %s (sparse, depth 1)...\n", *repo, *tag)
		commit, err = catalogue.Fetch(*repo, *tag, tmp)
		if err != nil {
			return err
		}
		root = tmp
	} else {
		var err error
		if commit, err = catalogue.Commit(root); err != nil {
			// A tree copied without its .git (an air-gap transfer) still
			// imports; the artefact then records the tag alone.
			fmt.Fprintf(os.Stderr, "note: %s is not a git checkout; the artefact will record no source commit\n", root)
			commit = ""
		}
	}

	src := catalogue.Source{Repository: repoIdentity(*repo), Ref: *tag, Commit: commit}
	cat, coverage, err := catalogue.Import(root, src)
	if err != nil {
		return err
	}

	fmt.Printf("Catalogue import of %s at %s", src.Repository, src.Ref)
	if commit != "" {
		fmt.Printf(" (%s)", commit[:min(12, len(commit))])
	}
	fmt.Println()
	fmt.Print(coverage)

	path, changed, err := cat.Write(*out)
	if err != nil {
		return err
	}
	if changed {
		fmt.Printf("wrote %s (%d components)\n", path, cat.Len())
	} else {
		fmt.Printf("%s already holds this import, so there is nothing to do\n", path)
	}
	return nil
}

// repoIdentity reduces a clone URL to the repository identity the artefact
// records: "github.com/open-telemetry/opentelemetry-collector-contrib".
func repoIdentity(url string) string {
	id := url
	for _, prefix := range []string{"https://", "http://", "git@"} {
		id = strings.TrimPrefix(id, prefix)
	}
	id = strings.Replace(id, ":", "/", 1)
	return strings.TrimSuffix(id, ".git")
}
