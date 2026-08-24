package conformance

import (
	"fmt"
	"sort"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
)

// A schema-conformance finding writes its own remediation out of the Schema
// Registry (ADR-0034 §7). Every requirement carries an authored remediation
// line, and for the other requirement kinds that line is the whole answer:
// the requirement asserts one thing, so the author can say what closes it.
// A schema requirement cannot work that way. It is a reference to a scope
// (ADR-0034 §2), the scope resolves to whatever the registry version
// declares, and the gap is whichever of those attributes the reading did
// not find. An authored line would either be generic, which defeats the
// point, or a copy of the registry, which drifts. So the text is generated
// from registry content: the group that demanded the attribute, the
// attribute, the type the registry declares it at, the level, and
// upstream's machine-readable deprecation notice where there is one.

// instrumentationFix is the sentence that keeps the work with the party who
// can do it. ADR-0034's Context records the failure mode directly: a schema
// violation's fix is an instrumentation change, and a reader who takes it to
// whoever edits collector config has been sent to the wrong person.
const instrumentationFix = "Fixing this is an instrumentation change in the service or its SDK configuration, not a change to collector configuration."

// truncatedReading is the fix when the reading itself is the problem: an
// absence read off a truncated sample is not knowledge (ADR-0034 §4), so
// the remediation asks for a better reading rather than for instrumentation
// the service may already have.
const truncatedReading = "The attribute-name reading was truncated, so an attribute it does not name may still be in use. Widen the sample the provider reads, or narrow the window, and judge this again."

// unreadValues is the fix when the value set itself could not be read, or
// could not be read whole. A clipped set has its violations exactly where it
// stopped looking, so the remediation asks for a better reading rather than
// for instrumentation that may already be right.
const unreadValues = "The value-set reading could not prove the enum clean: a reading nobody could take says nothing, and a clipped one cannot prove the values it does name are the only ones. Raise what the provider can read, or narrow the window, and judge this again."

// enumDetail says what one enum reading found, in the reading's own terms:
// the attribute, the values nobody declared, and the set the registry does
// declare, so the finding can be read without opening the registry.
func enumDetail(breached []enumVerdict) []string {
	out := make([]string, 0, len(breached))
	for _, v := range breached {
		out = append(out, fmt.Sprintf("attribute %q carries %s, which %s does not declare; the declared values are %s",
			v.Attribute, quoted(v.Undeclared), declaringGroup(v), quoted(v.Declared)))
	}
	return out
}

// enumRemediation writes the fix for one level's enum breaches. It names both
// ways out, because the registry is the adopter's own: either the value is
// wrong and the instrumentation changes, or the value is right and the
// registry has not caught up with it.
//
// Unlike a miss, this carries the instrumentation sentence at every level
// including opt_in. A miss at opt_in is an offer nobody took up, so there is
// nothing to fix and nobody to send; a value nobody declared is a real
// mismatch between the telemetry and the registry whatever level the
// attribute sits at.
func enumRemediation(level schemaregistry.Level, breached []enumVerdict) string {
	if len(breached) == 0 {
		return ""
	}
	clauses := make([]string, 0, len(breached))
	for _, v := range breached {
		clauses = append(clauses, fmt.Sprintf("%q carries %s where %s declares %s",
			v.Attribute, quoted(v.Undeclared), declaringGroup(v), quoted(v.Declared)))
	}
	lead := fmt.Sprintf("The Schema Registry declares these attributes as enums at %s, and the telemetry carries values it does not declare: %s.", level, strings.Join(clauses, "; "))
	return strings.Join([]string{
		lead,
		"Either stop emitting the undeclared values, or add them as members in the Schema Registry if they are right.",
		instrumentationFix,
	}, " ")
}

// declaringGroup names where the members are declared, which is not always
// the group that demanded the attribute: a signal group references an
// attribute an attribute_group declares, and the values live with the
// declaration.
func declaringGroup(v enumVerdict) string {
	if v.DeclaredIn == "" {
		return "the Schema Registry"
	}
	return "registry group " + v.DeclaredIn
}

// quoted renders a value set, quoted and with the serial comma.
func quoted(values []string) string {
	items := make([]string, 0, len(values))
	for _, v := range values {
		items = append(items, fmt.Sprintf("%q", v))
	}
	return joinAnd(items)
}

// joinFixes joins the fixes one finding's checks produced into one paragraph,
// dropping the empty ones. A finding carries one remediation string, and a
// level that both misses an attribute and carries an undeclared value has two
// things to say about it.
func joinFixes(fixes []string) string {
	var out []string
	for _, f := range fixes {
		if f != "" {
			out = append(out, f)
		}
	}
	return strings.Join(out, " ")
}

// schemaRemediation writes the fix for one level's misses. Nothing in it is
// authored beside the requirement: the attributes, their declared types,
// the groups that demanded them and any deprecation notice are all read out
// of the Schema Registry version the requirement references.
func schemaRemediation(level schemaregistry.Level, missing []demand) string {
	if len(missing) == 0 {
		return ""
	}

	parts := []string{levelLead(level) + " " + demandClauses(level, missing) + "."}
	parts = append(parts, deprecationNotes(missing)...)

	if level == schemaregistry.OptIn {
		// opt_in is an offer nobody took up, so there is nothing to fix,
		// nothing to enrich, and nobody to send. Naming an owner for a
		// non-gap would be the misrouting this text exists to avoid.
		return strings.Join(parts, " ")
	}
	if note := enrichmentNote(missing); note != "" {
		parts = append(parts, note)
	}
	parts = append(parts, instrumentationFix)
	return strings.Join(parts, " ")
}

// levelLead opens the remediation in the level's own terms. The registry
// token is used rather than paraphrased, so a reader can take the word out
// of the finding and search the registry for it.
func levelLead(level schemaregistry.Level) string {
	switch level {
	case schemaregistry.Required:
		return "Emit the attributes the Schema Registry demands at required:"
	case schemaregistry.ConditionallyRequired:
		return "Emit these attributes wherever the registry's condition applies, or tighten the level to required in the Schema Registry if it always applies here:"
	case schemaregistry.Recommended:
		return "Close the coverage gap by emitting the attributes the Schema Registry declares at recommended:"
	default:
		return "Nothing demands these, and the Schema Registry offers them at opt_in for anyone who wants them:"
	}
}

// demandClauses names each group that demanded a missing attribute and what
// it demanded, because the group is where an adopter goes to read the
// condition or change the level.
func demandClauses(level schemaregistry.Level, missing []demand) string {
	byGroup := map[string][]demand{}
	kinds := map[string]schemaregistry.Kind{}
	for _, d := range missing {
		byGroup[d.Group] = append(byGroup[d.Group], d)
		kinds[d.Group] = d.GroupKind
	}

	ids := make([]string, 0, len(byGroup))
	for id := range byGroup {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	clauses := make([]string, 0, len(ids))
	for _, id := range ids {
		clauses = append(clauses, fmt.Sprintf("%s %s %s", groupLabel(id, kinds[id]), demandVerb(level), typedList(byGroup[id])))
	}
	return strings.Join(clauses, "; ")
}

// demandVerb says what the group does with the attribute at this level, so
// the clause does not tell a reader that something is demanded when the
// registry only recommends or offers it.
func demandVerb(level schemaregistry.Level) string {
	switch level {
	case schemaregistry.Recommended:
		return "recommends"
	case schemaregistry.OptIn:
		return "offers"
	default:
		return "demands"
	}
}

// groupLabel names one registry group and, where it has one, the kind of
// telemetry it attaches to: "span group span.db.client" says both which
// group to open and which signal carries the gap.
func groupLabel(id string, kind schemaregistry.Kind) string {
	switch {
	case id == "":
		return "the Schema Registry"
	case kind == "" || kind == schemaregistry.AttributeGroup:
		return "registry group " + id
	default:
		return string(kind) + " group " + id
	}
}

// typedList renders the missing attributes with the type the registry
// declares each at.
func typedList(ds []demand) string {
	items := make([]string, 0, len(ds))
	for _, d := range ds {
		items = append(items, fmt.Sprintf("%q (%s)", d.Attribute, declaredType(d)))
	}
	return joinAnd(items)
}

// declaredType is the type the registry declares, or the honest answer when
// it declares none. An attribute this registry only references is defined in
// a registry it imports from, which is not in the adopter's tree: reporting
// the reference is right, and guessing at a type would not be.
func declaredType(d demand) string {
	if d.Type == "" {
		return "type not declared in this registry version"
	}
	return d.Type
}

// deprecationNotes carries upstream's machine-readable deprecation notice
// through to the reader, one sentence per deprecated attribute. This is the
// migration note ADR-0034 §7 asks a finding to carry: upstream already wrote
// the answer, and paraphrasing it would lose the renamed-to target.
func deprecationNotes(missing []demand) []string {
	var out []string
	for _, d := range missing {
		if d.Deprecation == nil {
			continue
		}
		out = append(out, deprecationNote(d))
	}
	return out
}

func deprecationNote(d demand) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The Schema Registry marks %q deprecated", d.Attribute)
	if reason := strings.TrimSpace(d.Deprecation.Reason); reason != "" {
		fmt.Fprintf(&b, " (%s)", reason)
	}
	if to := strings.TrimSpace(d.Deprecation.RenamedTo); to != "" {
		fmt.Fprintf(&b, ", renamed to %q", to)
	}
	b.WriteString(".")
	if note := strings.TrimSpace(d.Deprecation.Note); note != "" {
		b.WriteString(" " + fullStop(note))
	}
	return b.String()
}

// enrichmentNote suggests collection-time enrichment where the gap sits on a
// resource or entity group. Those groups describe the thing the telemetry
// came from rather than the operation it recorded, which is exactly what a
// collector processor can stamp when the service cannot.
//
// It is a suggestion and nothing more (ADR-0034 §7). The finding does not
// split into a second, collector-owned one, and it does not reroute: one
// finding, one owner, and the owner is the Service's.
func enrichmentNote(missing []demand) string {
	var at []string
	seen := map[string]bool{}
	for _, d := range missing {
		switch d.GroupKind {
		case schemaregistry.Resource, schemaregistry.Entity:
		default:
			continue
		}
		if seen[d.Attribute] {
			continue
		}
		seen[d.Attribute] = true
		at = append(at, fmt.Sprintf("%q", d.Attribute))
	}
	if len(at) == 0 {
		return ""
	}

	verb, object := "is", "it"
	if len(at) > 1 {
		verb, object = "are", "them"
	}
	return fmt.Sprintf("%s %s declared on a resource or entity group, so a processor in the collection path can add %s if the service cannot: k8sattributes for Kubernetes metadata, resourcedetection for host and cloud metadata, or a resource processor for a constant. Enriching at collection does not move this finding, which stays with the Service whose telemetry it is.",
		joinAnd(at), verb, object)
}

// readingRemediation is the fix when nothing could be judged at all. The
// finding is about the reading rather than about the shape of what arrived,
// so the remediation names the reading and never asks for instrumentation
// on the strength of telemetry nobody saw.
func readingRemediation(o Outcome) string {
	if o == NotDelivered {
		return "Nothing arrived for the signals this requirement covers, so there is no shape to judge. Get the service emitting them, check the pipeline that should carry them, and this requirement judges itself on the next evaluation."
	}
	return "No attribute-name reading covers this service, environment, and window, so nothing was judged. Check the telemetry provider can report attribute names for the signals this requirement covers."
}

// joinAnd joins a list with the serial comma the house style asks for.
func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
}

// fullStop ends registry-authored prose with a full stop, because it lands
// in a paragraph beside sentences the platform wrote.
func fullStop(s string) string {
	switch s[len(s)-1] {
	case '.', '!', '?':
		return s
	}
	return s + "."
}
