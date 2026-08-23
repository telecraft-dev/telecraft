package conformance

import (
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/renderer"
)

var readAt = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

// checkoutTopology is the shape the derivation exists for: checkout's Path
// runs edge then gateway in production, so the edge Tier is the collector
// nearest the Service.
func checkoutTopology() renderer.Topology {
	return renderer.Topology{
		Tiers: map[string]renderer.Tier{
			"shop/edge": {
				Team: "shop", Name: "edge", Environment: "production",
				Selector: map[string]string{"telecraft.tier": "edge", "deployment.environment": "production"},
			},
			"data-flow/gateway": {
				Team: "data-flow", Name: "gateway", Environment: "production",
				Selector: map[string]string{"telecraft.tier": "gateway", "deployment.environment": "production"},
			},
		},
		Services: map[string]renderer.Service{
			"shop/checkout": {
				Team: "shop", Name: "checkout", Class: "C1",
				Paths: []renderer.Path{{Through: []string{"shop/edge", "data-flow/gateway"}}},
			},
		},
	}
}

// edgeCollector is one collector matched into shop/edge, reporting the
// given pipelines as of the reading instant.
func edgeCollector(instance string, pipelines ...estate.Pipeline) estate.Collector {
	return estate.Collector{
		Identity: map[string]string{
			"telecraft.tier":         "edge",
			"deployment.environment": "production",
			"service.instance.id":    instance,
		},
		Effective: estate.Effective{Known: true, AsOf: readAt, Pipelines: pipelines},
	}
}

// gatewayCollector is one collector matched into data-flow/gateway.
func gatewayCollector(instance string, pipelines ...estate.Pipeline) estate.Collector {
	return estate.Collector{
		Identity: map[string]string{
			"telecraft.tier":         "gateway",
			"deployment.environment": "production",
			"service.instance.id":    instance,
		},
		Effective: estate.Effective{Known: true, AsOf: readAt, Pipelines: pipelines},
	}
}

func filelogPipeline() estate.Pipeline {
	return estate.Pipeline{Name: "logs", Receivers: []string{"filelog"}, Exporters: []string{"otlphttp"}}
}

func otlpPipeline() estate.Pipeline {
	return estate.Pipeline{Name: "logs", Receivers: []string{"otlp"}, Exporters: []string{"otlphttp"}}
}

// reading wraps collectors as one estate reading with a cadence long
// enough that nothing demotes for age.
func reading(collectors ...estate.Collector) estate.Estate {
	return estate.Estate{
		Declaration: estate.Declaration{
			Readings:       map[estate.ReadingKind]bool{estate.EffectiveKind: true},
			RefreshCadence: time.Minute,
		},
		AsOf:       readAt,
		Collectors: collectors,
	}
}

// derive runs the derivation over checkoutTopology at the reading instant.
func derive(t *testing.T, est estate.Estate, authored Estate) Estate {
	t.Helper()
	return Derive(Derivation{
		Topology: checkoutTopology(),
		Reading:  est,
		Authored: authored,
		Now:      readAt,
	})
}

// only returns the single row a one-Service topology produces.
func only(t *testing.T, e Estate) EstateRow {
	t.Helper()
	if len(e.Rows) != 1 {
		t.Fatalf("the estate has %d rows, want one for shop/checkout in production: %+v", len(e.Rows), e.Rows)
	}
	return e.Rows[0]
}

func TestRowReadsTheCollectorNearestTheService(t *testing.T) {
	row := only(t, derive(t, reading(
		edgeCollector("edge-1", filelogPipeline()),
		gatewayCollector("gw-1", otlpPipeline()),
	), Estate{}))

	if row.Row != (Row{Service: "shop/checkout", Environment: "production"}) {
		t.Fatalf("row = %+v, want shop/checkout in production", row.Row)
	}
	if !row.Effective.Known {
		t.Fatalf("effective = %+v, want the edge collector's reading", row.Effective)
	}
	if got := row.Effective.Pipelines[0].Receivers; len(got) != 1 || got[0] != "filelog" {
		t.Errorf("receivers = %v, want the first Tier's filelog rather than the gateway's otlp: the row is the collector nearest the Service", got)
	}
}

func TestRowTakesTheServiceClassFromTheTopology(t *testing.T) {
	row := only(t, derive(t, reading(edgeCollector("edge-1", filelogPipeline())), Estate{}))
	if row.Class != "C1" {
		t.Errorf("class = %q, want C1 from the authored Service: the Service Class lives in the topology", row.Class)
	}
}

func TestReplicasThatDisagreeLeaveTheRowUnknown(t *testing.T) {
	row := only(t, derive(t, reading(
		edgeCollector("edge-1", filelogPipeline()),
		edgeCollector("edge-2", otlpPipeline()),
	), Estate{}))

	if row.Effective.Known {
		t.Fatalf("effective = %+v, want an unknown reading: two collectors reporting two configs is not one answer", row.Effective)
	}
	if len(row.Effective.Pipelines) > 0 {
		t.Error("an unknown reading carries no pipelines: nothing downstream may quietly use a winner nobody picked")
	}
	for _, want := range []string{"2 different configs", "shop/edge", "edge-1", "edge-2"} {
		if !strings.Contains(row.Effective.Cause, want) {
			t.Errorf("cause = %q, want it to name %q", row.Effective.Cause, want)
		}
	}
}

func TestReplicasThatAgreeLeaveTheRowKnown(t *testing.T) {
	row := only(t, derive(t, reading(
		edgeCollector("edge-1", filelogPipeline()),
		edgeCollector("edge-2", filelogPipeline()),
	), Estate{}))

	if !row.Effective.Known || len(row.Effective.Pipelines) != 1 {
		t.Fatalf("effective = %+v, want the agreed reading: replicas running the same config are one answer", row.Effective)
	}
}

func TestOneUnreadableReplicaLeavesTheRowUnknown(t *testing.T) {
	quiet := edgeCollector("edge-2")
	quiet.Effective = estate.Effective{Known: false, Cause: "the collector has never reported a config", AsOf: readAt}

	row := only(t, derive(t, reading(edgeCollector("edge-1", filelogPipeline()), quiet), Estate{}))

	if row.Effective.Known {
		t.Fatalf("effective = %+v, want unknown: agreement that cannot be established has not been established", row.Effective)
	}
	if !strings.Contains(row.Effective.Cause, "never reported a config") {
		t.Errorf("cause = %q, want the unreadable collector's own cause carried through", row.Effective.Cause)
	}
}

func TestNoCollectorMatchedLeavesTheRowUnknownRatherThanEmpty(t *testing.T) {
	row := only(t, derive(t, reading(gatewayCollector("gw-1", otlpPipeline())), Estate{}))

	if row.Effective.Known {
		t.Fatalf("effective = %+v, want unknown: a Service whose Tier reports no collector has no reading, not an empty one", row.Effective)
	}
	if !strings.Contains(row.Effective.Cause, "no collector in the estate reading matches") {
		t.Errorf("cause = %q, want it to say nothing matched the first Tier", row.Effective.Cause)
	}
}

func TestACollectorReportingNoPipelinesIsAKnownReadingOfNothing(t *testing.T) {
	row := only(t, derive(t, reading(edgeCollector("edge-1")), Estate{}))

	if !row.Effective.Known {
		t.Fatalf("effective = %+v, want a known reading: a collector reporting an empty config is not a blind spot", row.Effective)
	}
	if len(row.Effective.Pipelines) != 0 {
		t.Errorf("pipelines = %+v, want none", row.Effective.Pipelines)
	}
}

func TestAFirstTierWithNoSelectorLeavesTheRowUnknown(t *testing.T) {
	topo := checkoutTopology()
	edge := topo.Tiers["shop/edge"]
	edge.Selector = nil
	topo.Tiers["shop/edge"] = edge

	est := Derive(Derivation{Topology: topo, Reading: reading(edgeCollector("edge-1", filelogPipeline())), Now: readAt})
	row := only(t, est)

	if row.Effective.Known {
		t.Fatalf("effective = %+v, want unknown: a Tier with no selector has no platform-known population", row.Effective)
	}
	if !strings.Contains(row.Effective.Cause, "declares no selector") {
		t.Errorf("cause = %q, want it to say why no collector could be attributed", row.Effective.Cause)
	}
}

func TestAStaleCollectorCannotFeedARow(t *testing.T) {
	est := reading(edgeCollector("edge-1", filelogPipeline()))
	row := only(t, Derive(Derivation{
		Topology: checkoutTopology(),
		Reading:  est,
		Now:      readAt.Add(time.Hour), // far past cadence × tolerance
	}))

	if row.Effective.Known {
		t.Fatalf("effective = %+v, want unknown: a reading past the staleness horizon never feeds a verdict", row.Effective)
	}
	if !strings.Contains(row.Effective.Cause, "stale") {
		t.Errorf("cause = %q, want the staleness demotion carried into the row", row.Effective.Cause)
	}
}

func TestAServiceGetsOneRowPerEnvironmentItsPathsEnter(t *testing.T) {
	topo := checkoutTopology()
	topo.Tiers["shop/edge-staging"] = renderer.Tier{
		Team: "shop", Name: "edge-staging", Environment: "staging",
		Selector: map[string]string{"telecraft.tier": "edge", "deployment.environment": "staging"},
	}
	svc := topo.Services["shop/checkout"]
	svc.Paths = append(svc.Paths, renderer.Path{Through: []string{"shop/edge-staging"}})
	topo.Services["shop/checkout"] = svc

	est := Derive(Derivation{Topology: topo, Reading: reading(edgeCollector("edge-1", filelogPipeline())), Now: readAt})

	if got := est.Environments(); len(got) != 2 || got[0] != "production" || got[1] != "staging" {
		t.Fatalf("environments = %v, want one row per Environment the Paths enter", got)
	}
}

func TestAnAuthoredRowOverridesTheDerivedOne(t *testing.T) {
	authored := Estate{Rows: []EstateRow{{
		Row:       Row{Service: "shop/checkout", Environment: "production"},
		Effective: Effective{Known: true, Pipelines: []Pipeline{{Name: "logs", Receivers: []string{"journald"}}}},
		Reason:    "the edge collectors are git-delivered and report to nothing",
	}}}

	row := only(t, derive(t, reading(edgeCollector("edge-1", filelogPipeline())), authored))

	if got := row.Effective.Pipelines[0].Receivers[0]; got != "journald" {
		t.Errorf("receivers = %v, want the authored row: an operator's explicit statement is never discarded silently", got)
	}
	if !row.Overridden {
		t.Error("the row is not marked overridden, so a report built on it could not say so")
	}
	if row.Reason == "" {
		t.Error("the stated reason is dropped, and an override with no visible reason is the thing this replaces")
	}
}

func TestAnAuthoredRowTheTopologyDerivesNothingForIsStillJudged(t *testing.T) {
	authored := Estate{Rows: []EstateRow{{
		Row:       Row{Service: "legacy/billing", Environment: "production"},
		Effective: Effective{Known: true},
	}}}

	est := derive(t, reading(edgeCollector("edge-1", filelogPipeline())), authored)

	if len(est.Rows) != 2 {
		t.Fatalf("rows = %+v, want the derived row and the authored one: dropping a row stops governing a Service", est.Rows)
	}
	if est.Rows[0].Service != "legacy/billing" {
		t.Errorf("rows are not in Service order: %+v", est.Rows)
	}
}

func TestADerivedRowKeepsTheGracePeriodInputs(t *testing.T) {
	onboarded := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	authored := Estate{
		Grace: GracePolicy{{Class: "C1", Window: 168 * time.Hour}},
		Rows: []EstateRow{{
			Row:       Row{Service: "shop/checkout", Environment: "staging"},
			Class:     "C1",
			Onboarded: onboarded,
		}},
	}

	est := derive(t, reading(edgeCollector("edge-1", filelogPipeline())), authored)

	var derived EstateRow
	for _, r := range est.Rows {
		if r.Environment == "production" {
			derived = r
		}
	}
	if !derived.Onboarded.Equal(onboarded) {
		t.Errorf("onboarded = %v, want the authored date: a Service keeps its Grace Period without authoring its pipelines", derived.Onboarded)
	}
	if len(est.Grace) != 1 {
		t.Errorf("grace = %+v, want the authored table carried through: no topology holds it", est.Grace)
	}
}

func TestAnUnknownDerivedRowIsNeverNotConfigured(t *testing.T) {
	row := only(t, derive(t, reading(), Estate{}))

	v := Evaluate(row.Row, lib(req()), Evidence{Effective: row.Effective}, evalAt)
	if len(v.Findings) != 1 {
		t.Fatalf("findings = %+v, want one", v.Findings)
	}
	if got := v.Findings[0].Outcome; got != Unknown {
		t.Errorf("outcome = %s, want unknown: a row nothing could be read for is not an accusation against its owner", got)
	}
}
