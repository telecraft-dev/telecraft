package allowlist

import (
	"fmt"
	"slices"

	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/ownership"
)

// Origin says why a component is in an effective palette. The audit chain is
// total (ADR-0021 §3): everything a team may use traces to either the
// authored lists surviving intersection, the default posture when nobody
// authored one, or a named Grant.
type Origin string

const (
	// OriginDefault: no Allow-list is declared anywhere on the team's chain,
	// so the effective list is the whole active Catalogue (ADR-0021 §4).
	OriginDefault Origin = "default-allow"

	// OriginAllowList: the component survived every declared list on the
	// chain from the root down to the team.
	OriginAllowList Origin = "allow-list"

	// OriginGrant: a named Grant admitted the component; the lists alone
	// would exclude it.
	OriginGrant Origin = "grant"
)

// PaletteEntry is one allowed component with its provenance. For a granted
// component the Grant is named, with the granting team (the grant owner's
// team, the authority) and the target team it was attached to.
type PaletteEntry struct {
	Component catalogue.Component
	Origin    Origin

	// Grant provenance, set only when Origin is OriginGrant.
	Grant     GrantID
	GrantedBy ownership.TeamID
	GrantedTo ownership.TeamID
}

// Palette is one team's effective palette: every component of the active
// Catalogue the team may use, in the Catalogue's (class, type) order, each
// with provenance. It is the membership the composer's Palette presents and
// the render gate enforces (ADR-0022). Evaluator verdicts like floor
// greying layer on top; nothing here judges stability.
type Palette struct {
	Team      ownership.TeamID
	Catalogue string
	Entries   []PaletteEntry
}

// EffectivePalette computes one team's effective palette per ADR-0021.
// Walking the chain from the root down to the team, each declared list
// intersects, then each Grant targeting that team unions back in, so a
// Grant widens from its target's subtree downward and is narrowed back out
// by a descendant's list like anything else.
func (p *Policy) EffectivePalette(team ownership.TeamID) (Palette, error) {
	chain, err := p.chain(team)
	if err != nil {
		return Palette{}, err
	}

	anyList := false
	for _, u := range chain {
		if _, ok := p.Lists[u]; ok {
			anyList = true
			break
		}
	}

	pal := Palette{Team: team, Catalogue: p.cat.Version()}
	for _, comp := range p.cat.Components {
		allowed := true
		var via *Grant
		for _, u := range chain {
			if l, ok := p.Lists[u]; ok && !l.matches(comp) {
				// The intersection removes it, including a component a
				// Grant higher up admitted: narrowed back out below.
				allowed, via = false, nil
			}
			if !allowed {
				for _, g := range p.byTarget[u] {
					if g.matches(comp) {
						allowed, via = true, &g
						break
					}
				}
			}
		}
		if !allowed {
			continue
		}

		e := PaletteEntry{Component: comp}
		switch {
		case via != nil:
			e.Origin = OriginGrant
			e.Grant = via.ID
			e.GrantedBy = p.tree.Owners[via.Owner].Team
			e.GrantedTo = via.Team
		case anyList:
			e.Origin = OriginAllowList
		default:
			e.Origin = OriginDefault
		}
		pal.Entries = append(pal.Entries, e)
	}
	return pal, nil
}

// Allows answers the ADR-0021 question: may this team use the component
// (class, typ)? The lookup resolves deprecated_type aliases like every
// Catalogue lookup. A component the Catalogue does not know is not allowed;
// whether an unknown component is additionally its own finding is the
// caller's judgement, not the Allow-list's.
func (p *Policy) Allows(team ownership.TeamID, class catalogue.Class, typ string) (bool, error) {
	comp, ok := p.cat.Lookup(class, typ)
	if !ok {
		return false, nil
	}
	pal, err := p.EffectivePalette(team)
	if err != nil {
		return false, err
	}
	for _, e := range pal.Entries {
		if e.Component.Class == comp.Class && e.Component.Type == comp.Type {
			return true, nil
		}
	}
	return false, nil
}

// chain returns the teams from the root down to team, inclusive.
func (p *Policy) chain(team ownership.TeamID) ([]ownership.TeamID, error) {
	if _, ok := p.tree.Teams[team]; !ok {
		return nil, fmt.Errorf("no team %q in the tree", team)
	}
	var out []ownership.TeamID
	for id := team; id != ""; id = p.tree.Teams[id].Parent {
		out = append(out, id)
	}
	slices.Reverse(out)
	return out, nil
}
