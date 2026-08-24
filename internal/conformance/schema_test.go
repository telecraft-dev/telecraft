package conformance

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/ownership"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// snapshotRef is the ref the fixture Schema Registry version is imported at.
const snapshotRef = "v1.4.0"

// schemaWindow is the window every fixture requirement here asks for.
const schemaWindow = 24 * time.Hour

// registry imports the Schema Registry fixture that lives beside the
// schemaregistry package. It is reused rather than copied for the reason a
// requirement references a registry rather than copying one: a second
// registry in this package's testdata would drift from the first.
//
// What the fixture declares, and what these tests turn on: span.db.client
// demands db.namespace, db.system.name, enterprise.criticality_tier and
// server.address at required, db.operation.name at conditionally_required
// and server.port at recommended; entity.service offers
// enterprise.cost_centre at opt_in.
func registry(t *testing.T) *schemaregistry.Registry {
	t.Helper()
	reg, _, err := schemaregistry.Import(
		filepath.Join("..", "schemaregistry", "testdata", "registry-"+snapshotRef),
		schemaregistry.Source{
			Repository: "git.example.test/estate/registry",
			Ref:        snapshotRef,
			Commit:     "3f2a1c8d5b7e9046a1c2d3e4f5061728394a5b6c",
		})
	if err != nil {
		t.Fatalf("importing the fixture Schema Registry: %v", err)
	}
	return reg
}

// schemaRequirement builds one loaded-shaped schema-conformance requirement
// over the named scope.
func schemaRequirement(scope requirements.Scope, signals ...requirements.SignalKind) requirements.Requirement {
	if len(signals) == 0 {
		signals = []requirements.SignalKind{requirements.Traces}
	}
	return requirements.Requirement{
		ID:      "db-spans-conform",
		Title:   "Database spans carry what the registry demands",
		Version: 1,
		Level:   requirements.Required,
		Owner:   "platform-observability",
		Schema: &requirements.SchemaAssertion{
			RegistryVersion: snapshotRef,
			Scope:           scope,
			Signals:         signals,
			Window:          requirements.Duration(schemaWindow),
		},
		Placement:   requirements.Landed,
		Remediation: "instrument the missing attributes",
	}
}

// schemaEvidence builds evidence carrying the fixture registry, one
// attribute-name reading naming exactly the attributes in use, and, for every
// enum-declared attribute among them, a value-set reading carrying exactly
// what the registry declares.
//
// The conforming value sets are part of the fixture rather than an
// afterthought. An enum attribute in use whose values nobody read is unknown
// rather than clean (ADR-0034 §4), so evidence that named the attribute and
// said nothing about its values would make every one of these tests unknown
// and prove nothing about the level mapping they are here for.
func schemaEvidence(t *testing.T, kind requirements.SignalKind, inUse ...string) Evidence {
	t.Helper()
	reg := registry(t)
	ev := Evidence{
		Schema: SchemaEvidence{
			Versions: map[string]*schemaregistry.Registry{snapshotRef: reg},
			Names: map[SchemaReading]telemetry.AttributeNames{
				{Kind: kind, Window: schemaWindow}: {
					Known:  true,
					AsOf:   time.Now(),
					Window: schemaWindow,
					Names:  inUse,
				},
			},
			Values: map[SchemaValueReading]telemetry.DistinctValues{},
		},
	}
	for _, name := range inUse {
		values := declaredIn(reg, name)
		if len(values) == 0 {
			continue
		}
		ev.Schema.Values[SchemaValueReading{Kind: kind, Window: schemaWindow, Attribute: name}] = conformingValues(name, values)
	}
	return ev
}

// declaredIn is the value set the fixture registry declares for one
// attribute, or nothing when the attribute is not an enum.
func declaredIn(reg *schemaregistry.Registry, attribute string) []string {
	def, _, ok := reg.Attribute(attribute)
	if !ok {
		return nil
	}
	return sortedSet(declaredValues(def.Members))
}

// conformingValues is a whole, untruncated reading carrying exactly the
// declared set: an enum nobody violated.
func conformingValues(attribute string, values []string) telemetry.DistinctValues {
	return telemetry.DistinctValues{
		Known:     true,
		AsOf:      time.Now(),
		Window:    schemaWindow,
		Attribute: attribute,
		Values:    values,
		Cap:       telemetry.MaxDistinctValues,
	}
}

// everything the fixture's span.db.client group demands, at every level.
func conformingSpan() []string {
	return []string{
		"db.namespace",
		"db.operation.name",
		"db.system.name",
		"enterprise.criticality_tier",
		"server.address",
		"server.port",
	}
}

func evaluateSchema(t *testing.T, req requirements.Requirement, ev Evidence) Verdict {
	t.Helper()
	lib := requirements.Library{Requirements: map[string]requirements.Requirement{req.ID: req}}
	return Evaluate(Row{Service: "checkout", Environment: "production"}, lib, ev, time.Now())
}

// findingAt returns the finding a level's grade produced, identified by the
// grade the level maps to and the level named in its detail.
func findingAt(t *testing.T, v Verdict, level schemaregistry.Level) Finding {
	t.Helper()
	for _, f := range v.Findings {
		if f.Weight() != gradeOf(level) {
			continue
		}
		if level == schemaregistry.Required || detailNames(f, string(level)) {
			return f
		}
	}
	t.Fatalf("no %s finding in %v", level, details(v))
	return Finding{}
}

func detailNames(f Finding, want string) bool {
	for _, d := range f.Detail {
		if strings.Contains(d, want) {
			return true
		}
	}
	return false
}

func details(v Verdict) []string {
	var out []string
	for _, f := range v.Findings {
		out = append(out, string(f.Weight())+": "+strings.Join(f.Detail, "; "))
	}
	return out
}

// The count of outcomes is the invariant ADR-0034 §3 promises to leave
// alone: it maps the registry's levels onto what exists and adds nothing.
func TestSchemaConformanceAddsNoOutcome(t *testing.T) {
	if len(Outcomes()) != 8 {
		t.Fatalf("Outcomes() has %d entries, want the eight ADR-0004's seven plus library_drift", len(Outcomes()))
	}

	known := map[Outcome]bool{}
	for _, o := range Outcomes() {
		known[o] = true
	}

	// Every reading shape judgeSchema can meet, so every outcome it can
	// produce is exercised here rather than asserted from the source.
	for _, ev := range []Evidence{
		schemaEvidence(t, requirements.Traces, conformingSpan()...),
		schemaEvidence(t, requirements.Traces, "db.namespace"),
		schemaEvidence(t, requirements.Traces),
		{},
	} {
		v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}), ev)
		for _, f := range v.Findings {
			if !f.Outcome.Valid() || !known[f.Outcome] {
				t.Errorf("schema conformance produced outcome %q, which is not one of the eight", f.Outcome)
			}
		}
	}
}

// required is the one level that flips the outcome, and telemetry that
// arrived in the wrong shape is misconfigured rather than not_delivered.
func TestSchemaRequiredMissFlipsToMisconfigured(t *testing.T) {
	ev := schemaEvidence(t, requirements.Traces,
		"db.namespace", "db.operation.name", "db.system.name", "server.address", "server.port")
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}), ev)

	f := findingAt(t, v, schemaregistry.Required)
	if f.Outcome != Misconfigured {
		t.Errorf("required miss outcome = %q, want %q", f.Outcome, Misconfigured)
	}
	if f.Weight() != ownership.Violation {
		t.Errorf("required grade = %q, want %q", f.Weight(), ownership.Violation)
	}
	if !f.Failing() {
		t.Error("a required miss does not fail the row, so nothing gates on it")
	}
	if !detailNames(f, "enterprise.criticality_tier") {
		t.Errorf("the detail does not name the missing attribute: %v", f.Detail)
	}
	if !detailNames(f, "span.db.client") {
		t.Errorf("the detail does not name the group that demanded it: %v", f.Detail)
	}
	if v.Worst() != Misconfigured {
		t.Errorf("Worst() = %q, want %q", v.Worst(), Misconfigured)
	}
}

// Every required attribute in use is compliant, whatever the weaker levels
// say.
func TestSchemaRequiredSatisfiedIsCompliant(t *testing.T) {
	ev := schemaEvidence(t, requirements.Traces, conformingSpan()...)
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}), ev)

	f := findingAt(t, v, schemaregistry.Required)
	if f.Outcome != Compliant {
		t.Errorf("outcome = %q, want %q (detail: %v)", f.Outcome, Compliant, f.Detail)
	}
	if s := v.Score(); s.Failing != 0 || s.Passing != 1 || s.Total != 1 {
		t.Errorf("score = %+v, want one passing violation-grade finding", s)
	}
}

// conditionally_required is demoted at evaluation: the condition is prose,
// and hard-failing on a condition nobody can evaluate manufactures false
// reds (ADR-0034 §3).
func TestSchemaConditionallyRequiredDoesNotFlipTheOutcome(t *testing.T) {
	inUse := []string{"db.namespace", "db.system.name", "enterprise.criticality_tier", "server.address", "server.port"}
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}), schemaEvidence(t, requirements.Traces, inUse...))

	if got := findingAt(t, v, schemaregistry.Required).Outcome; got != Compliant {
		t.Errorf("the violation-grade outcome = %q, want %q: a conditionally_required miss must not flip it", got, Compliant)
	}
	if v.Worst() != Compliant {
		t.Errorf("Worst() = %q, want %q", v.Worst(), Compliant)
	}
	if s := v.Score(); s.Failing != 0 || s.Ratio() != 1 {
		t.Errorf("score = %+v, want a clean binary", s)
	}

	f := findingAt(t, v, schemaregistry.ConditionallyRequired)
	if f.Weight() != ownership.Advisory {
		t.Errorf("conditionally_required grade = %q, want %q", f.Weight(), ownership.Advisory)
	}
	if f.Failing() {
		t.Error("an improvement-grade finding fails the row, so the binary is not violations alone")
	}
	if !detailNames(f, "db.operation.name") {
		t.Errorf("the detail does not name the attribute: %v", f.Detail)
	}
}

// recommended is an improvement finding carrying a coverage ratio: partial
// adoption is the normal state of a recommended attribute, and "1 of 2" is
// the actionable form of it.
func TestSchemaRecommendedCarriesACoverageRatio(t *testing.T) {
	// The db namespace demands db.namespace, db.operation.name,
	// db.system.name and db.connection_string, the last two of which the
	// span group does not reference: db.connection_string is declared with
	// no level and so is demanded at the default, recommended.
	scope := requirements.Scope{Namespaces: []string{"db"}}
	v := evaluateSchema(t, schemaRequirement(scope), schemaEvidence(t, requirements.Traces, "db.namespace", "db.system.name", "db.operation.name"))

	f := findingAt(t, v, schemaregistry.Recommended)
	got, ok := f.CoverageRatio()
	if !ok {
		t.Fatalf("the recommended finding carries no coverage ratio: %v", f.Detail)
	}
	if got != 0 {
		t.Errorf("coverage = %v, want 0: db.connection_string is the only recommended attribute in scope and it is not in use", got)
	}
	if f.Weight() != ownership.Advisory {
		t.Errorf("recommended grade = %q, want %q", f.Weight(), ownership.Advisory)
	}
	if v.Worst() != Compliant {
		t.Errorf("Worst() = %q, want %q: a recommended miss never darkens the badge", v.Worst(), Compliant)
	}
}

// A fully covered recommended set still carries its ratio, so the finding
// reads the same way whether it is 2 of 2 or 1 of 2.
func TestSchemaRecommendedFullCoverage(t *testing.T) {
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}), schemaEvidence(t, requirements.Traces, conformingSpan()...))

	f := findingAt(t, v, schemaregistry.Recommended)
	got, ok := f.CoverageRatio()
	if !ok || got != 1 {
		t.Errorf("coverage = (%v, %v), want (1, true)", got, ok)
	}
}

// opt_in is information: never a gap, never a pass, and out of every
// denominator.
func TestSchemaOptInIsInformation(t *testing.T) {
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"entity.service"}}), schemaEvidence(t, requirements.Traces, "service.name"))

	f := findingAt(t, v, schemaregistry.OptIn)
	if f.Weight() != ownership.Neutral {
		t.Errorf("opt_in grade = %q, want %q", f.Weight(), ownership.Neutral)
	}
	if f.Scored() || f.Failing() {
		t.Error("an information finding feeds the binary")
	}
	if !detailNames(f, "enterprise.cost_centre") {
		t.Errorf("the detail does not name the opt-in attribute: %v", f.Detail)
	}
	s := v.Score()
	if s.Neutral != 1 {
		t.Errorf("Score.Neutral = %d, want 1: an information finding is counted", s.Neutral)
	}
	if s.Total != 1 || s.Passing != 1 {
		t.Errorf("score = %+v, want the violation-grade finding alone in the denominator", s)
	}
}

// Improvement and information findings are visible and counted, and the
// binary is decided by violations alone (ADR-0034 §3).
func TestSchemaImprovementsRideAlongsideTheBinary(t *testing.T) {
	// Nothing but the required attributes is in use, so the
	// conditionally_required, recommended and opt_in levels all miss.
	inUse := []string{"db.namespace", "db.system.name", "enterprise.criticality_tier", "server.address", "service.name"}
	scope := requirements.Scope{Groups: []string{"span.db.client", "entity.service"}}
	v := evaluateSchema(t, schemaRequirement(scope), schemaEvidence(t, requirements.Traces, inUse...))

	if len(v.Findings) != 4 {
		t.Fatalf("got %d findings, want one per level in scope: %v", len(v.Findings), details(v))
	}
	s := v.Score()
	if s.Total != 1 || s.Passing != 1 || s.Failing != 0 {
		t.Errorf("score = %+v, want one passing violation-grade finding", s)
	}
	if s.Advisory != 2 || s.Neutral != 1 {
		t.Errorf("score = %+v, want two advisory and one neutral counted alongside", s)
	}
	if s.Ratio() != 1 {
		t.Errorf("Ratio() = %v, want 1: improvements never move the ratio", s.Ratio())
	}
	if v.Worst() != Compliant {
		t.Errorf("Worst() = %q, want %q", v.Worst(), Compliant)
	}
}

// A signal that never arrived is not_delivered: the shape of telemetry that
// does not exist cannot be judged, and calling it misconfigured would name
// the wrong fix.
func TestSchemaNeverArrivedIsNotDelivered(t *testing.T) {
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}), schemaEvidence(t, requirements.Traces))

	f := findingAt(t, v, schemaregistry.Required)
	if f.Outcome != NotDelivered {
		t.Errorf("outcome = %q, want %q", f.Outcome, NotDelivered)
	}
	if !detailNames(f, "no traces arrived") {
		t.Errorf("the detail does not say the signal never arrived: %v", f.Detail)
	}
	for _, d := range f.Detail {
		if strings.Contains(d, "not in use") {
			t.Errorf("an attribute is reported missing from telemetry that never arrived: %q", d)
		}
	}
}

// The presence reading answers the never-arrived question when the row
// carries one, even where the attribute-name reading names something.
func TestSchemaPresenceReadingDecidesArrival(t *testing.T) {
	ev := schemaEvidence(t, requirements.Traces, conformingSpan()...)
	ev.Observed = map[time.Duration]telemetry.Observed{
		schemaWindow: {
			AsOf:   time.Now(),
			Window: schemaWindow,
			Signals: map[requirements.SignalKind]telemetry.SignalObservation{
				requirements.Traces: {Known: true, Present: false},
			},
		},
	}
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}), ev)

	if got := findingAt(t, v, schemaregistry.Required).Outcome; got != NotDelivered {
		t.Errorf("outcome = %q, want %q", got, NotDelivered)
	}
}

// A provider that cannot produce the reading gives unknown with its cause,
// never a pass (ADR-0008).
func TestSchemaUnreadableSignalIsUnknown(t *testing.T) {
	ev := schemaEvidence(t, requirements.Traces)
	ev.Schema.Names[SchemaReading{Kind: requirements.Traces, Window: schemaWindow}] = telemetry.AttributeNames{
		Known: false,
		Cause: telemetry.NotServiceScoped(telemetry.Service{Name: "checkout", Environment: "production"}, "the index holds no service dimension"),
	}
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}), ev)

	f := findingAt(t, v, schemaregistry.Required)
	if f.Outcome != Unknown {
		t.Errorf("outcome = %q, want %q", f.Outcome, Unknown)
	}
	if f.Outcome.Passing() {
		t.Error("an unreadable schema requirement passes")
	}
	if !detailNames(f, "cannot scope this reading") {
		t.Errorf("the provider's cause is not preserved: %v", f.Detail)
	}
}

// A reading nobody took is unknown too, and says so in the window's terms.
func TestSchemaMissingReadingIsUnknown(t *testing.T) {
	ev := schemaEvidence(t, requirements.Logs, "enterprise.criticality_tier")
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}), ev)

	f := findingAt(t, v, schemaregistry.Required)
	if f.Outcome != Unknown {
		t.Errorf("outcome = %q, want %q (detail: %v)", f.Outcome, Unknown, f.Detail)
	}
	if !detailNames(f, "no attribute-name reading covers traces") {
		t.Errorf("the detail does not name the missing reading: %v", f.Detail)
	}
}

// A Schema Registry version the evaluation does not carry is unknown: what
// the scope demands is not known, so nothing can be judged against it.
func TestSchemaMissingRegistryVersionIsUnknown(t *testing.T) {
	v := evaluateSchema(t, schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}), Evidence{})

	if len(v.Findings) != 1 {
		t.Fatalf("got %d findings, want one: %v", len(v.Findings), details(v))
	}
	f := v.Findings[0]
	if f.Outcome != Unknown || f.Weight() != ownership.Violation {
		t.Errorf("finding = (%q, %q), want (unknown, violation)", f.Outcome, f.Weight())
	}
	if !detailNames(f, snapshotRef) {
		t.Errorf("the detail does not name the version it wanted: %v", f.Detail)
	}
}

// A truncated reading cannot tell an attribute that is absent from one it
// did not sample, so a required miss read off one is unknown rather than a
// manufactured red. Presence is still proof, so a truncated reading with
// nothing missing stays compliant.
func TestSchemaTruncatedReadingWithdrawsAMiss(t *testing.T) {
	req := schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}})
	key := SchemaReading{Kind: requirements.Traces, Window: schemaWindow}

	miss := schemaEvidence(t, requirements.Traces, "db.namespace", "db.system.name", "server.address")
	reading := miss.Schema.Names[key]
	reading.Truncated, reading.SampledRecords, reading.TotalRecords = true, 500, 90000
	miss.Schema.Names[key] = reading

	f := findingAt(t, evaluateSchema(t, req, miss), schemaregistry.Required)
	if f.Outcome != Unknown {
		t.Errorf("outcome = %q, want %q on a truncated reading with a miss", f.Outcome, Unknown)
	}
	if !detailNames(f, "truncated") {
		t.Errorf("the detail does not report the truncation: %v", f.Detail)
	}

	clean := schemaEvidence(t, requirements.Traces, conformingSpan()...)
	reading = clean.Schema.Names[key]
	reading.Truncated, reading.SampledRecords, reading.TotalRecords = true, 500, 90000
	clean.Schema.Names[key] = reading

	if got := findingAt(t, evaluateSchema(t, req, clean), schemaregistry.Required).Outcome; got != Compliant {
		t.Errorf("outcome = %q, want %q: a name a truncated reading did name is still proof", got, Compliant)
	}
}

// An attribute two groups demand at two levels is judged at the stricter
// one: tightening a level locally is what a custom registry is for, so the
// tightening has to win.
func TestSchemaStrictestDeclaredLevelWins(t *testing.T) {
	// registry.db declares db.namespace with no level, so the default
	// recommended; span.db.client references it at required. The namespace
	// scope reaches both.
	reg := registry(t)
	demands := demandsOf(reg, requirements.Scope{Namespaces: []string{"db"}})

	var found bool
	for _, d := range demands {
		if d.Attribute != "db.namespace" {
			continue
		}
		found = true
		if d.Level != schemaregistry.Required {
			t.Errorf("db.namespace demanded at %q, want %q", d.Level, schemaregistry.Required)
		}
	}
	if !found {
		t.Fatalf("the db namespace demands nothing called db.namespace: %+v", demands)
	}
}

// An attribute the registry declares no level for is demanded at the
// upstream default, which is recommended and never required: inventing a
// floor the registry never stated would fail a Service on nobody's rule.
func TestSchemaUndeclaredLevelDefaultsToRecommended(t *testing.T) {
	demands := demandsOf(registry(t), requirements.Scope{Groups: []string{"registry.db"}})
	for _, d := range demands {
		if d.Level != schemaregistry.Recommended {
			t.Errorf("%s demanded at %q, want %q: registry.db declares no levels", d.Attribute, d.Level, schemaregistry.Recommended)
		}
	}
	if len(demands) == 0 {
		t.Fatal("registry.db demands nothing")
	}
}

// The scope is judged across every signal it covers, worst-first: being
// blind on one leg is not a pass on the scope.
func TestSchemaWorstSignalWins(t *testing.T) {
	req := schemaRequirement(requirements.Scope{Namespaces: []string{"enterprise"}}, requirements.Logs, requirements.Traces)
	ev := schemaEvidence(t, requirements.Traces, "enterprise.criticality_tier", "enterprise.cost_centre", "enterprise.owner_email")

	// Traces are clean; logs were never read at all.
	f := findingAt(t, evaluateSchema(t, req, ev), schemaregistry.Recommended)
	if f.Outcome != Unknown {
		t.Errorf("outcome = %q, want %q: one covered signal has no reading", f.Outcome, Unknown)
	}
}

// The reading's own verdict is not downgraded by a check that runs after it.
// A scope with one covered signal that never arrived and another that arrived
// in the wrong shape reads not_delivered: telemetry that is not there is the
// larger fact, and the severity ordering already says so.
func TestSchemaNeverArrivedOutranksAWrongShape(t *testing.T) {
	req := schemaRequirement(requirements.Scope{Namespaces: []string{"enterprise"}}, requirements.Logs, requirements.Traces)
	ev := schemaEvidence(t, requirements.Traces, "enterprise.cost_centre")
	ev.Schema.Names[SchemaReading{Kind: requirements.Logs, Window: schemaWindow}] = telemetry.AttributeNames{
		Known: true, Window: schemaWindow,
	}

	// enterprise.criticality_tier is demanded at recommended by
	// registry.enterprise, so the recommended finding carries both facts.
	f := findingAt(t, evaluateSchema(t, req, ev), schemaregistry.Recommended)
	if f.Outcome != NotDelivered {
		t.Errorf("outcome = %q, want %q (detail: %v)", f.Outcome, NotDelivered, f.Detail)
	}
	if !detailNames(f, "no logs arrived") {
		t.Errorf("the detail does not say the signal never arrived: %v", f.Detail)
	}
}

// A requirement placed at the live tap reads unknown rather than clean: the
// tap is not built, and a dead tap must not read as compliant (ADR-0034 §6).
func TestSchemaLivePlacementIsUnknown(t *testing.T) {
	req := schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}})
	req.Placement = requirements.Live

	v := evaluateSchema(t, req, schemaEvidence(t, requirements.Traces, conformingSpan()...))
	if len(v.Findings) != 1 || v.Findings[0].Outcome != Unknown {
		t.Fatalf("findings = %v, want one unknown", details(v))
	}
}

// Everything the Effective × Observed cross produces stays violation-grade,
// so the grade field changes no existing verdict.
func TestCrossFindingsStayViolationGrade(t *testing.T) {
	lib := requirements.Library{Requirements: map[string]requirements.Requirement{
		"logs-flow": {
			ID: "logs-flow", Owner: "platform", Level: requirements.Required,
			Signal: &requirements.SignalAssertion{Kind: requirements.Logs, Present: true, Window: requirements.Duration(time.Hour)},
		},
	}}
	v := Evaluate(Row{Service: "checkout", Environment: "production"}, lib, Evidence{}, time.Now())

	if len(v.Findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(v.Findings))
	}
	if got := v.Findings[0].Weight(); got != ownership.Violation {
		t.Errorf("grade = %q, want %q", got, ownership.Violation)
	}
	if !v.Findings[0].Scored() {
		t.Error("a cross finding does not feed the binary")
	}
}
