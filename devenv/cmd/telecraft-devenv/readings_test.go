package main

import (
	"context"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/console"
	seam "github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// The composer turning two live seams into a readings file. Every assertion
// below is about it carrying what a seam returned, and — at least as often
// — about it refusing to carry what no seam said.

var base = time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)

func TestComposeCarriesTheCollectorEstate(t *testing.T) {
	c := &composer{
		Collectors: fakeEstate{collectors: []seam.Collector{{
			Identity: map[string]string{
				"telecraft.tier":      "gateway",
				"service.instance.id": "gateway-1",
				"service.version":     "0.159.0",
			},
			Effective: seam.Effective{Known: true, AsOf: base.Add(-2 * time.Second)},
		}}, asOf: base},
		Telemetry: fakeTelemetry{},
		Now:       func() time.Time { return base },
	}

	got := c.compose(context.Background())

	if len(got.Collectors) != 1 {
		t.Fatalf("got %d collectors, want 1", len(got.Collectors))
	}
	col := got.Collectors[0]
	if col.ID != "gateway-1" {
		t.Errorf("id %q — the instance id is the readable name", col.ID)
	}
	if col.Attributes["telecraft.tier"] != "gateway" {
		t.Error("the reported identity did not survive; nothing could match this collector")
	}
	if col.Version != "0.159.0" {
		t.Errorf("version %q — the collector reported one", col.Version)
	}
	if col.State != "reporting" || col.Delivery != "served" {
		t.Errorf("state %q delivery %q — every collector here is on a live connection to the platform's own server", col.State, col.Delivery)
	}
	if !col.LastSeen.Equal(base.Add(-2 * time.Second)) {
		t.Errorf("last seen %v — the reading's own instant, not the composer's", col.LastSeen)
	}
}

func TestComposeNamesAnIdentityWithNoInstanceIDByItsWholeIdentity(t *testing.T) {
	identity := map[string]string{"telecraft.tier": "gateway", "deployment.environment": "production"}
	c := &composer{
		Collectors: fakeEstate{collectors: []seam.Collector{{Identity: identity}}, asOf: base},
		Telemetry:  fakeTelemetry{},
		Now:        func() time.Time { return base },
	}

	got := c.compose(context.Background()).Collectors[0]

	// A truncation would read as a name and be ambiguous; the whole set
	// never is.
	if got.ID != seam.Fingerprint(identity) {
		t.Errorf("id %q — a collector with no instance id is named by its identity", got.ID)
	}
}

func TestComposeCarriesArrivalsPerRow(t *testing.T) {
	c := &composer{
		Collectors: fakeEstate{asOf: base},
		Telemetry: fakeTelemetry{observed: map[string]telemetry.Observed{
			"shop/checkout|production": {Signals: map[requirements.SignalKind]telemetry.SignalObservation{
				requirements.Traces: {Known: true, Present: true, Volume: 412,
					AttributeCoverage: map[string]float64{"service.namespace": 1}},
			}},
		}},
		Rows: []row{{Service: "shop/checkout", Environment: "production"}},
		Now:  func() time.Time { return base },
	}

	got := c.compose(context.Background())

	if len(got.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(got.Rows))
	}
	traces := got.Rows[0].Signals["traces"]
	if traces.Known == nil || !*traces.Known || !traces.Present || traces.Volume != 412 {
		t.Errorf("the arrival reading did not survive: %+v", traces)
	}
	if traces.AttributeCoverage["service.namespace"] != 1 {
		t.Error("the coverage measurement did not survive, so a coverage assertion would be judged against a blank")
	}
	if !traces.Since.IsZero() {
		t.Error("a signal that is arriving was given a silence start")
	}
}

func TestComposeHoldsTheSilenceStartStillWhileTheGapPersists(t *testing.T) {
	tel := fakeTelemetry{observed: map[string]telemetry.Observed{
		"shop/checkout|production": {Signals: map[requirements.SignalKind]telemetry.SignalObservation{
			requirements.Traces: {Known: true, Present: false},
		}},
	}}
	now := base
	c := &composer{
		Collectors: fakeEstate{asOf: base},
		Telemetry:  tel,
		Rows:       []row{{Service: "shop/checkout", Environment: "production"}},
		Now:        func() time.Time { return now },
	}

	first := c.compose(context.Background()).Rows[0].Signals["traces"].Since
	now = base.Add(10 * time.Minute)
	second := c.compose(context.Background()).Rows[0].Signals["traces"].Since

	if first.IsZero() {
		t.Fatal("an observed gap carries no silence start, so no judgement can tell a new gap from an old one")
	}
	// A snapshot is one instant and no judgement raises a finding from one
	// instant (ADR-0035 §3). Restamping the start every refresh would leave
	// every gap permanently dampened and no scenario would ever surface.
	if !second.Equal(first) {
		t.Errorf("the silence start moved from %v to %v — the gap would never age", first, second)
	}
}

func TestComposeClearsTheSilenceWhenTheSignalReturns(t *testing.T) {
	tel := fakeTelemetry{observed: map[string]telemetry.Observed{
		"shop/checkout|production": {Signals: map[requirements.SignalKind]telemetry.SignalObservation{
			requirements.Traces: {Known: true, Present: false},
		}},
	}}
	c := &composer{
		Collectors: fakeEstate{asOf: base},
		Telemetry:  tel,
		Rows:       []row{{Service: "shop/checkout", Environment: "production"}},
		Now:        func() time.Time { return base },
	}
	c.compose(context.Background())

	tel.observed["shop/checkout|production"] = telemetry.Observed{
		Signals: map[requirements.SignalKind]telemetry.SignalObservation{
			requirements.Traces: {Known: true, Present: true, Volume: 1},
		}}
	c.Telemetry = tel
	got := c.compose(context.Background()).Rows[0].Signals["traces"]

	if !got.Since.IsZero() {
		t.Error("a signal that came back still carries a silence start")
	}
}

func TestComposeGivesAnUnknownReadingNoSilence(t *testing.T) {
	c := &composer{
		Collectors: fakeEstate{asOf: base},
		Telemetry: fakeTelemetry{observed: map[string]telemetry.Observed{
			"shop/checkout|production": {Signals: map[requirements.SignalKind]telemetry.SignalObservation{
				requirements.Traces: {Known: false, Cause: "backend unreachable"},
			}},
		}},
		Rows: []row{{Service: "shop/checkout", Environment: "production"}},
		Now:  func() time.Time { return base },
	}

	got := c.compose(context.Background()).Rows[0].Signals["traces"]

	if got.Known == nil || *got.Known {
		t.Fatal("an unknown reading was carried as known")
	}
	if got.Cause == "" {
		t.Error("not knowing was carried without its cause (ADR-0008)")
	}
	// Not knowing is a normal state; it is never an observed gap, and
	// dating it would turn a blind spot into a finding.
	if !got.Since.IsZero() {
		t.Error("a reading the backend could not make was dated as a silence")
	}
}

func TestComposeCarriesSelfTelemetryComponentsVerbatim(t *testing.T) {
	c := &composer{
		Collectors: fakeEstate{asOf: base},
		Telemetry: fakeTelemetry{self: map[string]telemetry.SelfObserved{
			"platform/gateway": {Signals: map[requirements.SignalKind]telemetry.SelfSignal{
				requirements.Metrics: {Known: true, Present: true, Volume: 90, Components: []telemetry.ComponentTelemetry{
					{Attributes: map[string]string{"exporter": "otlp_http/platform.backend-exporter"}, Records: 60},
					{Attributes: map[string]string{"receiver": "otlp/otlp-in"}, Records: 30},
					{Records: 12}, // collector-level telemetry carries no component identity
				}},
			}},
		}},
		Tiers: []string{"platform/gateway"},
		Now:   func() time.Time { return base },
	}

	got := c.compose(context.Background())

	if len(got.Tiers) != 1 {
		t.Fatalf("got %d Tier readings, want 1", len(got.Tiers))
	}
	if len(got.Tiers[0].Components) != 2 {
		t.Fatalf("got %d component identities, want the 2 that carry one", len(got.Tiers[0].Components))
	}
	if got.Tiers[0].Components[0]["exporter"] != "otlp_http/platform.backend-exporter" {
		t.Errorf("component identities are not in stable order: %v", got.Tiers[0].Components)
	}
}

func TestAttributeNamesIsTheUnionTheLibraryAsksAbout(t *testing.T) {
	lib := requirements.Library{Requirements: map[string]requirements.Requirement{
		"a": {Signal: &requirements.SignalAssertion{RequiredAttributes: []string{"deployment.environment.name", "service.namespace"}}},
		"b": {Signal: &requirements.SignalAssertion{RequiredAttributes: []string{"service.namespace"}}},
		"c": {Config: &requirements.ConfigAssertion{HasReceiver: []string{"otlp"}}},
	}}

	got := attributeNames(lib)

	want := []string{"deployment.environment.name", "service.namespace"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// fakeEstate is the EstateProvider seam's reading, fabricated.
type fakeEstate struct {
	collectors []seam.Collector
	asOf       time.Time
}

func (f fakeEstate) Estate(context.Context) seam.Estate {
	return seam.Estate{AsOf: f.asOf, Collectors: f.collectors}
}

// fakeTelemetry answers the TelemetryProvider seam from a map. A row it was
// not given reads as an empty observation, which is what a backend holding
// nothing for that Service would say.
type fakeTelemetry struct {
	observed map[string]telemetry.Observed
	self     map[string]telemetry.SelfObserved
}

func (f fakeTelemetry) Name() string { return "fake" }

func (f fakeTelemetry) Observe(_ context.Context, s telemetry.Service, _ time.Duration, _ []string) telemetry.Observed {
	return f.observed[s.Name+"|"+s.Environment]
}

func (f fakeTelemetry) AttributeNames(context.Context, telemetry.Service, requirements.SignalKind, time.Duration) telemetry.AttributeNames {
	return telemetry.AttributeNames{}
}

func (f fakeTelemetry) ObserveSelf(_ context.Context, tier string, _ time.Duration) telemetry.SelfObserved {
	return f.self[tier]
}

func (f fakeTelemetry) Meter(context.Context, string, time.Duration) telemetry.Metered {
	return telemetry.Metered{}
}

var _ telemetry.Provider = fakeTelemetry{}

func TestObservePopulationsClocksATierBelowItsFloor(t *testing.T) {
	floor := 2
	c := &composer{Tiers: []string{"platform/gateway"}, Collectors: fakeEstate{asOf: base},
		Telemetry: fakeTelemetry{}, Now: func() time.Time { return base }}

	c.observePopulations([]console.CardFace{{
		Tier:       "platform/gateway",
		Population: console.Population{Matched: 1, Floor: &floor},
	}}, base)
	got := c.compose(context.Background()).Tiers[0]

	if got.ShortfallSince.IsZero() {
		t.Fatal("a Tier below its floor carries no shortfall start, so the finding can never age past its grace window")
	}
	if !got.ShortfallSince.Equal(base) {
		t.Errorf("shortfall dated %v, want %v", got.ShortfallSince, base)
	}
}

func TestObservePopulationsHoldsTheShortfallStartStill(t *testing.T) {
	floor := 2
	card := []console.CardFace{{Tier: "platform/gateway", Population: console.Population{Matched: 1, Floor: &floor}}}
	c := &composer{Tiers: []string{"platform/gateway"}, Collectors: fakeEstate{asOf: base},
		Telemetry: fakeTelemetry{}, Now: func() time.Time { return base }}

	c.observePopulations(card, base)
	c.observePopulations(card, base.Add(5*time.Minute))
	got := c.compose(context.Background()).Tiers[0]

	if !got.ShortfallSince.Equal(base) {
		t.Errorf("the shortfall start moved to %v — the gap would never age past its grace window", got.ShortfallSince)
	}
}

func TestObservePopulationsClearsWhenTheFloorIsMetAgain(t *testing.T) {
	floor := 2
	c := &composer{Tiers: []string{"platform/gateway"}, Collectors: fakeEstate{asOf: base},
		Telemetry: fakeTelemetry{}, Now: func() time.Time { return base }}

	c.observePopulations([]console.CardFace{{Tier: "platform/gateway",
		Population: console.Population{Matched: 1, Floor: &floor}}}, base)
	c.observePopulations([]console.CardFace{{Tier: "platform/gateway",
		Population: console.Population{Matched: 2, Floor: &floor}}}, base.Add(time.Minute))
	got := c.compose(context.Background()).Tiers[0]

	if !got.ShortfallSince.IsZero() {
		t.Error("a Tier back at its floor still carries a shortfall start")
	}
}

func TestObservePopulationsIgnoresATierWithNoFloor(t *testing.T) {
	c := &composer{Tiers: []string{"platform/gateway"}, Collectors: fakeEstate{asOf: base},
		Telemetry: fakeTelemetry{}, Now: func() time.Time { return base }}

	// With no floor there are no teeth (ADR-0035): dating a shortfall
	// against a floor nobody declared would invent the only input the
	// finding needs.
	c.observePopulations([]console.CardFace{{Tier: "platform/gateway",
		Population: console.Population{Matched: 0, Floor: nil}}}, base)
	got := c.compose(context.Background()).Tiers[0]

	if !got.ShortfallSince.IsZero() {
		t.Error("a Tier with no declared floor was given a shortfall start")
	}
}
