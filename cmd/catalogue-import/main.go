// Command catalogue-import runs the Catalogue half of the one import
// pipeline (REQ-010, ADR-0020): it fetches opentelemetry-collector-contrib
// at a pinned release tag, walks every metadata.yaml, and writes one atomic,
// versioned Catalogue artefact plus a coverage report of what was found,
// excluded and missing.
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
//
// The Schema Registry is the other substrate on this pipeline; see
// cmd/registry-import.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/substrate"
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

	_, err := substrate.Run(catalogue.Substrate{}, substrate.Options{
		Repo: *repo,
		Ref:  *tag,
		Tree: *source,
		Out:  *out,
	})
	return err
}
