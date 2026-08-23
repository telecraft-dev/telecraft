package blueprint

import (
	"fmt"

	"github.com/telecraft-dev/telecraft/internal/catalogue"
)

// FindingKind separates the two problems this package can surface about a
// structurally valid Blueprint. Both are findings in the ADR-0022 sense:
// visible, owner-routed, never a block. Mechanical render refusal is the
// renderer's job, and policy hard-blocks only on allow-list violations.
type FindingKind string

const (
	// KindReference marks a Component reference that cannot deliver what it
	// promises: the Component or the pinned version is missing or retracted,
	// or the referenced class cannot live where the reference puts it.
	KindReference FindingKind = "reference"

	// KindOrdering marks a lane whose explicit order contradicts ordering
	// wisdom keyed on catalogue types (ADR-0024 §6).
	KindOrdering FindingKind = "ordering"
)

// Finding is one visible-but-not-fatal problem with a loaded Blueprint. It
// carries the Blueprint id so routing can page that Blueprint's owner
// (ADR-0016 §4) and the lane so the fix is a one-list edit.
type Finding struct {
	Kind      FindingKind
	Blueprint string // Blueprint id
	Lane      string // a signal lane name, or "extensions"
	Message   string
}

// ReferenceFindings resolves every Component reference in every Blueprint
// and reports the ones that cannot deliver: a shared Component nobody
// provides (missing, or retracted from the estate), a pin ahead of the
// owning team's current version (that version does not exist at head, the
// legible trace of a retraction or a typo), and a class that cannot live in
// the lane that references it.
//
// These are findings, never load errors, because consumers hold a
// reference, never a copy: the content that broke lives in another team's
// file, and one team's retraction must not stop every other team's load.
// Load calls this, so the acceptance shape is literal: a dangling pin is a
// load-time finding. A pin merely *behind* head is deliberately absent
// here: that is `library_drift`, a different diagnosis with its own
// detection (ADR-0026).
func (e Estate) ReferenceFindings() []Finding {
	var out []Finding
	for _, b := range e.SortedBlueprints() {
		for _, l := range b.lanes() {
			for _, entry := range l.entries {
				ref := entry.Reference()
				c, ok := e.resolve(b, ref)
				if !ok {
					out = append(out, Finding{KindReference, b.ID(), l.name,
						fmt.Sprintf("references %s, which no shared Component provides: it is missing or retracted. Nothing renders in its place until the reference changes or the owning team restores it", ref)})
					continue
				}
				if ref.Pin > c.Version {
					out = append(out, Finding{KindReference, b.ID(), l.name,
						fmt.Sprintf("pins %s, but the owning team's current version is %d. The pinned version is missing or retracted, so move the pin to a version that exists", ref, c.Version)})
				}
				switch {
				case l.name == ExtensionsLane && c.Class != catalogue.Extension:
					out = append(out, Finding{KindReference, b.ID(), l.name,
						fmt.Sprintf("references %s, a %s, but only extensions live in the extensions block. Move it to a signal lane", ref, c.Class)})
				case l.name != ExtensionsLane && c.Class == catalogue.Extension:
					out = append(out, Finding{KindReference, b.ID(), l.name,
						fmt.Sprintf("references %s, an extension. Extensions are collector-wide and live in the extensions block, never in a signal lane", ref)})
				}
			}
		}
	}
	return out
}
