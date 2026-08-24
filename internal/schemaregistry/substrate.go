package schemaregistry

import (
	"fmt"

	"github.com/telecraft-dev/telecraft/internal/substrate"
)

// Substrate is the Schema Registry's half of the one import pipeline
// (ADR-0020 §5, ADR-0034 §1): what it is called, which files it needs
// checked out, what its artefacts are named, and how to walk a materialised
// registry tree into one. Everything else belongs to the pipeline and is
// shared with the Catalogue, which is the point of there being one.
type Substrate struct{}

// Name is the substrate's full name. It is always written "Schema
// Registry": a bare "registry" is ambiguous, because RegistryProvider names
// an unrelated seam (ADR-0034 §1).
func (Substrate) Name() string { return "Schema Registry" }

// Files are the sparse-checkout patterns a Schema Registry import needs. A
// registry is YAML model files, and where under the tree they sit is the
// adopter's business, so the fetch takes the YAML and leaves everything
// else: generated documentation, code, and whatever else the repository
// keeps beside its registry.
func (Substrate) Files() []string { return []string{"**/*.yaml", "**/*.yml"} }

func (Substrate) Prefix() string { return "schema-registry-" }

// Build walks the registry tree at root and returns the Schema Registry
// version for src.Ref with its coverage report.
func (Substrate) Build(root string, src substrate.Source) (substrate.Artefact, fmt.Stringer, error) {
	reg, cov, err := Import(root, src)
	if err != nil {
		return nil, nil, err
	}
	return reg, cov, nil
}
