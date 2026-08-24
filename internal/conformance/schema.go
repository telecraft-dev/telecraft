package conformance

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/telecraft-dev/telecraft/internal/ownership"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// SchemaReading keys one Observed reading a schema-conformance requirement
// needs: the attribute names in use for one signal over one window. A
// requirement covering two signals needs two, and two requirements covering
// the same signal over the same window share one.
type SchemaReading struct {
	Kind   requirements.SignalKind
	Window time.Duration
}

func (r SchemaReading) String() string {
	return fmt.Sprintf("%s over %s", r.Kind, r.Window)
}

// SchemaEvidence is the evidence a schema-conformance requirement is judged
// against (ADR-0034): the resolved Schema Registry versions the row's
// requirements reference, and the attribute-name readings for the signals
// and windows they cover.
//
// The registry versions travel with the evidence rather than with the
// requirement because a requirement is a reference and never a copy
// (ADR-0034 §2): the requirement names a version, the caller resolves it,
// and nothing in the library holds a second copy of what the registry says.
type SchemaEvidence struct {
	// Versions holds the resolved Schema Registry versions, keyed by the
	// ref a requirement pins. A reference that tracks head reads the
	// version installed under requirements.TrackHead, because which
	// installed version is active is an activation decision rather than a
	// load-time one (ADR-0026 §1).
	Versions map[string]*schemaregistry.Registry

	// Names holds the attribute-name reading per signal and window. A
	// missing reading is unknown, never an empty one: "we did not look" and
	// "nothing is there" are different answers (ADR-0008).
	Names map[SchemaReading]telemetry.AttributeNames
}

// SchemaReadings returns the attribute-name readings the schema-conformance
// requirements applying in one Environment ask for: one per signal and
// window covered, de-duplicated and in stable order.
//
// It is the fetch plan the evidence is gathered against, kept beside the key
// it produces so a caller cannot ask for a reading under one key and file it
// under another. Two requirements covering the same signal over the same
// window share one reading, which is what makes the plan cheaper than a
// reading per requirement.
func SchemaReadings(lib requirements.Library, environment string) []SchemaReading {
	seen := map[SchemaReading]bool{}
	var out []SchemaReading
	for _, req := range lib.Sorted() {
		if req.Schema == nil || !req.AppliesTo(environment) {
			continue
		}
		window := req.Schema.Window.Std()
		for _, kind := range telemetry.Signals() {
			if !req.Schema.Covers(kind) {
				continue
			}
			key := SchemaReading{Kind: kind, Window: window}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, key)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Window != out[j].Window {
			return out[i].Window < out[j].Window
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// GatherSchema builds the evidence one row's schema-conformance
// requirements are judged against: the Schema Registry versions the load
// resolved, and one attribute-name reading per signal and window the
// applying requirements cover, taken by read.
//
// read takes one planned reading rather than this package taking a
// TelemetryProvider, so the plan and the seam stay apart: a caller reading
// from a live backend and one replaying a declared reading gather the same
// evidence through the same plan, and neither can quietly answer a key it
// was not asked for.
//
// A row with no schema requirement gathers nothing. Its evidence is zero
// valued, which is what a requirement of another kind is judged against
// anyway, and reading attribute names for a library that asks about none
// would be a round trip nobody wanted.
func GatherSchema(lib requirements.Library, environment string, read func(SchemaReading) telemetry.AttributeNames) SchemaEvidence {
	plan := SchemaReadings(lib, environment)
	if len(plan) == 0 {
		return SchemaEvidence{}
	}
	ev := SchemaEvidence{
		Versions: lib.SchemaRegistries,
		Names:    make(map[SchemaReading]telemetry.AttributeNames, len(plan)),
	}
	for _, key := range plan {
		ev.Names[key] = read(key)
	}
	return ev
}

// RegistryFor returns the Schema Registry version this assertion is judged
// against, and whether the evidence carries it.
func (e SchemaEvidence) RegistryFor(a requirements.SchemaAssertion) (*schemaregistry.Registry, bool) {
	reg, ok := e.Versions[registryKey(a)]
	return reg, ok && reg != nil
}

// registryKey is how a reference addresses its version in the evidence: the
// pinned ref, or requirements.TrackHead for a tracking reference.
func registryKey(a requirements.SchemaAssertion) string {
	if a.Tracking() {
		return requirements.TrackHead
	}
	return a.RegistryVersion
}

// demand is one attribute the scope demands, at the level the Schema
// Registry declares it at and named by the group that demanded it. It is
// read out of the registry at evaluation and never authored: a requirement
// file that held one would be the copy ADR-0034 §2 refuses.
//
// It carries what the finding's remediation has to say (ADR-0034 §7): the
// group and its kind, the attribute, the type the registry declares it at,
// and upstream's machine-readable deprecation notice. Those travel here
// rather than being looked up again when the text is written, because the
// lookup is not free and the answer cannot change between the two moments.
type demand struct {
	Attribute string
	Level     schemaregistry.Level
	Group     string

	// GroupKind is the demanding group's kind, which says what the
	// attribute is demanded of: a span, a metric, a resource. It also
	// decides whether collection-time enrichment is worth suggesting.
	GroupKind schemaregistry.Kind

	// Type is the type the registry declares the attribute at, read from
	// the definition rather than from a reference to it. Empty when the
	// definition lives in a registry this one imports from, which is not
	// in the adopter's tree: the remediation says so rather than guessing.
	Type string

	// Deprecation is upstream's notice, read from the definition. A
	// deprecated attribute already carries its own migration instruction,
	// and the finding hands it over rather than paraphrasing it.
	Deprecation *schemaregistry.Deprecation
}

// defaultLevel is the level an attribute is demanded at when the registry
// declares none, matching the upstream semantic-conventions default that
// requirements.Level adopts. Treating an undeclared level as required would
// invent a floor the registry never stated.
const defaultLevel = schemaregistry.Recommended

// judgeSchema evaluates one schema-conformance requirement, mapping the
// Schema Registry's four requirement levels onto the existing outcomes and
// adding none (ADR-0034 §3, ADR-0030's discipline).
//
// The requirement yields one finding per level present in its scope, in
// strictest-first order: required is violation-grade and decides the row's
// binary, conditionally_required and recommended are improvement-grade, and
// opt_in is information-grade. The violation-grade finding is always
// produced, because it is the requirement's verdict: a scope that demands
// nothing at the required level still has to say whether the signal arrived.
func judgeSchema(req requirements.Requirement, ev Evidence) []Finding {
	a := *req.Schema

	// The live tap is not built (ADR-0034 §6, issue #159). The loader
	// refuses `placement: live` today, so this is the belt to that
	// braces: a live requirement that reached the evaluator must read
	// unknown, because a dead tap must not read as clean.
	if req.Placement == requirements.Live {
		return []Finding{schemaUnknown(req,
			"this requirement is judged at placement live, and the collection-time tap that would emit its findings is not built yet",
			"Judge this requirement against landed telemetry by setting its placement to landed, or take it out of the library until the collection-time tap ships.")}
	}

	reg, ok := ev.Schema.RegistryFor(a)
	if !ok {
		return []Finding{schemaUnknown(req,
			fmt.Sprintf("no Schema Registry version %q is available to this evaluation, so what the scope demands is not known", registryKey(a)),
			fmt.Sprintf("Import Schema Registry version %q and make it available to the evaluation, or pin this requirement to a version that is installed.", registryKey(a)))}
	}

	demands := demandsOf(reg, a.Scope)
	byLevel := map[schemaregistry.Level][]demand{}
	for _, d := range demands {
		byLevel[d.Level] = append(byLevel[d.Level], d)
	}

	reading := readScope(a, ev)

	var out []Finding
	for _, level := range schemaregistry.Levels {
		at := byLevel[level]
		if len(at) == 0 && level != schemaregistry.Required {
			// A level the scope demands nothing at produces no finding:
			// there is nothing to report and nothing to count. Required is
			// the exception, because its finding is the requirement's own
			// verdict.
			continue
		}
		out = append(out, schemaFinding(req, level, at, reading))
	}
	return out
}

// scopeReading is what the readings say about one requirement's scope,
// gathered once and shared by every level's finding: the same reading
// answers all four.
type scopeReading struct {
	// present is the set of attribute names in use, unioned across the
	// covered signals: an attribute demanded of traces and logs is carried
	// if either carries it, which is the reading the scope asked for.
	present map[string]bool

	// outcome is the worst per-signal verdict the reading supports before
	// any demand is checked: unknown when a reading is missing or the
	// provider could not see, not_delivered when a covered signal never
	// arrived, and compliant when every covered signal arrived and can be
	// judged.
	outcome Outcome

	// judgeable reports whether any covered signal arrived and was read, so
	// a missing attribute means something.
	judgeable bool

	// truncated reports that at least one reading was derived from fewer
	// records than the window holds, so an absent name may be absent from
	// the reading rather than from the telemetry (ADR-0034 §4).
	truncated bool

	detail []string
}

// readScope reads every signal the assertion covers, in the stable signal
// order, and reduces them to one verdict about the reading itself. Signals
// are reduced worst-first on the existing severity ordering, so a covered
// signal that never arrived outranks one that arrived in the wrong shape,
// and a signal nobody could read outranks a clean one: being blind on one
// leg of a scope is not a pass on that scope (ADR-0008).
func readScope(a requirements.SchemaAssertion, ev Evidence) scopeReading {
	r := scopeReading{present: map[string]bool{}, outcome: Compliant}
	window := a.Window.Std()

	for _, kind := range telemetry.Signals() {
		if !a.Covers(kind) {
			continue
		}
		key := SchemaReading{Kind: kind, Window: window}
		names, have := ev.Schema.Names[key]
		switch {
		case !have:
			r.worsen(Unknown, fmt.Sprintf("no attribute-name reading covers %s", key))
			continue
		case !names.Known:
			cause := names.Cause
			if cause == "" {
				cause = "reading absent"
			}
			r.worsen(Unknown, fmt.Sprintf("%s attribute-name reading unavailable: %s", kind, cause))
			continue
		}

		if !arrived(kind, window, names, ev) {
			r.worsen(NotDelivered, fmt.Sprintf("no %s arrived in the last %s, so the shape of what arrived cannot be judged", kind, window))
			continue
		}

		r.judgeable = true
		if names.Truncated {
			r.truncated = true
			r.detail = append(r.detail, fmt.Sprintf("the %s attribute-name reading is truncated (%d of %d records), so an attribute it does not name may still be in use",
				kind, names.SampledRecords, names.TotalRecords))
		}
		for _, n := range names.Names {
			r.present[n] = true
		}
	}
	return r
}

// worsen records a per-signal verdict, keeping the worst on the existing
// severity ordering.
func (r *scopeReading) worsen(o Outcome, detail string) {
	if o.Severity() > r.outcome.Severity() {
		r.outcome = o
	}
	r.detail = append(r.detail, detail)
}

// arrived reports whether the covered signal arrived in the window at all.
// The presence reading answers it when the row carries one, because that is
// what it is for; otherwise an attribute-name reading that is known and
// names nothing is the answer, since a signal that arrived carries attribute
// names and one that did not carries none.
func arrived(kind requirements.SignalKind, window time.Duration, names telemetry.AttributeNames, ev Evidence) bool {
	if obs, have := ev.ObservedIn(window); have {
		if sig, seen := obs.Signals[kind]; seen && sig.Known {
			return sig.Present
		}
	}
	return len(names.Names) > 0
}

// schemaFinding builds one level's finding: the grade the level maps to, the
// outcome that grade's checks produce, and the detail naming every attribute
// the reading did not find.
func schemaFinding(req requirements.Requirement, level schemaregistry.Level, at []demand, r scopeReading) Finding {
	f := Finding{Requirement: req, Grade: gradeOf(level), Outcome: r.outcome}
	f.Detail = append(f.Detail, r.detail...)

	if !r.judgeable {
		// Nothing arrived, or nothing could be read. The reading's own
		// verdict stands and no attribute is judged against it: reporting
		// an attribute missing from telemetry that never arrived would
		// name the wrong fix.
		if len(f.Detail) == 0 {
			f.Detail = []string{"no reading covers this requirement's signals"}
		}
		f.Remediation = readingRemediation(f.Outcome)
		return f
	}

	var missing []demand
	for _, d := range at {
		if !r.present[d.Attribute] {
			missing = append(missing, d)
		}
	}

	if level == schemaregistry.Recommended {
		f.Coverage = ratio(len(at)-len(missing), len(at))
	}

	if len(missing) == 0 {
		if len(at) > 0 {
			f.Detail = append(f.Detail, fmt.Sprintf("every %s attribute the scope demands is in use (%d of %d)", level, len(at), len(at)))
		}
		return f
	}

	if r.truncated && level == schemaregistry.Required {
		// A truncated reading can miss a name that is in use, so an absence
		// read off one is not knowledge (ADR-0034 §4: truncation is always
		// reported, and this is what reporting it is for). Presence is
		// still proof, which is why a truncated reading with nothing
		// missing stays compliant: extra records can only add names.
		f.Outcome = Unknown
		f.Detail = append(f.Detail, fmt.Sprintf("%s not named by a truncated reading, which cannot tell an attribute that is absent from one it did not sample", attrList(missing)))
		f.Remediation = truncatedReading
		return f
	}

	if level == schemaregistry.Required {
		// The only level that flips the outcome: the telemetry arrived and
		// is the wrong shape, which is misconfigured (ADR-0034 §3).
		f.Outcome = Misconfigured
	}
	f.Detail = append(f.Detail, missDetail(level, missing, len(at)))
	f.Remediation = schemaRemediation(level, missing)
	return f
}

// missDetail renders one level's misses, in that level's own terms: a
// required miss is a breach, a conditionally_required one is a condition
// nobody can evaluate, a recommended one is coverage, and an opt_in one is
// an offer nobody took up.
func missDetail(level schemaregistry.Level, missing []demand, demanded int) string {
	// The level is named by its registry token rather than paraphrased, so
	// a reader can take the word out of the finding and search the registry
	// for it.
	found := demanded - len(missing)
	gap := fmt.Sprintf("%s demanded at %s by %s, and not in use", attrList(missing), level, groups(missing))
	switch level {
	case schemaregistry.Required:
		return gap
	case schemaregistry.ConditionallyRequired:
		return gap + ". The condition is prose rather than anything the platform can evaluate, so this is an improvement rather than a breach: tighten the level to required in the Schema Registry if it always applies here"
	case schemaregistry.Recommended:
		return fmt.Sprintf("%d of %d attributes at recommended in use; %s", found, demanded, gap)
	default:
		return gap + ". Nothing demands it: opt_in is what the registry makes available, not a gap"
	}
}

// gradeOf maps one Schema Registry requirement level onto the weight its
// findings carry (ADR-0034 §3). No new outcome and no new grade: the four
// levels land on the three weights the platform already has.
//
// conditionally_required demotes deliberately. Its condition is prose by
// construction, so the platform cannot tell whether it applies to this
// Service, and hard-failing on an unevaluable condition manufactures false
// reds. An adopter who means "always" tightens the level to required in
// their own registry, which is what a custom registry is for.
func gradeOf(level schemaregistry.Level) ownership.Grade {
	switch level {
	case schemaregistry.Required:
		return ownership.Violation
	case schemaregistry.ConditionallyRequired, schemaregistry.Recommended:
		return ownership.Advisory
	default:
		return ownership.Neutral
	}
}

// schemaUnknown is the finding a schema requirement produces when the
// evaluation cannot be attempted at all. It is violation-grade and unknown:
// a requirement nobody could judge must not read as a pass, and must not
// quietly leave the denominator either.
//
// It still routes to the Service and still carries a fix (ADR-0034 §7). The
// fix is about the evaluation rather than about instrumentation, because
// nothing here says the telemetry is the wrong shape.
func schemaUnknown(req requirements.Requirement, cause, fix string) Finding {
	return Finding{
		Requirement: req,
		Grade:       ownership.Violation,
		Outcome:     Unknown,
		Detail:      []string{cause},
		Remediation: fix,
	}
}

// demandsOf resolves a scope into the attributes it demands and the level
// the registry declares each at. Groups are demanded by id; a namespace
// demands every attribute the registry carries under it, whether the
// registry defines that attribute or references one a dependency defines.
//
// An attribute demanded twice at two levels is kept at the stricter one.
// Tightening a level locally is the whole mechanism a custom registry exists
// for (ADR-0009), so the tightening has to win: taking the looser reading
// would let a group that mentions an attribute in passing undo a group that
// demands it.
func demandsOf(reg *schemaregistry.Registry, scope requirements.Scope) []demand {
	strict := map[string]demand{}
	keep := func(d demand) {
		prev, seen := strict[d.Attribute]
		if !seen || rank(d.Level) < rank(prev.Level) {
			strict[d.Attribute] = d
		}
	}

	for _, id := range scope.Groups {
		g, ok := reg.Group(id)
		if !ok {
			// The loader resolves the scope against the pinned version and
			// refuses a group the version does not declare, so this is
			// reachable only for a tracking reference whose active version
			// dropped the group. Demanding nothing of it is right: the
			// registry no longer says it demands anything.
			continue
		}
		for _, a := range g.Attributes {
			keep(demandOf(reg, g, a))
		}
	}

	for _, ns := range scope.Namespaces {
		prefix := ns + "."
		for _, g := range reg.Groups {
			for _, a := range g.Attributes {
				if key := a.Key(); key == ns || strings.HasPrefix(key, prefix) {
					keep(demandOf(reg, g, a))
				}
			}
		}
	}

	out := make([]demand, 0, len(strict))
	for _, d := range strict {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Attribute < out[j].Attribute })
	return out
}

// demandOf reads one attribute entry into the demand the finding is written
// from. The level comes from the entry, because tightening a level on a
// reference is the whole mechanism a custom registry exists for (ADR-0009);
// the type and the deprecation notice come from the definition, because a
// reference restates neither.
func demandOf(reg *schemaregistry.Registry, g schemaregistry.Group, a schemaregistry.Attribute) demand {
	def := definitionOf(reg, a)
	return demand{
		Attribute:   a.Key(),
		Level:       levelOf(a),
		Group:       g.ID,
		GroupKind:   g.Kind,
		Type:        def.Type,
		Deprecation: def.Deprecation,
	}
}

// definitionOf resolves an entry to the declaration that defines it. An
// entry that defines an attribute is its own definition; a reference is
// resolved against this registry version, and one that resolves nowhere is
// defined in a dependency registry that is not in this tree, so the entry
// stands for itself and the fields it does not carry stay empty.
func definitionOf(reg *schemaregistry.Registry, a schemaregistry.Attribute) schemaregistry.Attribute {
	if a.Defines() {
		return a
	}
	if def, _, ok := reg.Attribute(a.Ref); ok {
		return def
	}
	return a
}

// levelOf reads the level the registry declares an attribute at, defaulting
// to recommended where it declares none.
func levelOf(a schemaregistry.Attribute) schemaregistry.Level {
	if a.Level == "" || !a.Level.Valid() {
		return defaultLevel
	}
	return a.Level
}

// rank orders levels strictest first, matching schemaregistry.Levels.
func rank(l schemaregistry.Level) int {
	for i, known := range schemaregistry.Levels {
		if known == l {
			return i
		}
	}
	return len(schemaregistry.Levels)
}

// ratio returns found over demanded, or nil when nothing was demanded: a
// coverage ratio over an empty set is not 1.0, it is nothing to report.
func ratio(found, demanded int) *float64 {
	if demanded <= 0 {
		return nil
	}
	r := float64(found) / float64(demanded)
	return &r
}

// attrList renders the missing attributes, quoted and in order, with the verb
// the caller's sentence needs.
func attrList(missing []demand) string {
	q := make([]string, 0, len(missing))
	for _, d := range missing {
		q = append(q, fmt.Sprintf("%q", d.Attribute))
	}
	if len(q) == 1 {
		return "attribute " + q[0] + " is"
	}
	return "attributes " + strings.Join(q, ", ") + " are"
}

// groups renders the distinct registry groups that demanded the missing
// attributes, which is where an adopter goes to change a level.
func groups(missing []demand) string {
	seen := map[string]bool{}
	var out []string
	for _, d := range missing {
		if d.Group == "" || seen[d.Group] {
			continue
		}
		seen[d.Group] = true
		out = append(out, d.Group)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return "the Schema Registry"
	}
	return "registry group " + strings.Join(out, ", ")
}
