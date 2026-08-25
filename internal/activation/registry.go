package activation

import (
	"fmt"
	"sort"

	"github.com/telecraft-dev/telecraft/internal/conformance"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
)

// RegistryInputs is everything the Schema Registry impact report reads: the
// two versions, and the estate reading that says what the difference between
// them does to Services.
type RegistryInputs struct {
	// From is the active version, nil when no version has ever been active.
	From *schemaregistry.Registry

	// To is the candidate version.
	To *schemaregistry.Registry

	// Estate is the estate half of the report (ADR-0034's Consequences: "N
	// services newly fail required on db.namespace"). Nil means the report
	// was computed without an estate reading, and the report says so rather
	// than reporting that no Service newly fails, which is a different
	// claim and one nobody checked.
	Estate *EstateReading
}

// EstateReading is the estate judged twice: once against the active version
// and once against the candidate, over the same Observed evidence.
//
// The evidence is the same reading in both passes because it is a reading of
// the estate rather than of a registry: what attributes the telemetry
// carries does not change with the version judging it. What changes is what
// the registry demands of them, which is the whole point of the comparison.
type EstateReading struct {
	Before []conformance.Verdict
	After  []conformance.Verdict
}

// RegistryImpact computes what changes when a candidate Schema Registry
// version becomes the active one: the version diff, and what it does to the
// estate.
//
// The version diff is what an adopter reviews before an estate reading
// exists at all: attributes added and removed, requirement levels tightened,
// groups and attributes newly deprecated. The estate half turns that into
// the reading that decides an activation, which is how many Services stop
// passing.
func RegistryImpact(in RegistryInputs) (Report, error) {
	if in.To == nil {
		return Report{}, fmt.Errorf("no candidate Schema Registry: an impact report is computed from the version being activated")
	}
	rep := Report{Kind: SchemaRegistry, To: in.To.Version()}
	if in.From != nil {
		rep.From = in.From.Version()
	}
	if rep.From == rep.To && rep.From != "" {
		return Report{}, fmt.Errorf("Schema Registry %s is already active, so there is nothing to report", rep.To)
	}

	rep.Changes = append(rep.Changes, deprecatedEntries(in.From, in.To)...)
	if in.From != nil {
		rep.Changes = append(rep.Changes, attributeDiff(in.From, in.To)...)
		rep.Changes = append(rep.Changes, tightenedLevels(in.From, in.To)...)
	}

	if in.Estate == nil {
		rep.Unknown = append(rep.Unknown, "No estate reading was taken, so this report does not say which Services stop passing.")
	} else {
		rep.Changes = append(rep.Changes, newlyFailing(*in.Estate)...)
	}

	rep.sortChanges()
	return rep, nil
}

// definitions indexes every attribute the registry defines, by id, with the
// group that defines it. A reference is not a definition: it demands an
// attribute somebody else declared, which is the level half of the diff, not
// the attribute half.
func definitions(r *schemaregistry.Registry) map[string]schemaregistry.Attribute {
	out := map[string]schemaregistry.Attribute{}
	for _, g := range r.Groups {
		for _, a := range g.Attributes {
			if a.Defines() {
				out[a.ID] = a
			}
		}
	}
	return out
}

// attributeDiff reports the attributes the candidate version adds and
// removes. A removed attribute is the half that matters most: a requirement
// scope that demanded it stops demanding it, so a Service that was failing
// on it silently starts passing.
func attributeDiff(from, to *schemaregistry.Registry) []Change {
	before, after := definitions(from), definitions(to)

	var out []Change
	for _, id := range sortedAttrs(after) {
		if _, existed := before[id]; existed {
			continue
		}
		detail := "is added"
		if lvl := after[id].Level; lvl != "" {
			detail = fmt.Sprintf("is added at %s", lvl)
		}
		out = append(out, Change{Kind: AttributeAdded, Subject: id, Detail: detail})
	}
	for _, id := range sortedAttrs(before) {
		if _, kept := after[id]; kept {
			continue
		}
		out = append(out, Change{Kind: AttributeRemoved, Subject: id, Detail: "is removed, so nothing demands it any more"})
	}
	return out
}

// tightenedLevels reports every requirement level the candidate version
// demands more strictly than the active one, per group: a level is a
// property of the demand a group makes, and the same attribute can be
// recommended in one group and required in another.
func tightenedLevels(from, to *schemaregistry.Registry) []Change {
	rank := map[schemaregistry.Level]int{}
	for i, l := range schemaregistry.Levels {
		rank[l] = i
	}

	before := map[string]schemaregistry.Level{}
	for _, g := range from.Groups {
		for _, a := range g.Attributes {
			before[g.ID+"/"+a.Key()] = a.Level
		}
	}

	var out []Change
	for _, g := range to.Groups {
		for _, a := range g.Attributes {
			was, existed := before[g.ID+"/"+a.Key()]
			if !existed || was == "" || a.Level == "" || a.Level == was {
				continue
			}
			// Strictest first in Levels, so a lower rank is a tighter
			// demand. A level the registry stops declaring is not a
			// tightening and is left to the attribute diff.
			if rank[a.Level] >= rank[was] {
				continue
			}
			out = append(out, Change{
				Kind:    LevelTightened,
				Subject: a.Key(),
				Detail:  fmt.Sprintf("tightens from %s to %s in %s", was, a.Level, g.ID),
			})
		}
	}
	return out
}

// deprecatedEntries reports the groups and attribute definitions the
// candidate version marks deprecated and the active one did not. On a first
// activation every deprecation in the version is new to this estate, because
// no version told it anything before.
func deprecatedEntries(from, to *schemaregistry.Registry) []Change {
	wasDeprecated := func(kind, id string) bool {
		if from == nil {
			return false
		}
		switch kind {
		case "group":
			g, ok := from.Group(id)
			return ok && g.Deprecation != nil
		default:
			a, _, ok := from.Attribute(id)
			return ok && a.Deprecation != nil
		}
	}

	var out []Change
	for _, g := range to.Groups {
		if g.Deprecation != nil && !wasDeprecated("group", g.ID) {
			out = append(out, Change{
				Kind:    Deprecated,
				Subject: g.ID,
				Detail:  "is a deprecated group in this version" + note(g.Deprecation),
			})
		}
		for _, a := range g.Attributes {
			if !a.Defines() || a.Deprecation == nil || wasDeprecated("attribute", a.ID) {
				continue
			}
			out = append(out, Change{
				Kind:    Deprecated,
				Subject: a.ID,
				Detail:  "is a deprecated attribute in this version" + note(a.Deprecation),
			})
		}
	}
	return out
}

// note renders the registry's own deprecation text, which is the
// ready-made remediation a finding carries (ADR-0034 §7) and reads the same
// way here.
func note(d *schemaregistry.Deprecation) string {
	switch {
	case d == nil:
		return ""
	case d.RenamedTo != "":
		return fmt.Sprintf(". It is renamed to %s", d.RenamedTo)
	case d.Reason != "":
		return fmt.Sprintf(". The registry gives the reason: %s", d.Reason)
	case d.Note != "":
		return fmt.Sprintf(". The registry says: %s", d.Note)
	}
	return ""
}

// newlyFailing is the estate half: the Services that pass a schema
// requirement under the active version and fail it under the candidate,
// reported against the attribute they fail on where the finding names one.
//
// A row that newly fails without naming a missing attribute is still
// reported, against the requirement it fails: a value the registry stopped
// declaring fails a Service as surely as an attribute nobody sets, and a
// report that only counted missing attributes would leave it out.
func newlyFailing(est EstateReading) []Change {
	before := failures(est.Before)
	after := failures(est.After)

	// attribute -> services, and requirement -> services, both deduplicated.
	byAttribute := map[string]map[string]bool{}
	byRequirement := map[string]map[string]bool{}

	for key, now := range after {
		was := before[key]
		fresh := subtract(now.attributes, was.attributes)
		if len(fresh) > 0 {
			for _, attr := range fresh {
				add(byAttribute, attr, key.service)
			}
			continue
		}
		if was.failed {
			continue
		}
		add(byRequirement, key.requirement, key.service)
	}

	var out []Change
	for _, attr := range sortedKeysOf(byAttribute) {
		services := namesOf(byAttribute[attr])
		out = append(out, Change{
			Kind:     NewlyFailing,
			Subject:  attr,
			Detail:   fmt.Sprintf("is demanded at %s, and %s newly fail on it", schemaregistry.Required, countWord(len(services), "Service", "Services")),
			Services: services,
		})
	}
	for _, req := range sortedKeysOf(byRequirement) {
		services := namesOf(byRequirement[req])
		out = append(out, Change{
			Kind:     NewlyFailing,
			Subject:  req,
			Detail:   fmt.Sprintf("is newly failed by %s", countWord(len(services), "Service", "Services")),
			Services: services,
		})
	}
	return out
}

// rowRequirement keys one requirement's verdict on one row.
type rowRequirement struct {
	service     string
	environment string
	requirement string
}

// failure is what one requirement did to one row: whether it failed at
// violation grade, and the attributes it failed on.
type failure struct {
	failed     bool
	attributes map[string]bool
}

// failures indexes the violation-grade schema failures in a set of verdicts.
// Only violation-grade findings are read: an improvement that gets worse is
// a real change and belongs in the report, but it is not a Service that
// stops passing, and conflating the two would make an activation look like
// it breaks an estate it only advises.
func failures(verdicts []conformance.Verdict) map[rowRequirement]failure {
	out := map[rowRequirement]failure{}
	for _, v := range verdicts {
		for _, f := range v.Findings {
			if f.Requirement.Kind() != requirements.KindSchemaConformance || !f.Scored() {
				continue
			}
			key := rowRequirement{
				service:     v.Row.Service,
				environment: v.Row.Environment,
				requirement: f.Requirement.ID,
			}
			state := out[key]
			if f.Outcome.Passing() {
				out[key] = state
				continue
			}
			state.failed = true
			if state.attributes == nil {
				state.attributes = map[string]bool{}
			}
			for _, a := range f.Missing {
				state.attributes[a] = true
			}
			out[key] = state
		}
	}
	return out
}

func subtract(now, was map[string]bool) []string {
	var out []string
	for a := range now {
		if !was[a] {
			out = append(out, a)
		}
	}
	sort.Strings(out)
	return out
}

func add(m map[string]map[string]bool, key, value string) {
	if m[key] == nil {
		m[key] = map[string]bool{}
	}
	m[key][value] = true
}

func namesOf(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func sortedKeysOf(m map[string]map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedAttrs(m map[string]schemaregistry.Attribute) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
