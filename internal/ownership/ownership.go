// Package ownership loads and validates the ownership model: the Team tree
// supplied through the teams.yaml seam (ADR-0017) and the owner every
// authored object carries (REQ-015, ADR-0016). It routes findings to the
// owner of the object each finding is about, and rolls compliance up the
// tree as ratio-plus-worst per finding kind — waived findings always
// visible, never a single blended number at any level.
//
// Loading is strict and fails closed, matching internal/requirements: an
// unknown field, a malformed document, a duplicate id or an ownerless
// authored object is a load error naming the file, never a silently
// unroutable finding. A finding that routes to nobody is this model's
// version of the lenient verdict: the problem exists and no one is paged.
package ownership

// OwnerID names an Owner: the lowest unit of management (ADR-0017).
type OwnerID string

// TeamID names a Team in the tree.
type TeamID string

// Owner is the accountable party attached to every authored object
// (ADR-0016). An Owner belongs to exactly one Team.
type Owner struct {
	ID   OwnerID
	Team TeamID
}

// Team is one node of the strict team tree (ADR-0017): at most one parent,
// never multi-parent — with two parents every roll-up would double-count.
type Team struct {
	ID       TeamID
	Name     string
	Parent   TeamID // empty at a root
	Owners   []OwnerID
	Children []TeamID
}

// Tree is the loaded, validated team hierarchy. It arrives through a seam —
// first-party is a reviewable teams.yaml in the estate repo — and is never
// owned by the platform (ADR-0017).
type Tree struct {
	Teams  map[TeamID]Team
	Owners map[OwnerID]Owner
}

// ObjectKind is the kind of an authored object — the ADR-0016 authored set —
// or, for finding subjects only, a collector.
type ObjectKind string

const (
	KindComponent   ObjectKind = "component"
	KindBlueprint   ObjectKind = "blueprint"
	KindTier        ObjectKind = "tier"
	KindHop         ObjectKind = "hop"
	KindPath        ObjectKind = "path"
	KindService     ObjectKind = "service"
	KindRequirement ObjectKind = "requirement"
	KindExemption   ObjectKind = "exemption"

	// KindCollector is valid only as a finding Subject. A collector is
	// derived, never authored and never ownable: it inherits owner and
	// policy from the Tier it matched into by selector, and where one
	// subset needs a different owner the Tier is split (ADR-0016).
	KindCollector ObjectKind = "collector"
)

// Authored reports whether k is in the ADR-0016 authored set — the kinds
// that carry an owner. A collector is deliberately not among them.
func (k ObjectKind) Authored() bool {
	switch k {
	case KindComponent, KindBlueprint, KindTier, KindHop, KindPath,
		KindService, KindRequirement, KindExemption:
		return true
	}
	return false
}

// Object is one authored object as the ownership model sees it: kind, id and
// the owner it carries. The full shape of each kind (what a Tier or a
// Blueprint holds) belongs to other packages — ownership is an attribute,
// not a parallel hierarchy (ADR-0016).
type Object struct {
	Kind  ObjectKind `yaml:"kind"`
	ID    string     `yaml:"id"`
	Owner OwnerID    `yaml:"owner"`
}

// Ref keys an authored object by kind and id.
type Ref struct {
	Kind ObjectKind
	ID   string
}

// Estate is the loaded, validated ownership view of one estate: the team
// tree plus every authored object and the owner it carries.
type Estate struct {
	Tree    Tree
	Objects map[Ref]Object
}

// Subtree returns team and every team beneath it, root first.
func (t Tree) Subtree(team TeamID) ([]TeamID, error) {
	root, ok := t.Teams[team]
	if !ok {
		return nil, errUnknownTeam(team)
	}
	out := []TeamID{root.ID}
	for i := 0; i < len(out); i++ {
		out = append(out, t.Teams[out[i]].Children...)
	}
	return out, nil
}
