// Package schemaregistry builds, serialises and queries the Schema Registry:
// the versioned declaration of what an adopter's telemetry is supposed to
// look like (REQ-022, REQ-023, ADR-0009, ADR-0034 §1).
//
// The Schema Registry is the second Catalogue-pattern substrate (ADR-0034
// §1). The adopter keeps a custom Weaver registry as ordinary git content:
// model files importing the OpenTelemetry semantic conventions, tightening
// requirement levels locally, and adding their own namespaced attributes.
// The platform imports versions of it through the one import pipeline
// (internal/substrate) at a pinned ref, exactly as it imports the Catalogue,
// and installed versions are retained rather than replaced (ADR-0020 §9).
//
// The import reads registry content out of git and nothing else. It runs no
// registry toolchain and ships none: REQ-003 is configurations, never
// binaries, and ADR-0034 §5 has the adopter deploy the upstream tooling.
// That is also why the model here is a record rather than a resolution: a
// reference to an attribute the adopter's own registry does not define lives
// in a dependency registry that is not in this tree, so the import records
// the reference and reports it, and never guesses at what it points to.
//
// No second schema syntax exists (ADR-0034 §1). Nothing in this package
// holds an authored attribute list, and a requirement never inlines one: the
// registry is the single source, and a copy would drift.
//
// The primary keys are the group id and the attribute id, both unique across
// a registry version. A Schema Registry version is atomic against one ref,
// importing the same ref twice yields byte-identical artefacts, and a
// different ref yields a new artefact beside the old one.
package schemaregistry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/substrate"
)

// Source records where one Schema Registry version came from. It is the
// shared pipeline's provenance record, identical for every substrate.
type Source = substrate.Source

// Kind is a group's declared `type`, adopted verbatim from the semantic
// convention model (ADR-0001, ADR-0009). `attribute_group` declares
// attributes; the other six attach them to a signal or to an entity.
type Kind string

const (
	AttributeGroup Kind = "attribute_group"
	Span           Kind = "span"
	Event          Kind = "event"
	Metric         Kind = "metric"
	Resource       Kind = "resource"
	Entity         Kind = "entity"
	Scope          Kind = "scope"
)

// Kinds lists the seven group kinds in stable report order.
var Kinds = []Kind{AttributeGroup, Span, Event, Metric, Resource, Entity, Scope}

// Known reports whether this kind belongs in a Schema Registry. The
// vocabulary has grown before, so an unknown kind is excluded and recorded
// in the import's coverage report rather than failing the import: a new
// upstream group type must surface in the report, not vanish and not stop an
// adopter importing.
func (k Kind) Known() bool {
	for _, known := range Kinds {
		if k == known {
			return true
		}
	}
	return false
}

// Level is an attribute's `requirement_level`, adopted verbatim: the
// four-level scale ADR-0009 chose over a binary required-attributes list.
type Level string

const (
	Required              Level = "required"
	ConditionallyRequired Level = "conditionally_required"
	Recommended           Level = "recommended"
	OptIn                 Level = "opt_in"
)

// Levels lists the four requirement levels from strictest to weakest.
var Levels = []Level{Required, ConditionallyRequired, Recommended, OptIn}

func (l Level) Valid() bool {
	switch l {
	case Required, ConditionallyRequired, Recommended, OptIn:
		return true
	}
	return false
}

// Deprecation is the machine-readable deprecation notice on a group, an
// attribute or an enum member: ready-made remediation text, which is what
// ADR-0034 §7 asks a schema-conformance finding to carry. The older prose
// form of the field lands in Note, so both forms read the same way here.
type Deprecation struct {
	Reason    string `json:"reason,omitempty"`
	RenamedTo string `json:"renamed_to,omitempty"`
	Note      string `json:"note,omitempty"`
}

// Member is one member of an enum-typed attribute. Value is the literal text
// of the declared value, which is the form an observed attribute value is
// compared against (ADR-0034 §4's DistinctValues).
type Member struct {
	ID          string       `json:"id"`
	Value       string       `json:"value"`
	Stability   string       `json:"stability,omitempty"`
	Brief       string       `json:"brief,omitempty"`
	Deprecation *Deprecation `json:"deprecation,omitempty"`
}

// Attribute is one entry in a group's attribute list. It is either a
// definition (ID set) or a reference to an attribute defined elsewhere (Ref
// set), never both: a signal group references the attributes it demands and
// may tighten their requirement level locally, which is the whole mechanism
// ADR-0009 adopted custom registries for.
type Attribute struct {
	ID  string `json:"id,omitempty"`
	Ref string `json:"ref,omitempty"`

	// Type is the declared type, verbatim: `string`, `int`, `double`,
	// `boolean`, one of the array forms, a `template[…]` form, or `enum`
	// when the declaration carries members instead of a type name.
	Type string `json:"type,omitempty"`

	// Members are the declared values of an enum-typed attribute.
	Members []Member `json:"members,omitempty"`

	// Level is the declared requirement level, and is empty when the file
	// declares none. The import records what the registry says and
	// interprets nothing: turning a level into a finding grade is the
	// evaluation-time mapping of ADR-0034 §3, not the import's business.
	Level Level `json:"requirement_level,omitempty"`

	// Condition is the prose a conditionally_required level carries. It is
	// prose by construction, never machine-evaluable (ADR-0034 §3).
	Condition string `json:"condition,omitempty"`

	Stability string `json:"stability,omitempty"`
	Brief     string `json:"brief,omitempty"`
	Note      string `json:"note,omitempty"`

	Deprecation *Deprecation `json:"deprecation,omitempty"`
}

// Key is how one attribute entry is addressed: its id when it defines an
// attribute, its ref when it references one.
func (a Attribute) Key() string {
	if a.ID != "" {
		return a.ID
	}
	return a.Ref
}

// Defines reports whether this entry declares an attribute rather than
// referencing one.
func (a Attribute) Defines() bool { return a.ID != "" }

// Group is one group of the registry: a set of attributes, either declared
// (`attribute_group`) or demanded of a signal or an entity.
type Group struct {
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`

	// File is the registry-relative model file the group was read from:
	// the provenance a remediation message needs to send somebody to the
	// line they have to edit.
	File string `json:"file"`

	Brief     string `json:"brief,omitempty"`
	Note      string `json:"note,omitempty"`
	Stability string `json:"stability,omitempty"`

	// SpanKind, MetricName, Instrument and Unit are the per-kind fields a
	// group carries when its kind uses them.
	SpanKind   string `json:"span_kind,omitempty"`
	MetricName string `json:"metric_name,omitempty"`
	Instrument string `json:"instrument,omitempty"`
	Unit       string `json:"unit,omitempty"`

	Deprecation *Deprecation `json:"deprecation,omitempty"`

	Attributes []Attribute `json:"attributes,omitempty"`
}

// Dependency is one registry this one imports from. The path is where the
// dependency's files are found; the import never follows it, because a
// dependency is not in the adopter's tree and fetching one at import time
// would be the runtime fetch ADR-0019 rules out.
type Dependency struct {
	Name         string `json:"name,omitempty"`
	SchemaURL    string `json:"schema_url,omitempty"`
	RegistryPath string `json:"registry_path,omitempty"`
}

// Manifest is the registry's own identity: what it is called, the schema URL
// that names this version of it, and the registries it imports from. The
// schema URL does not have to be fetchable; it is an identity, not a
// location.
type Manifest struct {
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	SchemaURL    string       `json:"schema_url,omitempty"`
	Dependencies []Dependency `json:"dependencies,omitempty"`
}

// Registry is one atomic Schema Registry version: every group the registry
// declared at one pinned ref.
type Registry struct {
	FormatVersion int      `json:"format_version"`
	Source        Source   `json:"source"`
	Manifest      Manifest `json:"manifest"`
	Groups        []Group  `json:"groups"`

	byGroup map[string]int
	byAttr  map[string]attrAt
}

// attrAt locates one attribute definition: which group declared it, and
// where in that group's list it sits.
type attrAt struct{ group, attr int }

// Version is the ref this Schema Registry version is pinned to.
func (r *Registry) Version() string { return r.Source.Ref }

// Len is the number of groups in this version.
func (r *Registry) Len() int { return len(r.Groups) }

// Summary is the count the import pipeline reports on a write.
func (r *Registry) Summary() string {
	return fmt.Sprintf("%d groups, %d attributes", r.Len(), len(r.byAttr))
}

// Group finds one group by its id.
func (r *Registry) Group(id string) (Group, bool) {
	if i, ok := r.byGroup[id]; ok {
		return r.Groups[i], true
	}
	return Group{}, false
}

// Attribute finds one attribute definition by its id, and the group that
// declares it. A reference is not a definition: an attribute this registry
// only references lives in a dependency registry and is not found here.
func (r *Registry) Attribute(id string) (Attribute, Group, bool) {
	at, ok := r.byAttr[id]
	if !ok {
		return Attribute{}, Group{}, false
	}
	g := r.Groups[at.group]
	return g.Attributes[at.attr], g, true
}

// GroupsOfKind returns every group of one kind, in id order.
func (r *Registry) GroupsOfKind(kind Kind) []Group {
	var out []Group
	for _, g := range r.Groups {
		if g.Kind == kind {
			out = append(out, g)
		}
	}
	return out
}

// index sorts the registry into its canonical order and builds the lookup
// tables. It assumes validate has passed: group and attribute ids are
// unique.
//
// The order is canonical rather than authored: groups by id, attributes
// within a group by key, members within an attribute by id. Two model files
// that differ only in the order they list things are the same registry, and
// making them encode to the same bytes is what keeps a diff between two
// retained versions showing real changes only (ADR-0020 §9).
func (r *Registry) index() {
	sort.Slice(r.Groups, func(i, j int) bool { return r.Groups[i].ID < r.Groups[j].ID })
	r.byGroup = make(map[string]int, len(r.Groups))
	r.byAttr = map[string]attrAt{}
	for i := range r.Groups {
		g := &r.Groups[i]
		sort.Slice(g.Attributes, func(a, b int) bool { return g.Attributes[a].Key() < g.Attributes[b].Key() })
		for j := range g.Attributes {
			a := &g.Attributes[j]
			sort.Slice(a.Members, func(x, y int) bool { return a.Members[x].ID < a.Members[y].ID })
			if a.Defines() {
				r.byAttr[a.ID] = attrAt{group: i, attr: j}
			}
		}
		r.byGroup[g.ID] = i
	}
}

// validate collects everything wrong with a Schema Registry, whether just
// built by the import or loaded from an artefact. Both paths fail closed on
// any problem: a silently wrong registry would corrupt every
// schema-conformance judgement made against it.
func (r *Registry) validate() error {
	var p []string

	if r.FormatVersion != FormatVersion {
		p = append(p, fmt.Sprintf("format_version %d is not the supported version %d", r.FormatVersion, FormatVersion))
	}
	if r.Source.Repository == "" {
		p = append(p, "source.repository is empty. A Schema Registry records the repository it came from")
	}
	if r.Source.Ref == "" {
		p = append(p, "source.ref is empty. A Schema Registry version is pinned to one ref")
	}
	if r.Manifest.Name == "" {
		p = append(p, "manifest.name is empty. A registry is named by its own manifest")
	}
	if len(r.Groups) == 0 {
		p = append(p, "no groups. An empty Schema Registry would make every conformance check a false pass")
	}

	seenGroup := map[string]bool{}
	definedIn := map[string]string{}
	for _, g := range r.Groups {
		p = append(p, g.problems()...)
		if seenGroup[g.ID] {
			p = append(p, fmt.Sprintf("group %q is declared twice. Each group id must be unique across a registry", g.ID))
			continue
		}
		seenGroup[g.ID] = true

		for _, a := range g.Attributes {
			if !a.Defines() {
				continue
			}
			if prev, dup := definedIn[a.ID]; dup {
				p = append(p, fmt.Sprintf("attribute %q is defined in both %s and %s. Each attribute id must be unique across a registry", a.ID, prev, g.ID))
				continue
			}
			definedIn[a.ID] = g.ID
		}
	}

	if len(p) > 0 {
		return fmt.Errorf("invalid schema registry:\n  - %s", strings.Join(p, "\n  - "))
	}
	return nil
}

// problems collects everything wrong with one group.
func (g Group) problems() []string {
	ctx := "group " + g.ID
	var p []string

	if g.ID == "" {
		p = append(p, "a group has no id. The id is the group's primary key")
		ctx = "group in " + g.File
	}
	if !g.Kind.Known() {
		p = append(p, fmt.Sprintf("%s: kind %q is not a known group kind. The known kinds are attribute_group, span, event, metric, resource, entity, and scope", ctx, g.Kind))
	}
	if g.File == "" {
		p = append(p, ctx+": no source file recorded. Every group records the model file it was read from")
	}

	seen := map[string]bool{}
	for _, a := range g.Attributes {
		p = append(p, a.problems(ctx)...)
		key := a.Key()
		if key == "" {
			continue
		}
		if seen[key] {
			p = append(p, fmt.Sprintf("%s: attribute %q is listed twice", ctx, key))
		}
		seen[key] = true
	}
	return p
}

// problems collects everything wrong with one attribute entry.
func (a Attribute) problems(group string) []string {
	ctx := group + ", attribute " + a.Key()
	var p []string

	switch {
	case a.ID == "" && a.Ref == "":
		p = append(p, group+": an attribute has neither an id nor a ref, so nothing names it")
	case a.ID != "" && a.Ref != "":
		p = append(p, fmt.Sprintf("%s: has both an id and a ref. An attribute entry either defines an attribute or references one", ctx))
	}

	if a.Level != "" && !a.Level.Valid() {
		p = append(p, fmt.Sprintf("%s: unknown requirement level %q. The levels are required, conditionally_required, recommended, and opt_in", ctx, a.Level))
	}
	if a.Level == ConditionallyRequired && a.Condition == "" {
		p = append(p, fmt.Sprintf("%s: is conditionally_required but carries no condition, so it cannot say when it applies", ctx))
	}
	if a.Condition != "" && a.Level != ConditionallyRequired && a.Level != Recommended {
		p = append(p, fmt.Sprintf("%s: carries a condition at level %q, which takes none", ctx, a.Level))
	}

	if len(a.Members) > 0 && a.Type != "enum" {
		p = append(p, fmt.Sprintf("%s: declares members at type %q. Members belong to an enum", ctx, a.Type))
	}
	seen := map[string]bool{}
	for _, m := range a.Members {
		if m.ID == "" {
			p = append(p, ctx+": an enum member has no id")
			continue
		}
		if seen[m.ID] {
			p = append(p, fmt.Sprintf("%s: enum member %q is listed twice", ctx, m.ID))
		}
		seen[m.ID] = true
	}
	return p
}
