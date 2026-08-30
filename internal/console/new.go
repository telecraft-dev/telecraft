package console

import (
	"path/filepath"

	"github.com/telecraft-dev/telecraft/internal/allowlist"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/serving"
	"github.com/telecraft-dev/telecraft/pkg/ownership"
)

// BuildNewEstate assembles the document set of an estate that authors no
// Tier: the team tree it was created with, and empty everything else.
//
// It is a second entry point rather than a branch inside Build because the
// two answer different questions. Build reads the inputs a verdict is
// computed from and refuses every absence among them, because each of
// those refusals guards a verdict: an absent requirements library would
// score every Tier compliant against a floor nobody wrote, an absent
// rows.yaml would pass every check, an absent rendered tree would serve
// collectors a config the sources no longer describe. An estate with no
// Tier has no verdict for any of that to guard, so there is nothing for
// this to read and nothing for it to refuse (ADR-0086).
//
// Nothing is fabricated. The documents are the same projections Build
// produces, over an estate that genuinely holds one team and no objects,
// so every empty list here is a reading rather than a placeholder.
func BuildNewEstate(in Inputs) (Bundle, error) {
	tree, err := ownership.LoadTeams(filepath.Join(in.Root, ownership.TeamsFile))
	if err != nil {
		return Bundle{}, err
	}
	readings, err := in.readings()
	if err != nil {
		return Bundle{}, err
	}

	// The empty values stand in for the loads Build makes, one for one.
	// They are written out rather than left nil so that a projection
	// reaching for a map or a version finds an empty estate's answer and
	// not a panic.
	b := builder{
		in:       in,
		tree:     tree,
		active:   &catalogue.Catalogue{},
		policy:   &allowlist.Policy{},
		topo:     renderer.Topology{Tiers: map[string]renderer.Tier{}, Services: map[string]renderer.Service{}, Rollouts: map[string]renderer.Rollout{}},
		floors:   renderer.DefaultFloors(),
		lib:      requirements.Library{Requirements: map[string]requirements.Requirement{}},
		readings: readings,
		snapshot: &serving.Snapshot{},
		own:      ownership.Estate{Tree: tree, Objects: map[ownership.Ref]ownership.Object{}},
		now:      readings.AsOf,
	}
	return b.build()
}
