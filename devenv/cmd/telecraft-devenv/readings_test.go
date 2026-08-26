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
// below is about it carrying what a seam returned, and, at least as often,
// about it refusing to carry what no seam said.

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
		Delivery:  fakeDelivery("served"),
		Telemetry: fakeTelemetry{},
		Now:       func() time.Time { return base },
	}

	got := c.compose(context.Background())

	if len(got.Collectors) != 1 {
		t.Fatalf("got %d collectors, want 1", len(got.Collectors))
	}
	col := got.Collectors[0]
	if col.ID != "gateway-1" {
		t.Errorf("id %q: the instance id is the readable name", col.ID)
	}
	if col.Attributes["telecraft.tier"] != "gateway" {
		t.Error("the reported identity did not survive; nothing could match this collector")
	}
	if col.Version != "0.159.0" {
		t.Errorf("version %q: the collector reported one", col.Version)
	}
	if col.State != "reporting" {
		t.Errorf("state %q: every collector in this reading is on a live connection to the platform's own server", col.State)
	}
	if !col.LastSeen.Equal(base.Add(-2 * time.Second)) {
		t.Errorf("last seen %v: the reading's own instant, not the composer's", col.LastSeen)
	}
}

func TestComposeCarriesTheDeliveryPathTheWireShowed(t *testing.T) {
	c := &composer{
		Collectors: fakeEstate{collectors: []seam.Collector{{
			Identity: map[string]string{"telecraft.tier": "appliance", "service.instance.id": "appliance-1"},
		}}, asOf: base},
		// A collector that reports here and takes nothing this server
		// sends. Carrying it as served would put the whole estate on one
		// delivery path by assertion, which is what the topology's
		// delivery split has been until now (REQ-041).
		Delivery:  fakeDelivery("git"),
		Telemetry: fakeTelemetry{},
		Now:       func() time.Time { return base },
	}

	got := c.compose(context.Background()).Collectors[0]

	if got.Delivery != "git" {
		t.Errorf("delivery %q: the reading did not carry the path the wire showed", got.Delivery)
	}
}

func TestComposeNamesAnIdentityWithNoInstanceIDByItsWholeIdentity(t *testing.T) {
	identity := map[string]string{"telecraft.tier": "gateway", "deployment.environment": "production"}
	c := &composer{
		Collectors: fakeEstate{collectors: []seam.Collector{{Identity: identity}}, asOf: base},
		Delivery:   fakeDelivery("served"),
		Telemetry:  fakeTelemetry{},
		Now:        func() time.Time { return base },
	}

	got := c.compose(context.Background()).Collectors[0]

	// A truncation would read as a name and be ambiguous; the whole set
	// never is.
	if got.ID != seam.Fingerprint(identity) {
		t.Errorf("id %q: a collector with no instance id is named by its identity", got.ID)
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
		t.Errorf("the silence start moved from %v to %v: the gap would never age", first, second)
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
		t.Error("not knowing was carried without its cause")
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

// fakeDelivery answers the delivery reading with one fixed path, so a test
// about anything else does not have to fabricate a wire.
type fakeDelivery string

func (f fakeDelivery) Path(map[string]string) string { return string(f) }

// fakeTelemetry answers the TelemetryProvider seam from a map. A row it was
// not given reads as an empty observation, which is what a backend holding
// nothing for that Service would say.
type fakeTelemetry struct {
	observed map[string]telemetry.Observed
	self     map[string]telemetry.SelfObserved

	// names answers the attribute-name primitive per signal. A signal it
	// holds nothing for reads Known false with the cause said out loud,
	// because an empty name set is what a conformance check reads as
	// "these attributes are not in use".
	names map[requirements.SignalKind]telemetry.AttributeNames

	// live answers the live-check primitive per Service, keyed like
	// observed. A row it holds nothing for reads Known false rather than
	// an empty record set, because an empty set beside a fed liveness leg
	// is exactly what the live arm reads as clean, and this fake has
	// looked at nothing.
	live map[string]telemetry.LiveCheckFindings
}

func (f fakeTelemetry) Name() string { return "fake" }

func (f fakeTelemetry) Observe(_ context.Context, s telemetry.Service, _ time.Duration, _ []string) telemetry.Observed {
	return f.observed[s.Name+"|"+s.Environment]
}

func (f fakeTelemetry) AttributeNames(_ context.Context, _ telemetry.Service, kind requirements.SignalKind, window time.Duration) telemetry.AttributeNames {
	if reading, held := f.names[kind]; held {
		return reading
	}
	return telemetry.AttributeNames{
		Known:  false,
		Cause:  "the fake telemetry reading declares no attribute names",
		AsOf:   base,
		Window: window,
	}
}

// The two ADR-0034 §4 primitives the fake holds nothing for. It answers
// Known false with the cause said out loud rather than an empty set,
// because an empty value set and an empty group set are exactly what a
// conformance check reads as "clean", and this fake has looked at nothing.
func (f fakeTelemetry) DistinctValues(_ context.Context, _ telemetry.Service, _ requirements.SignalKind, attribute string, window time.Duration) telemetry.DistinctValues {
	return telemetry.DistinctValuesUnknown(base, window, attribute, "the fake telemetry reading declares no value sets")
}

func (f fakeTelemetry) GroupNames(_ context.Context, _ telemetry.Service, kind requirements.SignalKind, window time.Duration) telemetry.GroupNames {
	return telemetry.GroupNamesUnknown(base, window, kind, "the fake telemetry reading declares no group names")
}

func (f fakeTelemetry) LiveCheckFindings(_ context.Context, s telemetry.Service, window time.Duration) telemetry.LiveCheckFindings {
	if reading, held := f.live[s.Name+"|"+s.Environment]; held {
		return reading
	}
	return telemetry.LiveCheckUnknown(base, window, "the fake telemetry reading declares no live-check findings")
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
		t.Errorf("the shortfall start moved to %v: the gap would never age past its grace window", got.ShortfallSince)
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

// The attribute-name reading is taken for the signals a schema-conformance
// requirement is judged on, and carried into the readings file so the
// snapshot judges the same reading the backend gave (ADR-0034 §4).
func TestComposeCarriesTheAttributeNameReading(t *testing.T) {
	c := &composer{
		Collectors:    fakeEstate{asOf: base},
		Delivery:      fakeDelivery("served"),
		Rows:          []row{{Service: "checkout", Environment: "production"}},
		SchemaSignals: []requirements.SignalKind{requirements.Traces},
		Telemetry: fakeTelemetry{
			observed: map[string]telemetry.Observed{
				"checkout|production": {Signals: map[requirements.SignalKind]telemetry.SignalObservation{
					requirements.Traces: {Known: true, Present: true, Volume: 12},
					requirements.Logs:   {Known: true, Present: true, Volume: 90},
				}},
			},
			names: map[requirements.SignalKind]telemetry.AttributeNames{
				requirements.Traces: {
					Known: true, AsOf: base, Window: time.Hour,
					Names:          []string{"db.namespace", "db.system.name"},
					Truncated:      true,
					SampledRecords: 200,
					TotalRecords:   4096,
				},
			},
		},
		Window: time.Hour,
		Now:    func() time.Time { return base },
	}

	got := c.compose(context.Background())

	if len(got.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(got.Rows))
	}
	traces := got.Rows[0].Signals["traces"]
	if traces.AttributeNames == nil {
		t.Fatal("the traces reading carries no attribute names, so no schema requirement can be judged from this file")
	}
	if len(traces.AttributeNames.Names) != 2 || traces.AttributeNames.Names[0] != "db.namespace" {
		t.Errorf("names = %v, want what the seam returned", traces.AttributeNames.Names)
	}
	// Truncation travels or the file lies: a sampled reading played back as
	// a complete one turns an unsampled attribute into a missing one.
	if !traces.AttributeNames.Truncated ||
		traces.AttributeNames.SampledRecords != 200 || traces.AttributeNames.TotalRecords != 4096 {
		t.Errorf("truncation not carried: %+v", traces.AttributeNames)
	}
	// Logs were not asked about, and a reading nobody took is not declared:
	// the console plays an undeclared reading back as Known false with a
	// cause, which is the honest answer.
	if got.Rows[0].Signals["logs"].AttributeNames != nil {
		t.Error("a signal no schema requirement covers was declared anyway")
	}
}

// A reading the backend could not give is left undeclared rather than
// written as an empty name set: an empty set is what a schema verdict reads
// as attributes nobody sets.
func TestComposeDeclaresNoNamesWhenTheBackendCannotSee(t *testing.T) {
	c := &composer{
		Collectors:    fakeEstate{asOf: base},
		Delivery:      fakeDelivery("served"),
		Rows:          []row{{Service: "checkout", Environment: "production"}},
		SchemaSignals: []requirements.SignalKind{requirements.Traces},
		Telemetry: fakeTelemetry{observed: map[string]telemetry.Observed{
			"checkout|production": {Signals: map[requirements.SignalKind]telemetry.SignalObservation{
				requirements.Traces: {Known: true, Present: true, Volume: 12},
			}},
		}},
		Window: time.Hour,
		Now:    func() time.Time { return base },
	}

	got := c.compose(context.Background())

	if got.Rows[0].Signals["traces"].AttributeNames != nil {
		t.Error("a reading the backend could not give was written as one it could")
	}
}

// Which signals the reading is taken for comes from the library: a library
// referencing no Schema Registry buys no aggregations.
func TestSchemaSignalsAreTheSignalsTheLibraryJudges(t *testing.T) {
	lib := requirements.Library{Requirements: map[string]requirements.Requirement{
		"a": {ID: "a", Schema: &requirements.SchemaAssertion{
			Signals: []requirements.SignalKind{requirements.Traces},
		}},
		"b": {ID: "b", Schema: &requirements.SchemaAssertion{
			Signals: []requirements.SignalKind{requirements.Logs, requirements.Traces},
		}},
		"c": {ID: "c", Signal: &requirements.SignalAssertion{Kind: requirements.Metrics}},
	}}

	got := schemaSignals(lib)

	want := []requirements.SignalKind{requirements.Logs, requirements.Traces}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v in the seam's stable order", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v in the seam's stable order", got, want)
		}
	}

	if signals := schemaSignals(requirements.Library{Requirements: map[string]requirements.Requirement{
		"c": {ID: "c", Signal: &requirements.SignalAssertion{Kind: requirements.Metrics}},
	}}); len(signals) != 0 {
		t.Errorf("a library referencing no registry asks for %v", signals)
	}
}
