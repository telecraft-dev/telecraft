package expectation

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/inventory"
	"github.com/telecraft-dev/telecraft/internal/ownership"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

var t0 = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

const fixtureRow = "product/checkout"

// testConfig keeps the windows short and explicit so tests state their
// timing arithmetic plainly.
func testConfig() Config {
	return Config{
		Settle: SettleWindows{Arrival: time.Minute, Enrichment: time.Minute, SelfTelemetry: 10 * time.Second},
		Grace:  5 * time.Minute,
		Population: inventory.Config{
			Grace: 5 * time.Minute,
		},
	}
}

// settledRow is evidence whose artefact went APPLIED long ago — every
// settle window has passed.
func settledRow(obs telemetry.Observed) RowEvidence {
	return RowEvidence{AppliedAt: t0.Add(-time.Hour), Observed: obs}
}

// tracesLibrary demands traces presence in every environment: the traces
// arrival claim is Requirement-backed, everything else is unbacked.
func tracesLibrary() requirements.Library {
	return requirements.Library{Requirements: map[string]requirements.Requirement{
		"traces-present": {
			ID: "traces-present", Version: 1, Owner: "sre",
			Signal: &requirements.SignalAssertion{Kind: requirements.Traces, Present: true,
				Window: requirements.Duration(DefaultObservationWindow)},
			Remediation: "instrument",
		},
	}}
}

func observed(signals map[requirements.SignalKind]telemetry.SignalObservation) telemetry.Observed {
	return telemetry.Observed{AsOf: t0, Window: DefaultObservationWindow, Signals: signals}
}

// Matching arrivals produce expectation-green (issue #34 AC): everything
// the config states arrives, so every data claim is green and no finding
// raises.
func TestRowGreenWhenArrivalsMatch(t *testing.T) {
	set := Derive(fixtureSource(t))
	obs := observed(map[requirements.SignalKind]telemetry.SignalObservation{
		requirements.Traces: {Known: true, Present: true, Volume: 100,
			AttributeCoverage: map[string]float64{"deployment.owner": 1, "telecraft.zone": 1}},
		requirements.Logs: {Known: true, Present: true, Volume: 100,
			AttributeCoverage: map[string]float64{"log.source": 0.8}},
	})

	res := EvaluateRow(set, fixtureRow, "production", tracesLibrary(), settledRow(obs),
		inventory.NewDamper(), testConfig(), t0)

	if len(res.Claims) == 0 {
		t.Fatal("no claims judged for the production row")
	}
	for _, c := range res.Claims {
		if c.Status != StatusGreen {
			t.Errorf("%s: status %s (%s), want green — the config worked", c.Claim.Key(), c.Status, c.Detail)
		}
	}
	if len(res.Findings) != 0 {
		t.Errorf("green row raised findings: %+v", res.Findings)
	}
}

// Missing expected telemetry produces an expectation finding distinct
// from delivery and conformance findings (issue #34 AC): the backed
// traces claim goes red with no finding of its own — its red is the
// Observed leg the cross already owns (ADR-0038 §5a) — while the
// unbacked logs claim raises the Service-attached, advisory-grade
// expectation finding with the fix-or-delete-the-lane fork (§5b).
func TestRowMissingTelemetryRaisesExpectationFinding(t *testing.T) {
	set := Derive(fixtureSource(t))
	damper := inventory.NewDamper()
	cfg := testConfig()
	obs := observed(map[requirements.SignalKind]telemetry.SignalObservation{
		requirements.Traces: {Known: true, Present: false},
		requirements.Logs:   {Known: true, Present: false},
	})
	lib := tracesLibrary()

	// First observation starts the shortfall clock: persistence-dampened,
	// nothing is red from a single instant (ADR-0035 §3).
	first := EvaluateRow(set, fixtureRow, "production", lib, settledRow(obs), damper, cfg, t0)
	for _, c := range first.Claims {
		if c.Claim.Kind == Arrival && c.Status != StatusPending {
			t.Errorf("first sight of the shortfall judged %s %s, want pending", c.Claim.Key(), c.Status)
		}
	}
	if len(first.Findings) != 0 {
		t.Errorf("findings raised inside the grace window: %+v", first.Findings)
	}

	later := t0.Add(cfg.Grace + time.Minute)
	res := EvaluateRow(set, fixtureRow, "production", lib, settledRow(obs), damper, cfg, later)

	byKey := map[string]ClaimResult{}
	for _, c := range res.Claims {
		byKey[c.Claim.Key()] = c
	}

	traces := byKey["arrival|product/checkout|production|traces"]
	if traces.Status != StatusRed || !traces.Backed {
		t.Errorf("backed traces claim = %s backed=%v, want red and backed", traces.Status, traces.Backed)
	}
	logs := byKey["arrival|product/checkout|production|logs"]
	if logs.Status != StatusRed || logs.Backed {
		t.Errorf("unbacked logs claim = %s backed=%v, want red and unbacked", logs.Status, logs.Backed)
	}

	if len(res.Findings) != 1 {
		t.Fatalf("findings = %+v, want exactly the one unbacked-claim advisory — the backed red belongs to the cross", res.Findings)
	}
	f := res.Findings[0]
	if f.Kind != ownership.Expectation {
		t.Errorf("finding kind = %q, want expectation — its own kind, distinct from delivery and service_conformance (ADR-0038 §5)", f.Kind)
	}
	if !f.Kind.Valid() {
		t.Error("the expectation finding kind is not valid in the roll-up — it must join ratio-plus-worst (ADR-0017)")
	}
	if f.Grade != ownership.Advisory {
		t.Errorf("unbacked data claim graded %q, want advisory — no human demanded the signal, so it cannot fail compliance", f.Grade)
	}
	if f.Subject.Kind != ownership.KindService || f.Subject.ID != fixtureRow {
		t.Errorf("finding routed to %+v, want the Service", f.Subject)
	}
	if !strings.Contains(f.Detail, "delete the dead lane") {
		t.Errorf("remediation is not honest about the fork: %q", f.Detail)
	}
}

// Inside the settle window after APPLIED, claims read neutral-pending —
// never red, never green (ADR-0038 §4b).
func TestRowSettleWindowReadsPending(t *testing.T) {
	set := Derive(fixtureSource(t))
	obs := observed(map[requirements.SignalKind]telemetry.SignalObservation{
		requirements.Traces: {Known: true, Present: true},
		requirements.Logs:   {Known: true, Present: true},
	})
	ev := RowEvidence{AppliedAt: t0.Add(-30 * time.Second), Observed: obs}

	res := EvaluateRow(set, fixtureRow, "production", tracesLibrary(), ev,
		inventory.NewDamper(), testConfig(), t0)
	for _, c := range res.Claims {
		if c.Status != StatusPending {
			t.Errorf("%s judged %s inside the settle window, want pending", c.Claim.Key(), c.Status)
		}
	}
}

// Known false readings yield unknown, never red — however long they
// persist (ADR-0008, ADR-0038 §4d).
func TestRowUnknownNeverRed(t *testing.T) {
	set := Derive(fixtureSource(t))
	damper := inventory.NewDamper()
	cfg := testConfig()
	obs := observed(map[requirements.SignalKind]telemetry.SignalObservation{
		requirements.Traces: {Known: false, Cause: "backend unreachable"},
		requirements.Logs:   {Known: false, Cause: "backend unreachable"},
	})

	EvaluateRow(set, fixtureRow, "production", tracesLibrary(), settledRow(obs), damper, cfg, t0)
	res := EvaluateRow(set, fixtureRow, "production", tracesLibrary(), settledRow(obs), damper, cfg,
		t0.Add(cfg.Grace+time.Hour))

	for _, c := range res.Claims {
		if c.Status != StatusUnknown {
			t.Errorf("%s judged %s on a Known false reading, want unknown", c.Claim.Key(), c.Status)
		}
	}
	if len(res.Findings) != 0 {
		t.Errorf("unknown evidence raised findings: %+v", res.Findings)
	}
}

// Enrichment judgement: an unmeasured attribute is unknown, a covered
// one green, an absent one red after dampening — with the arrival claim
// green throughout, because the signal itself lands.
func TestRowEnrichmentJudgement(t *testing.T) {
	set := Derive(fixtureSource(t))
	damper := inventory.NewDamper()
	cfg := testConfig()

	key := "enrichment|product/checkout|production|logs|log.source"
	find := func(res RowResult) ClaimResult {
		for _, c := range res.Claims {
			if c.Claim.Key() == key {
				return c
			}
		}
		t.Fatalf("claim %s not judged", key)
		return ClaimResult{}
	}

	unmeasured := observed(map[requirements.SignalKind]telemetry.SignalObservation{
		requirements.Logs: {Known: true, Present: true},
	})
	res := EvaluateRow(set, fixtureRow, "production", tracesLibrary(), settledRow(unmeasured), damper, cfg, t0)
	if c := find(res); c.Status != StatusUnknown {
		t.Errorf("unmeasured attribute judged %s, want unknown (%s)", c.Status, c.Detail)
	}

	absent := observed(map[requirements.SignalKind]telemetry.SignalObservation{
		requirements.Logs: {Known: true, Present: true, AttributeCoverage: map[string]float64{"log.source": 0}},
	})
	EvaluateRow(set, fixtureRow, "production", tracesLibrary(), settledRow(absent), damper, cfg, t0)
	res = EvaluateRow(set, fixtureRow, "production", tracesLibrary(), settledRow(absent), damper, cfg,
		t0.Add(cfg.Grace+time.Minute))
	c := find(res)
	if c.Status != StatusRed {
		t.Errorf("absent literal insertion judged %s, want red (%s)", c.Status, c.Detail)
	}
	var expectationFindings int
	for _, f := range res.Findings {
		if f.Kind == ownership.Expectation && strings.Contains(f.Detail, "log.source") {
			expectationFindings++
		}
	}
	if expectationFindings != 1 {
		t.Errorf("unbacked enrichment red raised %d expectation findings, want 1", expectationFindings)
	}
}

// --- Tier (pipeline claim) evaluation ---

// staticInventory is an InventoryProvider slice for the tests: the
// substrate's expected count, answered live (ADR-0035 §1).
type staticInventory struct{ instances int }

func (s staticInventory) Name() string { return "StaticInventory" }
func (s staticInventory) Declaration() inventory.Declaration {
	return inventory.Declaration{RefreshCadence: time.Minute}
}
func (s staticInventory) Expected(context.Context, map[string]string) inventory.Count {
	return inventory.Count{Known: true, AsOf: t0, Instances: s.instances}
}

// derivedFloor reads the provider's answer as floor resolution must see
// it — through ForEvaluation, so a stale count could never float a floor.
func derivedFloor(t *testing.T, p inventory.Provider, selector map[string]string) inventory.Count {
	t.Helper()
	return p.Expected(context.Background(), selector).ForEvaluation(p.Declaration(), t0)
}

func gatewaySelector(t *testing.T) map[string]string {
	t.Helper()
	return fixtureSource(t).Topology.Tiers["data-flow/gateway"].Selector
}

// healthySelf is a reading in which every fixture component emits: the
// legacy datapoint attributes on metrics, the scope attributes on logs,
// and memory_limiter as the identity-dropping singleton (R-4 §5.2).
func healthySelf() telemetry.SelfObserved {
	comp := func(attrs map[string]string) telemetry.ComponentTelemetry {
		return telemetry.ComponentTelemetry{Attributes: attrs, Records: 10}
	}
	return telemetry.SelfObserved{AsOf: t0, Window: time.Hour,
		Signals: map[requirements.SignalKind]telemetry.SelfSignal{
			requirements.Metrics: {Known: true, Present: true, Volume: 100,
				Commits: map[string]int64{fixtureSHA: 100},
				Components: []telemetry.ComponentTelemetry{
					comp(map[string]string{"receiver": "otlp/otlp-in"}),
					comp(map[string]string{"processor": "resource/stamp", "otel.signal": "traces"}),
					comp(map[string]string{"processor": "k8sattributes/k8s"}),
					comp(map[string]string{"exporter": "otlphttp/shipper", "data_type": "logs"}),
				}},
			requirements.Logs: {Known: true, Present: true, Volume: 40,
				Commits: map[string]int64{fixtureSHA: 40},
				Components: []telemetry.ComponentTelemetry{
					comp(map[string]string{"otelcol.component.kind": "processor", "otelcol.component.id": "transform/scrub", "otelcol.pipeline.id": "logs"}),
					// The singleton deliberately drops its id — an
					// expected shape, never a join failure (R-4 §5.2).
					comp(map[string]string{"otelcol.component.kind": "processor"}),
					// Synthetic graph nodes are tolerated and match
					// nothing (R-4 §5.4).
					comp(map[string]string{"otelcol.component.kind": "fanout", "otelcol.pipeline.id": "logs"}),
				}},
		}}
}

func healthyPopulation() inventory.Population {
	return inventory.Population{
		Tier:     "data-flow/gateway",
		Derived:  inventory.Count{Known: true, AsOf: t0, Instances: 2},
		Declared: 2,
		Seen:     2,
		EverSeen: true,
	}
}

// Every instantiated component emitting under R-4's join keys — either
// generation, expected shapes included — judges green with no finding.
func TestTierGreenWhenComponentsEmit(t *testing.T) {
	set := Derive(fixtureSource(t))
	ev := TierEvidence{RunningSHA: fixtureSHA, AppliedAt: t0.Add(-time.Hour),
		Self: healthySelf(), Population: healthyPopulation()}

	res, err := EvaluateTier(set, "data-flow/gateway", ev, inventory.NewDamper(), testConfig(), t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Claims) == 0 {
		t.Fatal("no pipeline claims judged")
	}
	for _, c := range res.Claims {
		if c.Status != StatusGreen {
			t.Errorf("%s judged %s (%s), want green", c.Claim.Key(), c.Status, c.Detail)
		}
	}
	if len(res.Findings) != 0 {
		t.Errorf("healthy tier raised findings: %+v", res.Findings)
	}
}

// A component silent past the settle window goes red once the shortfall
// persists, raising the Tier-attached, violation-grade expectation
// finding — "the config didn't work" in its sharpest form (ADR-0038 §5c).
func TestTierSilentComponentEscalates(t *testing.T) {
	set := Derive(fixtureSource(t))
	damper := inventory.NewDamper()
	cfg := testConfig()

	self := healthySelf()
	metrics := self.Signals[requirements.Metrics]
	metrics.Components = metrics.Components[:3] // drop the exporter's telemetry
	self.Signals[requirements.Metrics] = metrics

	ev := TierEvidence{RunningSHA: fixtureSHA, AppliedAt: t0.Add(-time.Hour),
		Self: self, Population: healthyPopulation()}

	first, err := EvaluateTier(set, "data-flow/gateway", ev, damper, cfg, t0)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range first.Claims {
		if c.Claim.Component == "otlphttp/shipper" && c.Status != StatusPending {
			t.Errorf("first sight of silence judged %s, want pending — persistence-dampened (ADR-0035 §3)", c.Status)
		}
	}

	res, err := EvaluateTier(set, "data-flow/gateway", ev, damper, cfg, t0.Add(cfg.Grace+time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range res.Claims {
		want := StatusGreen
		if c.Claim.Component == "otlphttp/shipper" {
			want = StatusRed
		}
		if c.Status != want {
			t.Errorf("%s judged %s, want %s (%s)", c.Claim.Key(), c.Status, want, c.Detail)
		}
	}

	if len(res.Findings) != 1 {
		t.Fatalf("findings = %+v, want exactly the silent exporter's violation", res.Findings)
	}
	f := res.Findings[0]
	if f.Kind != ownership.Expectation || f.Grade != ownership.Violation {
		t.Errorf("finding = kind %q grade %q, want expectation violation — pipeline claims are violation-capable after dampening", f.Kind, f.Grade)
	}
	if f.Subject.Kind != ownership.KindTier || f.Subject.ID != "data-flow/gateway" {
		t.Errorf("finding routed to %+v, want the Tier", f.Subject)
	}
}

// Claims are judged against the artefact the collector reports running —
// never head. Handing the engine head's claims with another commit's
// telemetry is a caller bug, refused outright (ADR-0038 §4a).
func TestTierRefusesSHAMismatch(t *testing.T) {
	set := Derive(fixtureSource(t))
	ev := TierEvidence{RunningSHA: "0000000000000000000000000000000000000000",
		Self: healthySelf(), Population: healthyPopulation()}
	if _, err := EvaluateTier(set, "data-flow/gateway", ev, inventory.NewDamper(), testConfig(), t0); err == nil {
		t.Fatal("a claim set derived at one SHA was judged against another commit's telemetry — the cascade ADR-0038 §4a forbids")
	}
}

// never_seen escalates through the floor from the InventoryProvider
// slice (issue #34 AC): the substrate expects 3, nothing has ever
// matched, the shortfall persisted — a violation-grade delivery finding.
// The pipeline claims read unknown, never red: nothing runs, so silence
// is the population's finding, and delivery failure structurally cannot
// cascade into expectation-red (ADR-0038 §4a).
func TestNeverSeenEscalatesThroughProviderFloor(t *testing.T) {
	set := Derive(fixtureSource(t))
	cfg := testConfig()

	pop := inventory.Population{
		Tier:           "data-flow/gateway",
		Derived:        derivedFloor(t, staticInventory{instances: 3}, gatewaySelector(t)),
		Seen:           0,
		EverSeen:       false,
		FirstWatched:   t0.Add(-time.Hour),
		ShortfallSince: t0.Add(-10 * time.Minute),
	}
	ev := TierEvidence{RunningSHA: "", Self: telemetry.SelfUnknown(t0, time.Hour, "no collectors"), Population: pop}

	res, err := EvaluateTier(set, "data-flow/gateway", ev, inventory.NewDamper(), cfg, t0)
	if err != nil {
		t.Fatal(err)
	}

	var neverSeen *inventory.Finding
	for i, f := range res.Population {
		if f.Class == inventory.NeverSeen {
			neverSeen = &res.Population[i]
		}
	}
	if neverSeen == nil || neverSeen.Grade != inventory.Violation {
		t.Fatalf("population findings = %+v, want never_seen escalated to violation through the derived floor (ADR-0035 §4)", res.Population)
	}
	if neverSeen.Floor.Source != inventory.FloorDerived || neverSeen.Floor.Min != 3 {
		t.Errorf("floor = %+v, want the InventoryProvider's derived 3", neverSeen.Floor)
	}

	var delivery int
	for _, f := range res.Findings {
		if f.Kind == ownership.Delivery && f.Grade == ownership.Violation {
			delivery++
		}
		if f.Kind == ownership.Expectation {
			t.Errorf("expectation finding raised while nothing runs: %+v — that red belongs to delivery", f)
		}
	}
	if delivery != 1 {
		t.Errorf("want the escalated never_seen as one delivery-kind violation, got findings %+v", res.Findings)
	}

	for _, c := range res.Claims {
		if c.Status != StatusUnknown {
			t.Errorf("%s judged %s with no collector matched, want unknown", c.Claim.Key(), c.Status)
		}
	}
}

// under_populated escalates the same way (issue #34 AC): collectors
// present but below the provider's floor, persisting — while the
// collectors that do run still have their pipeline claims judged.
func TestUnderPopulatedEscalatesThroughProviderFloor(t *testing.T) {
	set := Derive(fixtureSource(t))
	cfg := testConfig()

	pop := inventory.Population{
		Tier:           "data-flow/gateway",
		Derived:        derivedFloor(t, staticInventory{instances: 3}, gatewaySelector(t)),
		Seen:           1,
		EverSeen:       true,
		ShortfallSince: t0.Add(-10 * time.Minute),
	}
	ev := TierEvidence{RunningSHA: fixtureSHA, AppliedAt: t0.Add(-time.Hour),
		Self: healthySelf(), Population: pop}

	res, err := EvaluateTier(set, "data-flow/gateway", ev, inventory.NewDamper(), cfg, t0)
	if err != nil {
		t.Fatal(err)
	}

	var under *inventory.Finding
	for i, f := range res.Population {
		if f.Class == inventory.UnderPopulated {
			under = &res.Population[i]
		}
	}
	if under == nil || under.Grade != inventory.Violation {
		t.Fatalf("population findings = %+v, want under_populated escalated to violation (ADR-0035 §5)", res.Population)
	}
	if under.Floor.Source != inventory.FloorDerived || under.Floor.Min != 3 {
		t.Errorf("floor = %+v, want the InventoryProvider's derived 3", under.Floor)
	}

	for _, c := range res.Claims {
		if c.Status != StatusGreen {
			t.Errorf("%s judged %s, want green — the collector that does run emits, and its claims are judged on their own evidence", c.Claim.Key(), c.Status)
		}
	}
}
