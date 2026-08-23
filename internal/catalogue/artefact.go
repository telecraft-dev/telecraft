package catalogue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	return "catalogue-" + ref + ".json"
}

// Write stores the Catalogue artefact under dir, named for its release tag.
// The write is atomic (the bytes land in a temp file first and are renamed
// into place), so a reader never sees a half-written Catalogue. If the file
// already holds exactly these bytes the write is skipped and changed is
// false: re-importing the same tag is a no-op, not a rewrite.
func (c *Catalogue) Write(dir string) (path string, changed bool, err error) {
	if strings.ContainsAny(c.Source.Ref, `/\`) {
		return "", false, fmt.Errorf("release tag %q cannot name an artefact file", c.Source.Ref)
	}
	encoded, err := c.Encode()
	if err != nil {
		return "", false, err
	}
	path = filepath.Join(dir, ArtefactName(c.Source.Ref))

	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, encoded) {
		return path, false, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, err
	}
	tmp, err := os.CreateTemp(dir, ArtefactName(c.Source.Ref)+".tmp")
	if err != nil {
		return "", false, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(encoded); err != nil {
		tmp.Close()
		return "", false, err
	}
	if err := tmp.Close(); err != nil {
		return "", false, err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", false, err
	}
	return path, true, nil
}

// Load reads and validates one Catalogue artefact. Loading fails closed,
// exactly like the requirements library: an artefact travels (bundled in a
// release, downloaded, or carried across an air gap, ADR-0020 §5) and a
// tampered or truncated one silently accepted would corrupt every judgement
// downstream. An unknown field, a duplicate key, an alias collision or a
// missing mandatory field is a load error naming the file, and the returned
// Catalogue is nil, never partially loaded.
func Load(path string) (*Catalogue, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var cat Catalogue
	if err := dec.Decode(&cat); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%s: trailing data after the catalogue document", path)
	}

	if err := cat.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	cat.index()
	return &cat, nil
}
