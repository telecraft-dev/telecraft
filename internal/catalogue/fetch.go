package catalogue

import (
	"fmt"

	"github.com/telecraft-dev/telecraft/internal/substrate"
)

// Substrate is the Catalogue's half of the one import pipeline
// (ADR-0020 §5): what a Catalogue is called, which upstream files it needs
// checked out, what its artefacts are named, and how to walk a materialised
// collector source tree into one. Everything else, the fetch, the atomic
// idempotent write, the side-by-side version naming and the strict load,
// belongs to the pipeline and is shared with every other substrate.
type Substrate struct{}

func (Substrate) Name() string { return "Catalogue" }

// Files are the sparse-checkout patterns a Catalogue import needs: every
// metadata.yaml, and the sibling go.mod that is the discovery anchor. A few
// megabytes, not the whole repository.
func (Substrate) Files() []string { return []string{"**/metadata.yaml", "**/go.mod"} }

func (Substrate) Prefix() string { return "catalogue-" }

// Build walks the tree at root and returns the Catalogue for src.Ref with
// its coverage report.
func (Substrate) Build(root string, src substrate.Source) (substrate.Artefact, fmt.Stringer, error) {
	cat, cov, err := Import(root, src)
	if err != nil {
		return nil, nil, err
	}
	return cat, cov, nil
}

// Fetch materialises the collector source tree for one pinned release tag
// into dir, sparsely and at depth 1, and returns the commit the tag resolved
// to. It is the shared pipeline's fetch with the Catalogue's file patterns.
func Fetch(repoURL, tag, dir string) (commit string, err error) {
	return substrate.Fetch(repoURL, tag, dir, Substrate{}.Files())
}

// Commit resolves the HEAD commit of an existing checkout, for imports run
// against a pre-fetched tree.
func Commit(dir string) (string, error) { return substrate.Commit(dir) }
