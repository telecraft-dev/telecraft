// Package allowlist loads, validates and evaluates the Allow-list policy:
// the subset of the Catalogue each Team may use (REQ-011, ADR-0021). The
// Catalogue states what exists; an Allow-list states what a Team may use;
// this package answers "may team T use component X?" and materialises each
// team's effective palette with total provenance.
//
// The policy composes down the team tree by narrowing-only inheritance: a
// Team's effective list is its parent's effective list intersected with its
// own declared list, and descendants can only subtract (ADR-0021 §2). The
// one widening mechanism is the Grant: an ancestor-authored, owned
// exception adding named Catalogue entries to a specific descendant Team's
// effective list, applying to that Team's subtree and narrowable below like
// anything else (§3). Absent any authored list, the effective list is the
// whole active Catalogue. Governance pressure comes from floors and
// findings, not an empty-by-default shop (§4).
//
// Allow-lists and Grants are authored files in the estate repo beside
// teams.yaml (§5), reviewed and versioned like everything else; they are
// never rows edited live in the instance database. Loading is strict and
// fails closed, matching pkg/ownership: an unknown field, an unknown
// team or owner, a grant without ancestor authority, or an entry matching
// nothing in the active Catalogue is a load error naming the file, because a
// silently dropped entry would widen or narrow a palette nobody reviewed.
//
// Entries are shapes, never literals (the normaliser spike verdict on issue
// #13): each selects components by exact class plus a type pattern, an exact
// name being the degenerate pattern. Enforcement is not here: the allow-list
// check hard-blocks at render and feeds the composer palette through the one
// shared evaluator (ADR-0022), which consumes this package's answers.
package allowlist

import (
	"fmt"
	"path"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/pkg/ownership"
)

// Entry is one Allow-list or Grant entry: a shape selecting Catalogue
// components, authored as "class/type-pattern". The class side is exact,
// because the unit of allowing is the component key (class, type) per
// ADR-0021 §1, and the type side is a pattern: `*` matches any run of characters, `?`
// exactly one, anything else itself. `receiver/otlp` names one component;
// `exporter/kafka*` a family; `processor/*` a class.
type Entry struct {
	Class catalogue.Class
	Type  string // type pattern, never empty
}

// parseEntry reads the authored "class/type-pattern" form. The pattern
// vocabulary is deliberately small: literals plus `*` and `?`. Character
// classes and escapes are rejected so that no authored entry can be a
// malformed pattern at match time: what loads, matches.
func parseEntry(s string) (Entry, error) {
	class, pattern, ok := strings.Cut(s, "/")
	if !ok || class == "" || pattern == "" {
		return Entry{}, fmt.Errorf("entry %q is not in the form class/type-pattern, for example receiver/otlp, exporter/kafka* or processor/*", s)
	}
	c := catalogue.Class(class)
	if !c.Pipeline() {
		return Entry{}, fmt.Errorf("entry %q: %q is not a class. Use one of receiver, processor, exporter, connector, or extension", s, class)
	}
	if i := strings.IndexAny(pattern, `[]\/`); i >= 0 {
		return Entry{}, fmt.Errorf("entry %q: the type pattern contains %q. A pattern holds only literal characters, * and ?", s, string(pattern[i]))
	}
	return Entry{Class: c, Type: pattern}, nil
}

// String renders the entry back in its authored form.
func (e Entry) String() string {
	return string(e.Class) + "/" + e.Type
}

// Matches reports whether the entry selects this Catalogue component. The
// pattern is tried against the canonical type and the deprecated_type alias.
// Aliases resolve on every lookup (ADR-0020 §3), so an entry written
// against the historical name keeps selecting the component it always did.
func (e Entry) Matches(c catalogue.Component) bool {
	if c.Class != e.Class {
		return false
	}
	// parseEntry rejected every pattern path.Match can error on.
	if ok, _ := path.Match(e.Type, c.Type); ok {
		return true
	}
	if c.DeprecatedType != "" {
		ok, _ := path.Match(e.Type, c.DeprecatedType)
		return ok
	}
	return false
}

// AllowList is one Team's declared list: the entries that intersect the
// parent's effective list to form this Team's (ADR-0021 §2). At most one
// exists per Team. Declaring none at all means "inherit unchanged"; an
// empty list is rejected at load, because it would ban everything and the
// default-deny posture is deliberately not built in v1 (§4).
type AllowList struct {
	Team  ownership.TeamID
	Owner ownership.OwnerID
	Allow []Entry
}

// matches reports whether any entry of the list selects the component.
func (l AllowList) matches(c catalogue.Component) bool {
	for _, e := range l.Allow {
		if e.Matches(c) {
			return true
		}
	}
	return false
}

// GrantID names one Grant. Everything a team may use traces to either the
// root list surviving intersection or a named Grant (ADR-0021 §3). The id
// is that name, carried through to the effective palette's provenance.
type GrantID string

// Grant is an ancestor-authored scoped exception: it adds the components its
// entries select to the target Team's effective list, applies to that Team's
// subtree, and can be narrowed back out below like anything else (ADR-0021
// §3). A Grant is an authored object (owned, versioned, reviewable), and
// its authority is its owner's team, which must be a proper ancestor of the
// target: additions are the ancestor's call, by design.
type Grant struct {
	ID    GrantID
	Owner ownership.OwnerID
	Team  ownership.TeamID // the target: the Team (and subtree) widened
	Adds  []Entry
}

// matches reports whether any entry of the grant selects the component.
func (g Grant) matches(c catalogue.Component) bool {
	for _, e := range g.Adds {
		if e.Matches(c) {
			return true
		}
	}
	return false
}

// Policy is the loaded, validated Allow-list policy for one estate, bound to
// the team tree it was validated against and the active Catalogue version
// its entries were checked to select from. Judging with a different
// catalogue than the one validated against would let entries silently match
// nothing, so the binding is part of the type.
type Policy struct {
	Lists  map[ownership.TeamID]AllowList
	Grants map[GrantID]Grant

	tree     ownership.Tree
	cat      *catalogue.Catalogue
	byTarget map[ownership.TeamID][]Grant // per target team, in GrantID order
}

// Catalogue is the version ref of the Catalogue this policy validates and
// judges against.
func (p *Policy) Catalogue() string {
	return p.cat.Version()
}
