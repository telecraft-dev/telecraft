// Package substrate is the one import pipeline (ADR-0020 §5) that every
// Catalogue-pattern substrate runs through.
//
// A Catalogue-pattern substrate is content somebody else maintains as
// ordinary git files, which the platform imports at a pinned ref into an
// atomic, versioned artefact: the Catalogue, walked out of
// opentelemetry-collector-contrib (ADR-0020), and the Schema Registry,
// walked out of an adopter's own registry repository (ADR-0034 §1). The two
// differ in what they read out of a tree and in nothing else, so everything
// else lives here: the sparse fetch, the pinned Source record, the
// deterministic encoding, the atomic idempotent write, the side-by-side
// version naming, and the strict load.
//
// There is one pipeline rather than one per substrate for the reason
// ADR-0020 §5 gives for having one at all. A second code path drifts from
// the first, and the drift stays invisible until an artefact one path
// accepts the other rejects. A substrate supplies four facts: what it is
// called, which files it needs checked out, what its artefact is named, and
// how to build one from a materialised tree. The pipeline supplies the rest.
//
// Nothing here fetches at runtime. An import runs on an operator's machine
// and the result travels as the artefact (ADR-0019), which is why the
// pipeline can also import a tree that arrived by other means, such as
// across an air gap.
package substrate

import (
	"fmt"
	"strings"
)

// Source records where one artefact version came from: the repository, the
// pinned ref, and the commit that ref resolved to. Recording the commit is
// what makes every artefact reproducible and auditable (ADR-0020).
type Source struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
	Commit     string `json:"commit,omitempty"`
}

// Artefact is one imported version, ready to be written. Every substrate's
// artefact is versioned by the ref it was imported at and encodes
// deterministically, because those two properties together are what make
// versions sit side by side and re-imports be no-ops.
type Artefact interface {
	// Version is the pinned ref this version is named for.
	Version() string

	// Encode renders the canonical artefact bytes. The encoding must be
	// deterministic: importing the same ref twice yields byte-identical
	// bytes, which is what makes idempotency testable and artefacts
	// diffable across an air gap.
	Encode() ([]byte, error)

	// Summary is the one-line count the pipeline reports on a write,
	// such as "412 components" or "37 groups".
	Summary() string
}

// Substrate is everything the pipeline cannot know: what a substrate is
// called, which files it needs out of the repository, what its artefacts
// are named, and how to turn a materialised tree into one.
type Substrate interface {
	// Name is how the substrate is written in reports, in the vocabulary
	// the glossary fixes: "Catalogue", "Schema Registry".
	Name() string

	// Files are the sparse-checkout patterns the fetch materialises. A
	// substrate reads a small part of a large repository, so the fetch
	// takes that part and nothing else.
	Files() []string

	// Prefix is the artefact file-name prefix, such as "catalogue-". The
	// ref and the .json suffix are the pipeline's.
	Prefix() string

	// Build imports the tree at root, already materialised at src.Ref,
	// returning the artefact and the coverage report saying what was
	// found, what was left out and why, and what looked like content but
	// carried none. Build fails closed: on any error the artefact is nil
	// rather than partial.
	Build(root string, src Source) (Artefact, fmt.Stringer, error)
}

// Identity reduces a clone URL to the repository identity an artefact
// records: "github.com/open-telemetry/opentelemetry-collector-contrib".
func Identity(url string) string {
	id := url
	for _, prefix := range []string{"https://", "http://", "git@"} {
		id = strings.TrimPrefix(id, prefix)
	}
	id = strings.Replace(id, ":", "/", 1)
	return strings.TrimSuffix(id, ".git")
}
