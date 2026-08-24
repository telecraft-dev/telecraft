package schemaregistry

import (
	"encoding/json"
	"fmt"

	"github.com/telecraft-dev/telecraft/internal/substrate"
)

// FormatVersion is the artefact format this package writes and reads. The
// artefact is a public, versioned contract (ADR-0020): it must be validated
// on import and evolved compatibly, so the version is explicit in every file.
const FormatVersion = 1

// Encode renders the Schema Registry as its canonical artefact bytes. The
// encoding is deterministic (groups in id order, attributes and enum members
// in key order, JSON object keys sorted, no timestamps), so importing the
// same ref twice yields byte-identical artefacts, which is what makes
// idempotency testable and versions diffable against each other.
func (r *Registry) Encode() ([]byte, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	sorted := Registry{
		FormatVersion: r.FormatVersion,
		Source:        r.Source,
		Manifest:      r.Manifest,
		Groups:        append([]Group(nil), r.Groups...),
	}
	sorted.index()
	out, err := json.MarshalIndent(&sorted, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// ArtefactName is the file name one Schema Registry version lives under.
// Naming by ref is what lets versions sit side by side: installed versions
// are retained, never replaced (ADR-0020 §9), which is what an activation
// impact report between two of them will read.
func ArtefactName(ref string) string {
	return substrate.Name(Substrate{}.Prefix(), ref)
}

// Write stores the Schema Registry artefact under dir, named for its ref.
// The write is atomic and idempotent: the bytes land in a temp file first
// and are renamed into place, so a reader never sees a half-written
// registry, and a file that already holds exactly these bytes is left alone
// with changed false.
func (r *Registry) Write(dir string) (path string, changed bool, err error) {
	return substrate.Write(r, dir, Substrate{}.Prefix())
}

// Load reads and validates one Schema Registry artefact. Loading fails
// closed, exactly as the Catalogue's does: an artefact travels, and a
// tampered or truncated one silently accepted would corrupt every
// schema-conformance judgement made against it. An unknown field, a
// duplicate group or attribute id, or a missing mandatory field is a load
// error naming the file, and the returned Registry is nil, never partially
// loaded.
func Load(path string) (*Registry, error) {
	var reg Registry
	if err := substrate.LoadStrict(path, "schema registry", &reg); err != nil {
		return nil, err
	}
	if err := reg.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	reg.index()
	return &reg, nil
}
