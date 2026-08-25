package activation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/ownership"
	"github.com/telecraft-dev/telecraft/internal/renderer"
)

// CatalogueInputs is everything the Catalogue impact report reads: the two
// versions, and the estate the difference between them lands on.
//
// The report is a pure function of these, as the render and the drift
// detection are of theirs. Nothing here reads a backend: what a Catalogue
// activation changes is judgement over authored configuration (ADR-0020 §8),
// and authored configuration is all in the tree.
type CatalogueInputs struct {
	// From is the active version, nil when no version has ever been active.
	From *catalogue.Catalogue

	// To is the candidate version.
	To *catalogue.Catalogue

	Estate   blueprint.Estate
	Topology renderer.Topology
	Tree     ownership.Tree
	Floors   renderer.FloorPolicy
}

// use records one Catalogue key a Blueprint configures, and the signals that
// Blueprint routes through it. A key configured by two Blueprints is one
// entry with two uses: the report is about the key, and says whose.
type use struct {
	uses    map[string]string // blueprint id -> team id
	signals map[string]bool
}

// CatalogueImpact computes what changes when a candidate Catalogue version
// becomes the active one (ADR-0020 §6): components in use that the candidate
// removes or newly deprecates, by Blueprint and Team, and stability changes
// that take a component in use under the floor its Tier is held to.
//
// The floor half re-runs the render's own breach enumeration under each
// version and reports what is newly breaching, so the report and the finding
// an operator sees after activating can never disagree about what a breach
// is (the same reason the drift detection calls it rather than reimplementing
// it).
//
// A first activation has no version to diff against. It reports what the
// candidate holds for this estate instead: components in use it does not
// carry, entries in use it marks deprecated, and the floors the estate
// breaches under it. Reporting a change of nothing would read as an
// all-clear on a question nobody asked.
func CatalogueImpact(in CatalogueInputs) (Report, error) {
	if in.To == nil {
		return Report{}, fmt.Errorf("no candidate Catalogue: an impact report is computed from the version being activated")
	}
	rep := Report{Kind: Catalogue, To: in.To.Version()}
	if in.From != nil {
		rep.From = in.From.Version()
	}
	if rep.From == rep.To && rep.From != "" {
		return Report{}, fmt.Errorf("Catalogue %s is already active, so there is nothing to report", rep.To)
	}

	inUse := catalogueUse(in.Estate)
	for _, key := range sortedUseKeys(inUse) {
		u := inUse[key]
		class, typ := splitKey(key)
		candidate, held := in.To.Lookup(class, typ)
		if !held {
			// A key the active version holds and the candidate does not is
			// a removal. On a first activation nothing held it before, so
			// the same absence reads as a gap in the version rather than
			// as something taken away.
			if in.From != nil {
				if _, was := in.From.Lookup(class, typ); !was {
					continue
				}
			}
			detail := "is not in this version"
			if in.From != nil {
				detail = "is removed"
			}
			rep.Changes = append(rep.Changes, Change{
				Kind:    Removed,
				Subject: key,
				Detail:  detail,
				Uses:    usesOf(u, in.Tree),
			})
			continue
		}
		rep.Changes = append(rep.Changes, deprecations(key, u, class, typ, in.From, candidate, in.Tree)...)
	}

	crossings, err := floorCrossings(in)
	if err != nil {
		return Report{}, err
	}
	rep.Changes = append(rep.Changes, crossings...)
	rep.sortChanges()
	return rep, nil
}

// deprecations reports the signals the candidate version marks deprecated or
// unmaintained on a component in use, and the active version did not. It is
// per signal because stability is per signal: a component beta for logs and
// deprecated for profiles is deprecated only for the estate that routes
// profiles through it.
func deprecations(key string, u use, class catalogue.Class, typ string, from *catalogue.Catalogue, candidate catalogue.Component, tree ownership.Tree) []Change {
	var previous catalogue.Component
	if from != nil {
		previous, _ = from.Lookup(class, typ)
	}

	var out []Change
	for _, signal := range sortedSignals(u.signals) {
		level, supported := candidate.StabilityFor(signal)
		if !supported || !endOfLife(level) {
			continue
		}
		if was, ok := previous.StabilityFor(signal); ok && endOfLife(was) {
			continue
		}
		detail := fmt.Sprintf("is %s for %s in this version", level, signal)
		if note, ok := candidate.Deprecation[signal]; ok && note.Migration != "" {
			detail += fmt.Sprintf(". The upstream migration note is: %s", strings.TrimSuffix(note.Migration, "."))
		}
		out = append(out, Change{
			Kind:    Deprecated,
			Subject: key,
			Detail:  detail,
			Uses:    usesOf(u, tree),
		})
	}
	return out
}

// endOfLife reports the two lifecycle end-states. They are not rungs on the
// maturity ladder (ADR-0023 §6), so a floor never judges them and this report
// is where an estate hears about them.
func endOfLife(l catalogue.Level) bool {
	return l == catalogue.Deprecated || l == catalogue.Unmaintained
}

// floorCrossings reports the breaches the candidate version introduces: a
// breach under the candidate that the active version did not produce. On a
// first activation every breach is new, because no version judged the estate
// before.
func floorCrossings(in CatalogueInputs) ([]Change, error) {
	if err := in.Floors.Validate(); err != nil {
		return nil, err
	}
	var out []Change
	for _, tier := range in.Topology.SortedTiers() {
		bp, ok := in.Estate.Blueprint(tier.Binding().ID())
		if !ok {
			continue
		}
		before := map[string]renderer.FloorBreach{}
		if in.From != nil {
			breaches, err := in.Floors.Breaches(in.Topology, in.From, in.Estate, tier, bp)
			if err != nil {
				return nil, fmt.Errorf("tier %q: cannot derive judgement strictness: %w", tier.ID(), err)
			}
			for _, b := range breaches {
				before[breachKey(b)] = b
			}
		}
		after, err := in.Floors.Breaches(in.Topology, in.To, in.Estate, tier, bp)
		if err != nil {
			return nil, fmt.Errorf("tier %q: cannot derive judgement strictness: %w", tier.ID(), err)
		}
		for _, b := range after {
			if _, existed := before[breachKey(b)]; existed {
				continue
			}
			out = append(out, Change{
				Kind:    FloorCrossing,
				Subject: fmt.Sprintf("%s/%s", b.Component.Class, b.Component.Type),
				Detail: fmt.Sprintf("is %s for %s in this version, under the %s floor %s holds in %s for Service Class %s",
					b.Level, b.Signal, b.Floor, tier.ID(), tier.Environment, b.Class),
				Uses: []Use{{Blueprint: bp.ID(), Team: teamName(in.Tree, bp.Team)}},
			})
		}
	}
	return out, nil
}

func breachKey(b renderer.FloorBreach) string {
	return fmt.Sprintf("%s/%s/%s/%s", b.Component.Class, b.Component.Type, b.Component.ID(), b.Signal)
}

// catalogueUse walks every Blueprint in the estate and records which
// Catalogue keys it configures, and on which signals. Extensions are
// collector-wide rather than per signal, so they contribute a key with no
// signal: a deprecation on an extension is reported on the entry, and there
// is no lane to name.
func catalogueUse(est blueprint.Estate) map[string]use {
	out := map[string]use{}
	record := func(bp blueprint.Blueprint, c catalogue.Class, typ, signal string) {
		if typ == "" {
			return
		}
		key := string(c) + "/" + typ
		u, ok := out[key]
		if !ok {
			u = use{uses: map[string]string{}, signals: map[string]bool{}}
		}
		u.uses[bp.ID()] = bp.Team
		if signal != "" {
			u.signals[signal] = true
		}
		out[key] = u
	}

	for _, bp := range est.SortedBlueprints() {
		for _, signal := range blueprint.Signals {
			for _, entry := range bp.Lane(signal) {
				if comp, ok := resolve(est, bp, entry); ok {
					record(bp, comp.Class, comp.Type, string(signal))
				}
			}
		}
		for _, entry := range bp.Extensions {
			if comp, ok := resolve(est, bp, entry); ok {
				record(bp, comp.Class, comp.Type, "")
			}
		}
	}
	return out
}

// resolve turns one lane entry into the Component it references, local or
// shared. A reference that resolves to nothing is the loader's finding, not
// this report's: a report that invented an entry for it would attribute a
// change to a Blueprint nobody could fix.
func resolve(est blueprint.Estate, bp blueprint.Blueprint, entry blueprint.Entry) (blueprint.Component, bool) {
	ref := entry.Reference()
	if ref.Local() {
		return bp.Local(ref.Name)
	}
	return est.Component(ref.ID())
}

// usesOf renders one key's users in stable order, Blueprint id first so two
// reports over the same estate read identically.
func usesOf(u use, tree ownership.Tree) []Use {
	ids := make([]string, 0, len(u.uses))
	for id := range u.uses {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Use, 0, len(ids))
	for _, id := range ids {
		out = append(out, Use{Blueprint: id, Team: teamName(tree, u.uses[id])})
	}
	return out
}

// teamName is the Team's authored name where the tree carries one, and its
// id otherwise. A report names the Team a person would recognise.
func teamName(tree ownership.Tree, team string) string {
	t, ok := tree.Teams[ownership.TeamID(team)]
	if !ok || t.Name == "" {
		return team
	}
	return t.Name
}

func sortedUseKeys(m map[string]use) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedSignals(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func splitKey(key string) (catalogue.Class, string) {
	class, typ, _ := strings.Cut(key, "/")
	return catalogue.Class(class), typ
}
