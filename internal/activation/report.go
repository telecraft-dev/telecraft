package activation

import (
	"fmt"
	"sort"
	"strings"
)

// ChangeKind is what one entry of an impact report says happened. The
// vocabulary is per substrate and closed: the Catalogue's three changes are
// the ones ADR-0020 §6 names, and the Schema Registry's are the ones
// ADR-0034's Consequences ask a registry activation to report.
type ChangeKind string

const (
	// Removed is a Catalogue entry that Blueprints use and the candidate
	// version does not hold.
	Removed ChangeKind = "removed"

	// Deprecated is a Catalogue entry in use, or a Schema Registry group or
	// attribute, that the candidate version marks deprecated and the active
	// one did not.
	Deprecated ChangeKind = "deprecated"

	// FloorCrossing is a stability change that takes a component in use
	// under the stability floor its Tier is held to.
	FloorCrossing ChangeKind = "floor_crossing"

	// AttributeAdded and AttributeRemoved are the two halves of the Schema
	// Registry's attribute diff.
	AttributeAdded   ChangeKind = "attribute_added"
	AttributeRemoved ChangeKind = "attribute_removed"

	// LevelTightened is a requirement level the candidate version demands
	// more strictly than the active one.
	LevelTightened ChangeKind = "level_tightened"

	// NewlyFailing is the estate half of a Schema Registry report: Services
	// that pass a requirement level under the active version and fail it
	// under the candidate.
	NewlyFailing ChangeKind = "newly_failing"
)

// order ranks the change kinds for report order: what fails now, then what
// is about to, then what merely moved.
var order = map[ChangeKind]int{
	NewlyFailing:     0,
	Removed:          1,
	FloorCrossing:    2,
	Deprecated:       3,
	LevelTightened:   4,
	AttributeRemoved: 5,
	AttributeAdded:   6,
}

// Use is one authored object a Catalogue change lands on, with the Team
// accountable for it. ADR-0020 §6 asks the report to say which Blueprints
// and which Teams, because "two components you run are deprecated" is not
// actionable until it says whose.
type Use struct {
	Blueprint string
	Team      string
}

func (u Use) String() string {
	if u.Team == "" {
		return u.Blueprint
	}
	return fmt.Sprintf("%s (%s)", u.Blueprint, u.Team)
}

// Change is one entry of an impact report.
type Change struct {
	Kind ChangeKind

	// Subject is what changed, written the way a reader of the surface
	// names it: a Catalogue key such as "receiver/kafka", or a Schema
	// Registry group or attribute id such as "db.namespace".
	Subject string

	// Detail is the reading of the change: what moved, and to what.
	Detail string

	// Uses are the Blueprints a Catalogue change lands on, in stable order.
	Uses []Use

	// Services are the Services a Schema Registry change lands on, in
	// stable order.
	Services []string
}

// Report is one impact report: what changes when a candidate version becomes
// the active one, computed from the two retained versions before anything is
// activated (ADR-0020 §6).
type Report struct {
	Kind Kind

	// From is the version active when the report was computed, empty when
	// the substrate has never had an active version.
	From string

	// To is the candidate version.
	To string

	// Changes are the entries, worst first in stable order.
	Changes []Change

	// Unknown names what the report could not judge, so a reader takes
	// silence for silence rather than for a clean bill. A report with no
	// estate reading in it says so here rather than reporting nothing
	// changed (ADR-0008's discipline: not knowing is a normal state).
	Unknown []string
}

// Baseline reports whether this is the first activation of a substrate.
// Nothing is judged against a version that was never active, so the first
// report is what the version holds rather than what it changes, and it says
// so rather than reading as a change of nothing.
func (r Report) Baseline() bool { return r.From == "" }

// Empty reports whether the report found nothing at all: no change and
// nothing it could not judge.
func (r Report) Empty() bool { return len(r.Changes) == 0 && len(r.Unknown) == 0 }

// Count returns how many changes of one kind the report holds.
func (r Report) Count(kind ChangeKind) int {
	n := 0
	for _, c := range r.Changes {
		if c.Kind == kind {
			n++
		}
	}
	return n
}

// sortChanges puts the report in its stable order: worst kind first, then by
// subject, so two runs over the same versions read identically.
func (r *Report) sortChanges() {
	sort.SliceStable(r.Changes, func(i, j int) bool {
		a, b := r.Changes[i], r.Changes[j]
		if order[a.Kind] != order[b.Kind] {
			return order[a.Kind] < order[b.Kind]
		}
		if a.Subject != b.Subject {
			return a.Subject < b.Subject
		}
		return a.Detail < b.Detail
	})
}

// Summary is the one-line reading of the report: the counts that decide
// whether an operator reads further.
func (r Report) Summary() string {
	move := fmt.Sprintf("%s %s", r.Kind.Name(), r.To)
	if !r.Baseline() {
		move = fmt.Sprintf("%s %s to %s", r.Kind.Name(), r.From, r.To)
	}

	// A first activation is not a change: nothing was judged against a
	// version that was never active, so the same counts read as what the
	// version holds rather than as what it takes away.
	var parts []string
	add := func(kind ChangeKind, one, many, baselineOne, baselineMany string) {
		n := r.Count(kind)
		if n == 0 {
			return
		}
		if r.Baseline() && baselineOne != "" {
			one, many = baselineOne, baselineMany
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, plural(n, one, many)))
	}
	add(NewlyFailing,
		"Service newly fails a required attribute", "Services newly fail a required attribute",
		"Service fails a required attribute", "Services fail a required attribute")
	add(Removed,
		"component in use is removed", "components in use are removed",
		"component in use is missing", "components in use are missing")
	add(FloorCrossing,
		"stability change crosses a floor", "stability changes cross a floor",
		"component in use is under a floor", "components in use are under a floor")
	add(Deprecated,
		"entry is newly deprecated", "entries are newly deprecated",
		"entry in use is deprecated", "entries in use are deprecated")
	add(LevelTightened,
		"requirement level tightens", "requirement levels tighten",
		"attribute is required", "attributes are required")
	add(AttributeRemoved, "attribute is removed", "attributes are removed", "", "")
	add(AttributeAdded, "attribute is added", "attributes are added", "", "")

	switch {
	case len(parts) == 0 && r.Baseline():
		return fmt.Sprintf("%s: nothing in this estate is affected.", move)
	case len(parts) == 0 && len(r.Unknown) > 0:
		return fmt.Sprintf("%s: nothing the report could read changes.", move)
	case len(parts) == 0:
		return fmt.Sprintf("%s: nothing in this estate changes.", move)
	}
	return fmt.Sprintf("%s: %s.", move, joinList(parts))
}

// Lines are the report beneath its summary: one line per change, then what
// the report could not judge. They are what an operator reads before
// deciding, and what the audit record keeps afterwards.
func (r Report) Lines() []string {
	out := make([]string, 0, len(r.Changes)+len(r.Unknown))
	for _, c := range r.Changes {
		out = append(out, c.Line())
	}
	for _, u := range r.Unknown {
		out = append(out, u)
	}
	return out
}

// Line is one change written for a reader.
func (c Change) Line() string {
	var b strings.Builder
	b.WriteString(c.Subject)
	b.WriteString(" ")
	b.WriteString(c.Detail)
	if len(c.Uses) > 0 {
		names := make([]string, 0, len(c.Uses))
		for _, u := range c.Uses {
			names = append(names, u.String())
		}
		fmt.Fprintf(&b, ". %s %s: %s",
			countWord(len(c.Uses), "Blueprint", "Blueprints"),
			plural(len(c.Uses), "uses it", "use it"),
			strings.Join(names, ", "))
	}
	if len(c.Services) > 0 {
		fmt.Fprintf(&b, ": %s", strings.Join(c.Services, ", "))
	}
	if !strings.HasSuffix(b.String(), ".") {
		b.WriteString(".")
	}
	return b.String()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func countWord(n int, one, many string) string {
	return fmt.Sprintf("%d %s", n, plural(n, one, many))
}

// joinList writes a list the way a sentence does: "a", "a and b", "a, b and c".
func joinList(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}
