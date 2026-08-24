// Command schema-registry-import runs the Schema Registry half of the one
// import pipeline (REQ-022, ADR-0020, ADR-0034 §1): it fetches an adopter's
// custom Weaver registry at a pinned ref, reads every model file out of it,
// and writes one atomic, versioned Schema Registry artefact plus a coverage
// report of what was found, what was left out, and which references come
// from a dependency registry.
//
// Usage:
//
//	go run ./cmd/schema-registry-import -repo https://git.example/registry -ref v1.4.0
//
// writes schema-registries/schema-registry-v1.4.0.json and prints the
// coverage report. Re-running against the same ref is idempotent: the
// artefact is byte-identical and left untouched. A different ref writes a
// new artefact beside the old one; installed versions are retained, never
// replaced (ADR-0020 §9).
//
// A repository that keeps its registry in a subdirectory says so with -path,
// which is where the registry manifest lives:
//
//	go run ./cmd/schema-registry-import -repo … -ref v1.4.0 -path registry
//
// The fetch is a sparse, depth-1 checkout of only the YAML into a temporary
// directory that is removed afterwards; no fetched tree is vendored into
// this repository. An already-fetched tree, for example one carried across
// an air gap, is imported offline with -source.
//
// The import reads registry content out of git. It runs no registry
// toolchain: REQ-003 is configurations, never binaries, and ADR-0034 §5 has
// the adopter deploy the upstream tooling themselves.
//
// The Catalogue is the other substrate on this pipeline; see
// cmd/catalogue-import.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
	"github.com/telecraft-dev/telecraft/internal/substrate"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "schema-registry-import:", err)
		os.Exit(1)
	}
}

func run() error {
	ref := flag.String("ref", "", "registry version to import: a tag or branch in the registry repository (required)")
	repo := flag.String("repo", "", "registry repository URL, which the artefact records as its provenance (required)")
	path := flag.String("path", "", "registry root within the repository, where the registry manifest lives")
	out := flag.String("out", "schema-registries", "directory the versioned artefact is written to")
	source := flag.String("source", "", "import an existing checkout instead of fetching (offline path)")
	flag.Parse()

	if *ref == "" {
		return fmt.Errorf("-ref is required: each Schema Registry version is imported at one pinned ref")
	}
	if *repo == "" {
		return fmt.Errorf("-repo is required: every Schema Registry version records the repository it came from")
	}

	_, err := substrate.Run(schemaregistry.Substrate{}, substrate.Options{
		Repo: *repo,
		Ref:  *ref,
		Tree: *source,
		Path: *path,
		Out:  *out,
	})
	return err
}
