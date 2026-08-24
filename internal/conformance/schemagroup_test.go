package conformance

import (
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// dbMetric is the metric.name the fixture registry declares for the one group
// a grouping-key reading can locate.
const dbMetric = "db.client.operation.duration"

// metricScope is a scope reaching that group and nothing else: it demands
// db.system.name at required, of metrics.
func metricScope() requirements.Scope {
	return requirements.Scope{Groups: []string{"metric.db.client.operation.duration"}}
}

// metricEvidence builds evidence over the metrics leg: the fixture registry,
// an attribute-name reading, and the grouping-key reading that says which
// metrics arrived.
func metricEvidence(t *testing.T, groups []string, inUse ...string) Evidence {
	t.Helper()
	ev := schemaEvidence(t, requirements.Metrics, inUse...)
	ev.Schema.Groups = map[SchemaReading]telemetry.GroupNames{
		{Kind: requirements.Metrics, Window: schemaWindow}: {
			Known:  true,
			AsOf:   time.Now(),
			Window: schemaWindow,
			Key:    telemetry.MetricName,
			Names:  groups,
		},
	}
	return ev
}

func metricRequirement() requirements.Requirement {
	return schemaRequirement(metricScope(), requirements.Metrics)
}

// A group that arrived is judged against its own required-set, and a required
// attribute it does not carry is the wrong shape: misconfigured.
func TestSchemaArrivedGroupIsJudgedAgainstItsRequiredSet(t *testing.T) {
	ev := metricEvidence(t, []string{dbMetric, "http.server.request.duration"}, "service.name")

	f := findingAt(t, evaluateSchema(t, metricRequirement(), ev), schemaregistry.Required)
	if f.Outcome != Misconfigured {
		t.Errorf("outcome = %q, want %q: the metric arrived and does not carry what its group demands (detail: %v)", f.Outcome, Misconfigured, f.Detail)
	}
	if !detailNames(f, "db.system.name") {
		t.Errorf("the detail does not name the missing attribute: %v", f.Detail)
	}
}

// The distinction the flat union could not draw. A group that never arrived
// reads differently from one that arrived missing a required attribute: its
// required-set is not in play, nothing it alone demands is judged, and the
// verdict is not_delivered rather than a red naming a fix nobody can make.
func TestSchemaAbsentGroupIsNotDeliveredRatherThanAViolation(t *testing.T) {
	ev := metricEvidence(t, []string{"http.server.request.duration"}, "service.name")

	v := evaluateSchema(t, metricRequirement(), ev)
	f := findingAt(t, v, schemaregistry.Required)

	if f.Outcome != NotDelivered {
		t.Errorf("outcome = %q, want %q: the group never arrived, so its required-set cannot be violated (detail: %v)", f.Outcome, NotDelivered, f.Detail)
	}
	if !detailNames(f, dbMetric) {
		t.Errorf("the detail does not name the group that never arrived: %v", f.Detail)
	}
	if !detailNames(f, "not in play") {
		t.Errorf("the detail does not say the required-set is out of play: %v", f.Detail)
	}
	if detailNames(f, "not in use") {
		t.Errorf("an attribute is reported missing from a group that never arrived: %v", f.Detail)
	}
	// No new outcome: not_delivered is ADR-0034 §3's own mapping, applied
	// at the grain below the signal.
	if len(Outcomes()) != 8 {
		t.Fatalf("Outcomes() has %d entries, want eight", len(Outcomes()))
	}
}

// An attribute a group that never arrived demands is still judged when
// another group in play demands it too: the group drops out of the scope, not
// the attribute out of the registry.
func TestSchemaAbsentGroupDoesNotWithdrawAnotherGroupsDemand(t *testing.T) {
	// The db namespace reaches registry.db, span.db.client and the metric
	// group. Judged on metrics, only the metric group is locatable, and it
	// did not arrive; db.system.name is still demanded by span.db.client.
	req := schemaRequirement(requirements.Scope{Namespaces: []string{"db"}}, requirements.Metrics)
	ev := metricEvidence(t, []string{"http.server.request.duration"}, "db.namespace")

	f := findingAt(t, evaluateSchema(t, req, ev), schemaregistry.Required)
	if !detailNames(f, "db.system.name") {
		t.Errorf("the detail does not report the attribute another group still demands: %v", f.Detail)
	}
}

// A truncated group reading cannot tell a group that did not arrive from one
// it did not sample, so neither answer is available: unknown, and the group
// is neither judged nor written off.
func TestSchemaTruncatedGroupReadingIsUnknown(t *testing.T) {
	ev := metricEvidence(t, []string{"http.server.request.duration"}, "service.name")
	key := SchemaReading{Kind: requirements.Metrics, Window: schemaWindow}
	reading := ev.Schema.Groups[key]
	reading.Truncated = true
	ev.Schema.Groups[key] = reading

	f := findingAt(t, evaluateSchema(t, metricRequirement(), ev), schemaregistry.Required)
	if f.Outcome != Unknown {
		t.Errorf("outcome = %q, want %q (detail: %v)", f.Outcome, Unknown, f.Detail)
	}
	if !detailNames(f, "did not sample") {
		t.Errorf("the detail does not report the truncation: %v", f.Detail)
	}
}

// Presence in the group reading is proof the group arrived, whatever else the
// reading missed: extra records can only add group names.
func TestSchemaTruncatedGroupReadingThatNamesTheGroupStillJudgesIt(t *testing.T) {
	ev := metricEvidence(t, []string{dbMetric}, "db.system.name")
	key := SchemaReading{Kind: requirements.Metrics, Window: schemaWindow}
	reading := ev.Schema.Groups[key]
	reading.Truncated = true
	ev.Schema.Groups[key] = reading

	f := findingAt(t, evaluateSchema(t, metricRequirement(), ev), schemaregistry.Required)
	if f.Outcome != Compliant {
		t.Errorf("outcome = %q, want %q: the reading named the group, which is proof it arrived (detail: %v)", f.Outcome, Compliant, f.Detail)
	}
}

// A grouping-key reading nobody took leaves the arrival unknown, so whether
// the required-set is in play is unknown too. Judging it anyway would be the
// silent approximation ADR-0034 §4 forbids.
func TestSchemaMissingGroupReadingIsUnknown(t *testing.T) {
	ev := metricEvidence(t, []string{dbMetric}, "db.system.name")
	ev.Schema.Groups = map[SchemaReading]telemetry.GroupNames{}

	f := findingAt(t, evaluateSchema(t, metricRequirement(), ev), schemaregistry.Required)
	if f.Outcome != Unknown {
		t.Errorf("outcome = %q, want %q (detail: %v)", f.Outcome, Unknown, f.Detail)
	}
	if !detailNames(f, "metric.name") {
		t.Errorf("the detail does not name the reading it wanted: %v", f.Detail)
	}
}

// A provider that could not take the reading keeps its cause.
func TestSchemaUnreadableGroupReadingKeepsItsCause(t *testing.T) {
	ev := metricEvidence(t, nil, "db.system.name")
	ev.Schema.Groups[SchemaReading{Kind: requirements.Metrics, Window: schemaWindow}] = telemetry.GroupNamesUnknown(
		time.Now(), schemaWindow, requirements.Metrics,
		telemetry.NotServiceScoped(telemetry.Service{Name: "checkout", Environment: "production"}, "the index holds no service dimension"))

	f := findingAt(t, evaluateSchema(t, metricRequirement(), ev), schemaregistry.Required)
	if f.Outcome != Unknown {
		t.Errorf("outcome = %q, want %q", f.Outcome, Unknown)
	}
	if !detailNames(f, "cannot scope this reading") {
		t.Errorf("the provider's cause is not preserved: %v", f.Detail)
	}
}

// A group the registry states no grouping-key value for cannot be located, so
// its demands stay in play and are judged against the scope's own reading.
// The registry declares no span name and this model carries no event name, so
// today that is every group but a metric group.
func TestSchemaUnlocatableGroupsStayInPlay(t *testing.T) {
	reg := registry(t)
	for _, tc := range []struct {
		group     string
		locatable bool
		kind      requirements.SignalKind
	}{
		{group: "metric.db.client.operation.duration", locatable: true, kind: requirements.Metrics},
		{group: "span.db.client"},
		{group: "entity.service"},
		{group: "registry.db"},
	} {
		g, ok := reg.Group(tc.group)
		if !ok {
			t.Fatalf("the fixture registry has no group %q", tc.group)
		}
		kind, _, locatable := groupKeyValue(g)
		if locatable != tc.locatable {
			t.Errorf("groupKeyValue(%s) locatable = %v, want %v", tc.group, locatable, tc.locatable)
		}
		if locatable && kind != tc.kind {
			t.Errorf("groupKeyValue(%s) kind = %q, want %q", tc.group, kind, tc.kind)
		}
	}

	// A span scope judged with no grouping-key reading at all is unaffected
	// by this check: nothing about it was locatable, so nothing about it is
	// unknown.
	f := findingAt(t, evaluateSchema(t, schemaRequirement(dbSpans()), schemaEvidence(t, requirements.Traces, conformingSpan()...)), schemaregistry.Required)
	if f.Outcome != Compliant {
		t.Errorf("outcome = %q, want %q: a span group is judged as it always was (detail: %v)", f.Outcome, Compliant, f.Detail)
	}
}

// The plan buys a grouping-key reading only where a scope reaches a group one
// can locate. A scope of span and attribute groups pays for none.
func TestSchemaGroupReadingsPlanOnlyWhereAGroupIsLocatable(t *testing.T) {
	lib := fixtureLibrary(t)
	if got := SchemaGroupReadings(lib, "production"); len(got) != 0 {
		t.Errorf("planned %v for a library whose scopes reach no locatable group", got)
	}

	reg := registry(t)
	lib = requirements.Library{
		Requirements:     map[string]requirements.Requirement{"metrics": metricRequirement()},
		SchemaRegistries: map[string]*schemaregistry.Registry{snapshotRef: reg},
	}
	lib.Requirements["metrics"] = withID(lib.Requirements["metrics"], "metrics")

	got := SchemaGroupReadings(lib, "production")
	want := SchemaReading{Kind: requirements.Metrics, Window: schemaWindow}
	if len(got) != 1 || got[0] != want {
		t.Errorf("plan = %v, want %v", got, []SchemaReading{want})
	}
}

func withID(req requirements.Requirement, id string) requirements.Requirement {
	req.ID = id
	return req
}
