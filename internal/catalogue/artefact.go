package catalogue

import (
	"encoding/json"
	"fmt"

	"github.com/telecraft-dev/telecraft/internal/substrate"
)

// FormatVersion is the artefact format this package writes and reads. The
// artefact is a public, versioned contract (ADR-0020): it must be validated
// on import and evolved compatibly, so the version is explicit in every file.
const FormatVersion = 1

// Encode renders the Catalogue as its canonical artefact bytes. The
// encoding is deterministic (components in (class, type) order, JSON object
// keys sorted, no timestamps), so importing the same tag twice yields
// byte-identical artefacts, which is what makes idempotency testable and
// artefacts diffable across an air gap.
func (c *Catalogue) Encode() ([]byte, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	sorted := Catalogue{
		FormatVersion: c.FormatVersion,
		Source:        c.Source,
		Components:    append([]Component(nil), c.Components...),
	}
	sorted.index()
	out, err := json.MarshalIndent(&sorted, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// ArtefactName is the file name one Catalogue version lives under. Naming
// by tag is what lets versions sit side by side: installed catalogues are
// retained, never replaced (ADR-0020 §9).
func ArtefactName(ref string) string {
	return substrate.Name(Substrate{}.Prefix(), ref)
}

// Write stores the Catalogue artefact under dir, named for its release tag.
// The write is atomic and idempotent: the bytes land in a temp file first
// and are renamed into place, so a reader never sees a half-written
// Catalogue, and a file that already holds exactly these bytes is left
// alone with changed false.
func (c *Catalogue) Write(dir string) (path string, changed bool, err error) {
	return substrate.Write(c, dir, Substrate{}.Prefix())
}

// Load reads and validates one Catalogue artefact. Loading fails closed,
// exactly like the requirements library: an artefact travels (bundled in a
// release, downloaded, or carried across an air gap, ADR-0020 §5) and a
// tampered or truncated one silently accepted would corrupt every judgement
// downstream. An unknown field, a duplicate key, an alias collision or a
// missing mandatory field is a load error naming the file, and the returned
// Catalogue is nil, never partially loaded.
func Load(path string) (*Catalogue, error) {
	var cat Catalogue
	if err := substrate.LoadStrict(path, "catalogue", &cat); err != nil {
		return nil, err
	}
	if err := cat.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	cat.index()
	return &cat, nil
}
