package conformance

import (
	"fmt"
	"path/filepath"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
)

// ActiveSchemaRegistry resolves the Schema Registry version an estate has
// designated active, so an evaluation can judge each pinned
// schema-conformance scope against it beside its pin (ADR-0034 §2). It is
// the one resolution both judging surfaces share: the console snapshot and
// the CLI check mode call it with the same designation, so the two read one
// estate the same way. The library's own resolved copy is preferred,
// whether a requirement pins the active ref or tracks head; loading from
// the installed artefacts under dir is the fallback for an estate whose
// requirements all pin older versions.
//
// Nil with no error means there is nothing to judge drift against: no
// designation, or a library that references no Schema Registry at all. A
// designation naming a version that cannot be read is an error, not a
// silent nil: a run that shrugged it off would show pinned references clean
// against a bar nobody could check.
func ActiveSchemaRegistry(ref, dir string, lib requirements.Library) (*schemaregistry.Registry, error) {
	if ref == "" || len(lib.SchemaRegistries) == 0 {
		return nil, nil
	}
	if reg := lib.SchemaRegistries[ref]; reg != nil {
		return reg, nil
	}
	if reg := lib.SchemaRegistries[requirements.TrackHead]; reg != nil {
		return reg, nil
	}
	reg, err := schemaregistry.Load(filepath.Join(dir, schemaregistry.ArtefactName(ref)))
	if err != nil {
		return nil, fmt.Errorf("the estate designates Schema Registry version %q as active, and it cannot be read: %v. Import that version, or activate one that is installed", ref, err)
	}
	return reg, nil
}
