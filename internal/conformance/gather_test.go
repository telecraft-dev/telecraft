package conformance

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// installedRegistries writes the fixture Schema Registry version out as an
// installed artefact, the way an import run would, so a library can be
// loaded through the same door a command loads one through.
func installedRegistries(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, _, err := registry(t).Write(dir); err != nil {
		t.Fatalf("installing the fixture Schema Registry: %v", err)
	}
	return dir
}

// fixtureLibrary loads the requirements package's own fixture library,
// which holds one pinned schema reference and one tracking one. It is
// reused rather than copied for the reason a requirement references a
// registry rather than copying one: a second copy drifts from the first.
func fixtureLibrary(t *testing.T) requirements.Library {
	t.Helper()
	lib, err := requirements.Load(
		filepath.Join("..", "requirements", "testdata", "library"),
		requirements.WithSchemaRegistries(installedRegistries(t)))
	if err != nil {
		t.Fatalf("the fixture library does not load: %v", err)
	}
	return lib
}

// The fetch plan is one reading per signal and window covered, shared by
// every requirement that covers them: a reading per requirement would pay
// twice for the same answer.
func TestSchemaReadingsPlanOneReadingPerSignalAndWindow(t *testing.T) {
	lib := requirements.Library{Requirements: map[string]requirements.Requirement{
		"traces-24h": {ID: "traces-24h", Schema: &requirements.SchemaAssertion{
			Signals: []requirements.SignalKind{requirements.Traces},
			Window:  requirements.Duration(24 * time.Hour),
		}},
		"also-traces-24h": {ID: "also-traces-24h", Schema: &requirements.SchemaAssertion{
			Signals: []requirements.SignalKind{requirements.Traces, requirements.Logs},
			Window:  requirements.Duration(24 * time.Hour),
		}},
		"traces-6h": {ID: "traces-6h", Schema: &requirements.SchemaAssertion{
			Signals: []requirements.SignalKind{requirements.Traces},
			Window:  requirements.Duration(6 * time.Hour),
		}},
		"staging-only": {ID: "staging-only", Environments: []string{"staging"},
			Schema: &requirements.SchemaAssertion{
				Signals: []requirements.SignalKind{requirements.Metrics},
				Window:  requirements.Duration(6 * time.Hour),
			}},
		"not-a-schema-requirement": {ID: "not-a-schema-requirement", Signal: &requirements.SignalAssertion{
			Kind: requirements.Logs, Window: requirements.Duration(time.Hour),
		}},
	}}

	got := SchemaReadings(lib, "production")
	want := []SchemaReading{
		{Kind: requirements.Traces, Window: 6 * time.Hour},
		{Kind: requirements.Logs, Window: 24 * time.Hour},
		{Kind: requirements.Traces, Window: 24 * time.Hour},
	}
	if len(got) != len(want) {
		t.Fatalf("plan = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("plan[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	// The staging-only requirement's metrics reading is not in the
	// production plan: evidence for two environments never meets in one
	// Evidence value (ADR-0033), so it is not fetched for this row either.
	for _, r := range got {
		if r.Kind == requirements.Metrics {
			t.Errorf("the production plan fetches %v, which only a staging requirement covers", r)
		}
	}
}

// A library with no schema requirement in this Environment gathers nothing:
// there is no verdict to feed, and a round trip nobody reads is a round trip
// nobody should pay for.
func TestGatherSchemaReadsNothingWithoutASchemaRequirement(t *testing.T) {
	lib := requirements.Library{Requirements: map[string]requirements.Requirement{
		"logs-delivered": {ID: "logs-delivered", Signal: &requirements.SignalAssertion{
			Kind: requirements.Logs, Window: requirements.Duration(time.Hour),
		}},
	}}

	reads := 0
	ev := GatherSchema(lib, "production", SchemaSource{
		Names: func(SchemaReading) telemetry.AttributeNames {
			reads++
			return telemetry.AttributeNames{Known: true}
		},
		Groups: func(SchemaReading) telemetry.GroupNames {
			reads++
			return telemetry.GroupNames{Known: true}
		},
		Values: func(SchemaValueReading) telemetry.DistinctValues {
			reads++
			return telemetry.DistinctValues{Known: true}
		},
	})
	if reads != 0 {
		t.Errorf("took %d readings for a library that asks for none", reads)
	}
	if len(ev.Names) != 0 || len(ev.Groups) != 0 || len(ev.Values) != 0 || len(ev.Versions) != 0 {
		t.Errorf("gathered evidence %+v, want none", ev)
	}
}

// The gathered evidence is what the verdict is judged against: the versions
// the load resolved, and the reading each planned key asked for, filed under
// the key it was planned as.
func TestGatherSchemaFilesEachReadingUnderThePlannedKey(t *testing.T) {
	lib := fixtureLibrary(t)

	asked := map[SchemaReading]bool{}
	ev := GatherSchema(lib, "production", SchemaSource{
		Names: func(r SchemaReading) telemetry.AttributeNames {
			asked[r] = true
			return telemetry.AttributeNames{Known: true, Window: r.Window, Names: []string{string(r.Kind)}}
		},
	})

	if ev.Versions[snapshotRef] == nil {
		t.Fatalf("the evidence carries no resolved registry: %v", ev.Versions)
	}
	plan := SchemaReadings(lib, "production")
	if len(plan) == 0 {
		t.Fatal("the fixture library plans no readings, so this proves nothing")
	}
	for _, key := range plan {
		if !asked[key] {
			t.Errorf("planned reading %v was never taken", key)
		}
		names, filed := ev.Names[key]
		if !filed {
			t.Fatalf("reading %v is not filed under the key it was planned as", key)
		}
		if len(names.Names) != 1 || names.Names[0] != string(key.Kind) {
			t.Errorf("reading %v carries %v, which is another key's answer", key, names.Names)
		}
	}
}

// The Observed fetch plan carries the schema windows too. A schema verdict
// tells "nothing arrived" from "arrived in the wrong shape" by reading
// presence over its own window (ADR-0034 §3), and a plan that skipped it
// would leave the evaluator answering that from a fallback.
func TestWindowsCoverSignalAndSchemaAssertions(t *testing.T) {
	lib := requirements.Library{Requirements: map[string]requirements.Requirement{
		"signal": {ID: "signal", Signal: &requirements.SignalAssertion{
			Kind: requirements.Logs, Window: requirements.Duration(time.Hour),
		}},
		"schema": {ID: "schema", Schema: &requirements.SchemaAssertion{
			Signals: []requirements.SignalKind{requirements.Traces},
			Window:  requirements.Duration(24 * time.Hour),
		}},
		"also-an-hour": {ID: "also-an-hour", Signal: &requirements.SignalAssertion{
			Kind: requirements.Traces, Window: requirements.Duration(time.Hour),
		}},
		"elsewhere": {ID: "elsewhere", Environments: []string{"staging"},
			Signal: &requirements.SignalAssertion{
				Kind: requirements.Logs, Window: requirements.Duration(72 * time.Hour),
			}},
	}}

	got := Windows(lib, "production")
	want := []time.Duration{time.Hour, 24 * time.Hour}
	if len(got) != len(want) {
		t.Fatalf("windows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("windows[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// The whole path, end to end, through the production seams: a library
// loaded against installed registries, evidence gathered from a reading, and
// a verdict that says something. Before the wiring existed this row read
// unknown whatever the telemetry said, because nothing populated the schema
// evidence.
func TestASchemaRequirementJudgedAgainstAReadingProducesARealVerdict(t *testing.T) {
	lib := fixtureLibrary(t)
	arrived := map[time.Duration]telemetry.Observed{}
	for _, w := range Windows(lib, "production") {
		arrived[w] = telemetry.Observed{Window: w, Signals: map[requirements.SignalKind]telemetry.SignalObservation{
			requirements.Traces: {Known: true, Present: true, Volume: 12},
			requirements.Logs:   {Known: true, Present: true, Volume: 12},
		}}
	}

	for name, tc := range map[string]struct {
		inUse []string
		want  Outcome
	}{
		"every required attribute in use": {inUse: conformingSpan(), want: Compliant},
		"one required attribute missing": {
			inUse: []string{"db.namespace", "db.system.name", "enterprise.criticality_tier"},
			want:  Misconfigured,
		},
	} {
		t.Run(name, func(t *testing.T) {
			ev := Evidence{
				Observed: arrived,
				Schema:   GatherSchema(lib, "production", conformingSource(t, tc.inUse...)),
			}
			v := Evaluate(Row{Service: "checkout", Environment: "production"}, lib, ev, time.Now())

			f, found := findingFor(v, "db-spans-conform")
			if !found {
				t.Fatalf("no violation-grade finding for db-spans-conform in %v", details(v))
			}
			if f.Outcome != tc.want {
				t.Errorf("outcome = %q, want %q. Detail: %v", f.Outcome, tc.want, f.Detail)
			}
		})
	}
}

// A signal that never arrived is not_delivered, not misconfigured: the shape
// of what arrived cannot be judged when nothing did.
func TestASchemaRequirementOverASilentSignalIsNotDelivered(t *testing.T) {
	lib := fixtureLibrary(t)
	ev := Evidence{
		Observed: map[time.Duration]telemetry.Observed{},
		Schema:   GatherSchema(lib, "production", conformingSource(t)),
	}
	for _, w := range Windows(lib, "production") {
		ev.Observed[w] = telemetry.Observed{Window: w, Signals: map[requirements.SignalKind]telemetry.SignalObservation{
			requirements.Traces: {Known: true, Present: false},
			requirements.Logs:   {Known: true, Present: false},
		}}
	}

	v := Evaluate(Row{Service: "checkout", Environment: "production"}, lib, ev, time.Now())
	f, found := findingFor(v, "db-spans-conform")
	if !found {
		t.Fatalf("no violation-grade finding for db-spans-conform in %v", details(v))
	}
	if f.Outcome != NotDelivered {
		t.Errorf("outcome = %q, want %q. Detail: %v", f.Outcome, NotDelivered, f.Detail)
	}
}

// The other half of the same wiring: a reference the evaluation cannot
// resolve still reads unknown with a cause, never a pass. The fixture's
// tracking reference is that case today, because which installed version is
// active is an activation decision and no load makes it.
func TestAReferenceNothingResolvedStaysUnknownWithACause(t *testing.T) {
	lib := fixtureLibrary(t)
	ev := Evidence{Schema: GatherSchema(lib, "production", conformingSource(t, conformingSpan()...))}

	v := Evaluate(Row{Service: "checkout", Environment: "production"}, lib, ev, time.Now())
	f, found := findingFor(v, "enterprise-attributes-tracked")
	if !found {
		t.Fatalf("no violation-grade finding for enterprise-attributes-tracked in %v", details(v))
	}
	if f.Outcome != Unknown {
		t.Fatalf("outcome = %q, want unknown: a reference nobody resolved must never pass", f.Outcome)
	}
	if len(f.Detail) == 0 || !strings.Contains(strings.Join(f.Detail, "; "), "Schema Registry version") {
		t.Errorf("detail %v does not say which version was unavailable", f.Detail)
	}
	if f.Remediation == "" {
		t.Error("an unknown verdict with no fix is a complaint")
	}
}

// conformingSource answers every planned reading the way a backend carrying
// exactly the named attributes would: those names in use, every registry
// group arrived, and every enum carrying exactly what the registry declares.
// It is the shape a caller has to answer in, all three readings together, so
// a test that only declared names would leave the value readings unknown and
// the verdict with them.
func conformingSource(t *testing.T, inUse ...string) SchemaSource {
	t.Helper()
	reg := registry(t)
	return SchemaSource{
		Names: func(r SchemaReading) telemetry.AttributeNames {
			return telemetry.AttributeNames{Known: true, Window: r.Window, Names: inUse}
		},
		Groups: func(r SchemaReading) telemetry.GroupNames {
			return telemetry.GroupNames{Known: true, Window: r.Window, Key: telemetry.GroupKeyFor(r.Kind), Names: groupNamesOf(reg, r.Kind)}
		},
		Values: func(r SchemaValueReading) telemetry.DistinctValues {
			return conformingValues(r.Attribute, declaredIn(reg, r.Attribute))
		},
	}
}

// groupNamesOf is every grouping-key value the registry declares for one
// signal: the reading a backend carrying all of them would give.
func groupNamesOf(reg *schemaregistry.Registry, kind requirements.SignalKind) []string {
	var out []string
	for _, g := range reg.Groups {
		if k, name, ok := groupKeyValue(g); ok && k == kind {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// The active version travels with the evidence, filed under its own ref,
// and the library's own map is left alone: it is the library's, and every
// other gather reads it.
func TestGatherSchemaCarriesTheActiveVersionWithoutTouchingTheLibrary(t *testing.T) {
	lib := fixtureLibrary(t)
	active := registry(t)
	active.Source.Ref = "v1.5.0"

	ev := GatherSchema(lib, "production", SchemaSource{
		Names: func(r SchemaReading) telemetry.AttributeNames {
			return telemetry.AttributeNames{Known: true, Window: r.Window}
		},
	}, WithActiveSchemaRegistry(active))

	if ev.ActiveVersion != "v1.5.0" {
		t.Errorf("ActiveVersion = %q, want v1.5.0", ev.ActiveVersion)
	}
	if got, _, ok := ev.ActiveRegistry(); !ok || got != active {
		t.Errorf("ActiveRegistry() = (%v, %v), want the version the option handed over", got, ok)
	}
	if ev.Versions[snapshotRef] == nil {
		t.Errorf("the pinned version fell out of the evidence: %v", ev.Versions)
	}
	if lib.SchemaRegistries["v1.5.0"] != nil {
		t.Error("the gather wrote the active version into the library's map")
	}

	bare := GatherSchema(lib, "production", SchemaSource{}, WithActiveSchemaRegistry(nil))
	if bare.ActiveVersion != "" {
		t.Errorf("a nil registry set ActiveVersion = %q, want none: nil is no designation", bare.ActiveVersion)
	}
}

// The value plan widens to what the active version demands of a pinned
// scope: an enum only the active version declares, in use, buys a reading
// the pin alone would not have, so the drift arm judges values rather than
// reporting them unknown.
func TestGatherSchemaWidensTheValuePlanForTheActiveVersion(t *testing.T) {
	pinned := registry(t)
	active := registry(t)
	active.Source.Ref = "v1.5.0"
	declared := false
	for i, g := range active.Groups {
		for j, a := range g.Attributes {
			if a.ID == "enterprise.cost_centre" {
				active.Groups[i].Attributes[j].Members = []schemaregistry.Member{{ID: "platform", Value: "platform"}}
				declared = true
			}
		}
	}
	if !declared {
		t.Fatal("the fixture registry has no enterprise.cost_centre definition to declare members for")
	}

	lib := requirements.Library{
		Requirements: map[string]requirements.Requirement{"enterprise-attrs": {
			ID: "enterprise-attrs", Owner: "platform-observability",
			Schema: &requirements.SchemaAssertion{
				RegistryVersion: snapshotRef,
				Scope:           requirements.Scope{Namespaces: []string{"enterprise"}},
				Signals:         []requirements.SignalKind{requirements.Traces},
				Window:          requirements.Duration(schemaWindow),
			},
		}},
		SchemaRegistries: map[string]*schemaregistry.Registry{snapshotRef: pinned},
	}
	src := SchemaSource{
		Names: func(r SchemaReading) telemetry.AttributeNames {
			return telemetry.AttributeNames{Known: true, Window: r.Window, Names: []string{"enterprise.cost_centre"}}
		},
		Values: func(r SchemaValueReading) telemetry.DistinctValues {
			return telemetry.DistinctValues{Known: true, Window: r.Window, Attribute: r.Attribute}
		},
	}

	key := SchemaValueReading{Kind: requirements.Traces, Window: schemaWindow, Attribute: "enterprise.cost_centre"}
	if ev := GatherSchema(lib, "production", src); len(ev.Values) != 0 {
		t.Errorf("the pin alone bought value readings %v: nothing it declares as an enum is in use", ev.Values)
	}
	ev := GatherSchema(lib, "production", src, WithActiveSchemaRegistry(active))
	if _, taken := ev.Values[key]; !taken {
		t.Errorf("the widened plan did not buy the active version's enum reading: %v", ev.Values)
	}
}

// findingFor returns one requirement's violation-grade finding: the one
// that is its verdict, as opposed to the improvement and information
// findings that ride alongside it (ADR-0034 §3).
func findingFor(v Verdict, id string) (Finding, bool) {
	for _, f := range v.Findings {
		if f.Requirement.ID == id && f.Scored() {
			return f, true
		}
	}
	return Finding{}, false
}
