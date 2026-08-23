package card

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/inventory"
	"github.com/telecraft-dev/telecraft/internal/metering"
	"github.com/telecraft-dev/telecraft/internal/ownership"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

var update = flag.Bool("update", false, "rewrite the shared contract fixture")

var readAt = time.Date(2026, 8, 18, 11, 59, 0, 0, time.UTC)
var derivedAt = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

// ADR-0041 §2: the honest neutrals are each distinct, and each of them
// beats a verdict: every one of them means the evidence a verdict would
// rest on is missing.
func TestNeutralStatesTakePrecedenceOverFindings(t *testing.T) {
	red := []ownership.Finding{{Kind: ownership.Delivery, Grade: ownership.Violation, Detail: "nothing applied"}}

	for _, tc := range []struct {
		name  string
		input BandInput
		want  BandState
	}{
		{"not applicable", BandInput{NotApplicable: true, Known: true, Findings: red}, StateNotApplicable},
		{"stale demoted", BandInput{StaleDemoted: true, Known: true, Findings: red}, StateStaleDemoted},
		{"pending settle", BandInput{PendingSettle: true, Known: true, Findings: red}, StatePendingSettle},
		{"unknown", BandInput{Known: false, Findings: red}, StateUnknown},
		{"finding", BandInput{Known: true, Findings: red}, StateFinding},
		{"ok", BandInput{Known: true}, StateOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.input.band()
			if got.State != tc.want {
				t.Errorf("state = %q, want %q", got.State, tc.want)
			}
			if tc.want != StateFinding && got.WorstSeverity != SeverityNone {
				t.Errorf("a %s band carries severity %q: a neutral is not a verdict", tc.want, got.WorstSeverity)
			}
		})
	}
}

func TestWorstSeverityAndLabelComeFromTheFindings(t *testing.T) {
	band := BandInput{Known: true, Findings: []ownership.Finding{
		{Kind: ownership.Expectation, Grade: ownership.Advisory, Detail: "unbacked arrival claim on the logs lane"},
		{Kind: ownership.Delivery, Grade: ownership.Violation, Detail: "the artefact never applied"},
	}}.band()

	if band.WorstSeverity != SeverityViolation {
		t.Errorf("worst severity = %q, want violation", band.WorstSeverity)
	}
	if band.WorstFinding != "the artefact never applied" {
		t.Errorf("worst finding = %q, want the violation's detail", band.WorstFinding)
	}
}

// P4's verdict, held structurally: hue appears nowhere in the contract,
// so a renderer cannot make colour load-bearing by reading a field. Band
// position and state are what distinguish the three reds.
func TestHueIsUnderivableFromTheContract(t *testing.T) {
	face := Assemble(richInput())
	drawer, err := NewDrawer("data-flow/gateway", []Finding{violation("v"), advisory("a")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(struct {
		Face   Face   `json:"face"`
		Drawer Drawer `json:"drawer"`
	}{face, drawer})
	if err != nil {
		t.Fatal(err)
	}

	// Whole words: "required" is a reading, not a colour.
	hue := regexp.MustCompile(`(?i)\b(colours?|colors?|hue|red|amber|green|rgb|hsl)\b|#[0-9a-f]{3,8}\b`)
	if found := hue.FindAllString(string(payload), -1); len(found) > 0 {
		t.Errorf("the payload carries %v: hue must not be derivable from the contract, which is how mono-red stays structural rather than conventional", found)
	}
}

func TestTheThreeBandsKeepAFixedOrder(t *testing.T) {
	want := []BandName{Delivery, Expectation, Conformance}
	got := BandOrder()
	if len(got) != len(want) {
		t.Fatalf("band order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("band order = %v, want %v: position is load-bearing where hue is not", got, want)
		}
	}
}

func TestSignalRowsCarryReductionWithoutJudgement(t *testing.T) {
	face := Assemble(richInput())

	if len(face.Signals) != 3 {
		t.Fatalf("signal rows = %d, want one lane per signal", len(face.Signals))
	}
	var traces SignalRow
	for _, row := range face.Signals {
		if row.Signal == requirements.Traces {
			traces = row
		}
	}
	if traces.Volume.Reduction != 900_000 {
		t.Errorf("reduction = %d, want 900000 (in 1000000, out 100000)", traces.Volume.Reduction)
	}
	if traces.Volume.Refused != 0 || traces.Volume.SendFailed != 0 {
		t.Errorf("a reduction raised error-rate readings: %+v", traces.Volume)
	}

	// The same reading with no findings behind it leaves every band
	// green: a filter dropping ninety per cent is doing its job, and
	// nothing on the card grades it (ADR-0040 §3).
	quiet := richInput()
	quiet.Delivery = BandInput{Known: true}
	quiet.Expectation = BandInput{Known: true}
	quiet.Conformance = BandInput{Known: true}
	quiet.Findings = nil
	for band, got := range Assemble(quiet).Bands {
		if got.State != StateOK {
			t.Errorf("%s band = %q under a ninety per cent reduction, want ok: the meter presents the delta and passes no judgement", band, got.State)
		}
	}
}

// ADR-0040 §6 through the contract: a reading nobody took carries no
// arithmetic, and the row still carries its as-of so the surface renders
// last-known-plus-age.
func TestUnknownLanesCarryCauseAndNoArithmetic(t *testing.T) {
	face := Assemble(richInput())

	var logs SignalRow
	for _, row := range face.Signals {
		if row.Signal == requirements.Logs {
			logs = row
		}
	}
	if logs.Volume.Known {
		t.Fatal("an unreadable lane came back Known")
	}
	if logs.Volume.Reduction != 0 || logs.Volume.In != 0 {
		t.Errorf("an unknown lane carries arithmetic: %+v", logs.Volume)
	}
	if logs.Volume.Cause == "" {
		t.Error("an unknown lane carries no cause")
	}
	if logs.Volume.AsOf.IsZero() {
		t.Error("an unknown lane carries no as-of: last-known-plus-age renders from the contract")
	}
}

// #98: a lane the Tier's artefact never wired metered `in 0 / out 0`,
// which is the same shape a broken pipeline reads. The zero was true and
// the rendering was a lie, so the row now carries the lane state and no
// readings at all: there is no number left to mistake for a stopped lane.
//
// The contrast is the whole point, so both cases are asserted together:
// an unwired lane and a wired lane moving nothing meter identically, and
// must not read identically.
func TestAnUnwiredLaneAndAStoppedLaneDoNotReadAlike(t *testing.T) {
	in := zeroedInput()
	in.Lanes = LaneSet{requirements.Logs: true, requirements.Traces: true}

	rows := map[requirements.SignalKind]SignalRow{}
	for _, row := range Assemble(in).Signals {
		rows[row.Signal] = row
	}

	metrics := rows[requirements.Metrics]
	if metrics.Lane != LaneNotApplicable {
		t.Errorf("an unwired lane = %q, want not_applicable", metrics.Lane)
	}
	if metrics.Volume != nil || metrics.Freshness != nil || metrics.Shape != nil {
		t.Errorf("an unwired lane carries readings %+v: a metered zero on a lane that does not exist reads as a fault", metrics)
	}

	// The same meter reading on a lane the artefact does wire: a
	// pipeline that has stopped, and a real finding. It keeps its zero.
	logs := rows[requirements.Logs]
	if logs.Lane != LanePresent {
		t.Errorf("a wired lane = %q, want present", logs.Lane)
	}
	if logs.Volume == nil || !logs.Volume.Known || logs.Volume.In != 0 || logs.Volume.Out != 0 {
		t.Errorf("a stopped lane = %+v, want its metered zero kept: it is a reading, and a finding", logs.Volume)
	}

	traces := rows[requirements.Traces]
	if traces.Lane != LanePresent || traces.Volume == nil || traces.Volume.In != 41_020 {
		t.Errorf("a moving lane = %+v, want its reading carried through", traces)
	}
}

// A collector still serving an older artefact reports flow on a lane the
// current one dropped. Intended and Observed are separate readings and
// neither overrules the other (ADR-0004): suppressing the figures would
// hide the disagreement, which is the interesting part.
func TestAnUnwiredLaneTheMeterHasFiguresForKeepsThem(t *testing.T) {
	in := zeroedInput()
	in.Lanes = LaneSet{requirements.Logs: true}

	for _, row := range Assemble(in).Signals {
		if row.Signal != requirements.Traces {
			continue
		}
		if row.Lane != LanePresent {
			t.Errorf("a lane the meter has 41,020 items for = %q, want present: the pipeline evidently exists", row.Lane)
		}
		if row.Volume == nil || row.Volume.In != 41_020 {
			t.Errorf("a reading was dropped because the config disagreed with it: %+v", row)
		}
	}
}

// ADR-0008 through the lane state: a caller with no artefact to read has
// not established that a lane is absent, and says so.
func TestLanesGoUnknownWhenNoArtefactWasRead(t *testing.T) {
	in := zeroedInput()
	in.Lanes = nil

	for _, row := range Assemble(in).Signals {
		if row.Lane != LaneUnknown {
			t.Errorf("%s lane = %q with no artefact read, want unknown: a lane nobody looked for is not a lane that is not there", row.Signal, row.Lane)
		}
		if row.Volume == nil {
			t.Errorf("%s dropped its readings on an unknown lane: only not_applicable does that", row.Signal)
		}
	}

	// The empty set is the other answer: an artefact was read and it
	// wires nothing. Only the lanes the meter reported nothing for lose
	// their readings.
	in.Lanes = LaneSet{}
	states := map[requirements.SignalKind]LaneState{}
	for _, row := range Assemble(in).Signals {
		states[row.Signal] = row.Lane
	}
	if states[requirements.Metrics] != LaneNotApplicable || states[requirements.Logs] != LaneNotApplicable {
		t.Errorf("lanes = %v, want the unreported ones not_applicable under an artefact that wires none", states)
	}
}

// zeroedInput is richInput with the flow reading the issue describes: a
// lane metering a true zero, and a lane moving items. Which of the zeros
// is a fault and which is an absent lane is not a question the meter can
// answer, and that is why the artefact is asked.
func zeroedInput() Input {
	in := richInput()
	in.Flow = metering.ForTier("data-flow/gateway", telemetry.Metered{
		AsOf:   readAt,
		Window: time.Hour,
		Signals: map[requirements.SignalKind]telemetry.MeteredSignal{
			requirements.Logs:    {Known: true},
			requirements.Metrics: {Known: true},
			requirements.Traces: {
				Known: true, In: 41_020, Out: 41_020,
				Newest: readAt.Add(-30 * time.Second),
			},
		},
		Incarnations: telemetry.Incarnations{Known: true, Count: 2},
	}, derivedAt)
	return in
}

func TestPopulationCarriesTheInventoryOutputsVerbatim(t *testing.T) {
	face := Assemble(richInput())

	if face.Population.Matched != 12 {
		t.Errorf("matched = %d, want 12", face.Population.Matched)
	}
	if face.Population.Floor == nil || *face.Population.Floor != 40 {
		t.Errorf("floor = %v, want 40", face.Population.Floor)
	}
	if face.Population.FloorSource != inventory.FloorDerived {
		t.Errorf("floor source = %q, want derived", face.Population.FloorSource)
	}
	if face.Population.State != PopulationUnderPopulated {
		t.Errorf("state = %q, want under_populated: never_seen and under_populated are siblings, never degrees", face.Population.State)
	}
}

func TestShelfSummaryCountsRideTheFace(t *testing.T) {
	face := Assemble(richInput())

	if face.FindingCounts["delivery"] != 1 || face.FindingCounts["expectation"] != 1 {
		t.Errorf("finding counts = %v, want one per kind", face.FindingCounts)
	}
	if face.WaivedCounts["conformance"] != 1 {
		t.Errorf("waived counts = %v: a waiver waives the count, never the diagnosis, and rides every roll-up", face.WaivedCounts)
	}
	if face.FindingCounts["conformance"] != 1 {
		t.Error("a waived finding vanished from the counts entirely")
	}
}

func TestDrawerRefusesAComplaint(t *testing.T) {
	_, err := NewDrawer("data-flow/gateway", []Finding{{
		ID: "no-fix", Kind: "delivery", Severity: SeverityViolation,
		Summary: "the artefact never applied",
		WhoActs: WhoActs{Target: ObjectRef{Kind: "tier", ID: "data-flow/gateway"}, Label: "Inspect delivery in Topology"},
	}}, nil)
	if err == nil {
		t.Fatal("a finding without remediation was accepted: every finding must say what to do")
	}

	_, err = NewDrawer("data-flow/gateway", []Finding{{
		ID: "nowhere", Kind: "delivery", Severity: SeverityViolation,
		Summary: "the artefact never applied", Remediation: "Re-serve the Tier at head.",
	}}, nil)
	if err == nil {
		t.Fatal("a finding routing to nobody was accepted")
	}
}

func TestDrawerOrdersFindingsWorstFirst(t *testing.T) {
	drawer, err := NewDrawer("data-flow/gateway", []Finding{
		advisory("a"), violation("b"), advisory("c"),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if drawer.Findings[0].ID != "b" {
		t.Errorf("first finding = %q, want the violation", drawer.Findings[0].ID)
	}
	if drawer.ContractVersion != Version {
		t.Errorf("drawer contract version = %d, want %d", drawer.ContractVersion, Version)
	}
}

// The contract is the sole source for card surfaces (ADR-0041 §4), so
// both sides are held to one artefact: this test writes it, and the
// console's own contract test reads the same file. A field added on
// either side without the other noticing is exactly what an
// integer-versioned contract exists to prevent.
func TestSharedContractFixture(t *testing.T) {
	face := Assemble(richInput())
	drawer, err := NewDrawer("data-flow/gateway", []Finding{
		violation("gateway-delivery-stalled"),
		advisory("gateway-expectation-logs"),
	}, []Provenance{{
		Key:   "band:expectation",
		Claim: "arrival claim: logs should land for product/checkout in production (via data-flow/gateway)",
		Lines: []ProvenanceLine{{
			File: "teams/data-flow/blueprints/gateway-standard.yaml",
			Line: 24,
			Text: "    - ref: infosec/pii-redaction@3",
		}},
		SHA:   "8b7df143d91c716ecfa5fc1730022f6b421b05cd",
		Trace: &Trace{Service: "product/checkout"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	payload := struct {
		Face   Face   `json:"face"`
		Drawer Drawer `json:"drawer"`
	}{face, drawer}

	got, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	path := filepath.Join("..", "..", "console", "fixtures", "card-contract.json")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the shared contract fixture: %v (run go test ./internal/card -update)", err)
	}
	if string(got) != string(want) {
		t.Errorf("the assembled payload no longer matches %s.\nThe console reads the same file: bump Version and update both sides, or run go test ./internal/card -update.\n\ngot:\n%s", path, got)
	}
}

func violation(id string) Finding {
	return Finding{
		ID: id, Kind: "delivery", Severity: SeverityViolation, Dampening: DampeningNone,
		Summary:     "the serving artefact never reached APPLIED",
		Remediation: "Re-serve data-flow/gateway at head, then check the supervisor logs on the stalled collectors.",
		WhoActs:     WhoActs{Target: ObjectRef{Kind: "tier", ID: "data-flow/gateway"}, Label: "Inspect delivery in Topology"},
	}
}

func advisory(id string) Finding {
	return Finding{
		ID: id, Kind: "expectation", Severity: SeverityAdvisory, Dampening: DampeningNone,
		Summary:     "the logs lane is authored and nothing has ever landed on it",
		Remediation: "Back the claim with a Requirement, or delete the dead logs lane from gateway-standard.",
		WhoActs: WhoActs{
			Target: ObjectRef{Kind: "blueprint", ID: "data-flow/gateway-standard"},
			Lane:   "logs",
			Label:  "Fix the logs lane in Compose",
		},
	}
}

// richInput is one Tier exercising the contract's whole surface: a
// double-red card with a reduction that is nobody's fault, one lane the
// backend could not read, and an under-populated Tier.
func richInput() Input {
	metered := telemetry.Metered{
		AsOf:   readAt,
		Window: time.Hour,
		Signals: map[requirements.SignalKind]telemetry.MeteredSignal{
			requirements.Traces: {
				Known: true, In: 1_000_000, Out: 100_000,
				Exporters: map[string]int64{"otlp/gateway": 90_000, "debug": 10_000},
				Newest:    readAt.Add(-30 * time.Second),
			},
			requirements.Metrics: {
				Known: true, In: 48_000, Out: 47_900, Refused: 100,
				Exporters: map[string]int64{"otlp/gateway": 47_900},
				Newest:    readAt.Add(-45 * time.Second),
			},
			requirements.Logs: {Known: false, Cause: `no index matches "metrics-*"`},
		},
		Incarnations: telemetry.Incarnations{Known: true, Count: 14},
	}

	deliveryFinding := ownership.Finding{
		Kind:    ownership.Delivery,
		Subject: ownership.Subject{Kind: ownership.KindTier, ID: "data-flow/gateway"},
		Grade:   ownership.Violation,
		Detail:  "the serving artefact never reached APPLIED",
	}
	expectationFinding := ownership.Finding{
		Kind:    ownership.Expectation,
		Subject: ownership.Subject{Kind: ownership.KindTier, ID: "data-flow/gateway"},
		Grade:   ownership.Advisory,
		Detail:  "the logs lane is authored and nothing has ever landed on it",
	}
	waivedConformance := ownership.Finding{
		Kind:    ownership.ServiceConformance,
		Subject: ownership.Subject{Kind: ownership.KindTier, ID: "data-flow/gateway"},
		Grade:   ownership.Advisory,
		Detail:  "pii-redaction@3 runs under an expiring Grant",
		Waived:  true,
	}
	// The roll-up column is service_conformance; the card's conformance
	// band is where it reads.
	waivedConformance.Kind = "conformance"

	return Input{
		Tier:         "data-flow/gateway",
		Name:         "gateway",
		Team:         "data-flow",
		Environment:  "production",
		ServiceClass: "C1",

		Delivery:    BandInput{Known: true, Findings: []ownership.Finding{deliveryFinding}},
		Expectation: BandInput{Known: true, Findings: []ownership.Finding{expectationFinding}},
		Conformance: BandInput{Known: true, Findings: []ownership.Finding{waivedConformance}},

		Flow: metering.ForTier("data-flow/gateway", metered, derivedAt),

		// gateway-standard wires all three lanes, so every row on this
		// face has a pipeline behind its readings.
		Lanes: LaneSet{requirements.Logs: true, requirements.Metrics: true, requirements.Traces: true},

		Shape: map[requirements.SignalKind]ShapeReading{
			requirements.Traces: {
				Reading:  Reading{Known: true, AsOf: readAt},
				Required: 4, Missing: 0,
				Summary: "4 of 4 required attributes present",
			},
			requirements.Metrics: {
				Reading:  Reading{Known: true, AsOf: readAt},
				Required: 2, Missing: 1,
				Summary: "deployment.environment.name missing on landed metrics",
			},
			requirements.Logs: {
				Reading: Reading{Cause: `no index matches "logs-*"`, AsOf: readAt},
			},
		},

		Population: inventory.Population{
			Tier:           "data-flow/gateway",
			Derived:        inventory.Count{Known: true, AsOf: readAt, Instances: 40},
			Seen:           12,
			EverSeen:       true,
			ShortfallSince: readAt.Add(-6 * time.Hour),
		},
		PopulationFindings: []inventory.Finding{{
			Class:  inventory.UnderPopulated,
			Tier:   "data-flow/gateway",
			Grade:  inventory.Violation,
			Seen:   12,
			Since:  readAt.Add(-6 * time.Hour),
			Detail: "expected at least 40, seen 12",
		}},
		Findings: []ownership.Finding{deliveryFinding, expectationFinding, waivedConformance},
	}
}
