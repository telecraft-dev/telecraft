package console_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/console"
)

// The snapshot generator over the fixture estate under testdata/. The
// estate is deliberately small and deliberately imperfect: one Tier below
// its declared population floor, one lane breaching the C1 stability
// floor, one requirement failing under an authored Exemption, and two
// collectors matching no selector at all. Every assertion below is about
// the snapshot carrying what the evaluators decided, never about this
// package deciding anything.

const fixtureCommit = "1111111111111111111111111111111111111111"

func fixtureInputs() console.Inputs {
	root := "testdata/estate"
	return console.Inputs{
		Root:         root,
		Active:       root + "/catalogues/catalogue-v1.0.0.json",
		Catalogues:   []string{root + "/catalogues/catalogue-v1.0.0.json"},
		Library:      root + "/requirements",
		Exemptions:   root + "/exemptions",
		EstateFile:   root + "/rows.yaml",
		ReadingsFile: root + "/readings.yaml",
		Commit:       fixtureCommit,
		Repository:   "telecraft-dev/estate-fixture",
		User: console.User{
			ID:    "demo",
			Name:  "Demo user",
			Email: "demo@estate.internal",
			Team:  "data-flow",
		},
	}
}

func build(t *testing.T) console.Bundle {
	t.Helper()
	bundle, err := console.Build(fixtureInputs())
	if err != nil {
		t.Fatalf("building the snapshot: %v", err)
	}
	return bundle
}

func cardFor(t *testing.T, b console.Bundle, tier string) console.CardFace {
	t.Helper()
	for _, c := range b.Estate.Cards {
		if c.Tier == tier {
			return c
		}
	}
	t.Fatalf("no card for tier %q", tier)
	return console.CardFace{}
}

func TestSnapshotCarriesTheCardContract(t *testing.T) {
	b := build(t)

	if b.Meta.Commit != fixtureCommit {
		t.Errorf("meta commit = %q, want the commit the snapshot was taken at", b.Meta.Commit)
	}
	if len(b.Estate.Cards) != 1 {
		t.Fatalf("cards = %d, want one per authored Tier", len(b.Estate.Cards))
	}

	card := cardFor(t, b, "data-flow/gateway")
	if card.ContractVersion != console.ContractVersion {
		t.Errorf("contract version = %d, want %d", card.ContractVersion, console.ContractVersion)
	}
	// Strictness is derived from traversal, never authored (ADR-0025 §4).
	if card.ServiceClass != "C1" {
		t.Errorf("service class = %q, want C1 derived from the traversing Service", card.ServiceClass)
	}
	for _, band := range []string{"delivery", "expectation", "conformance"} {
		if _, ok := card.Bands[band]; !ok {
			t.Errorf("face carries no %s band: the three bands are the contract", band)
		}
	}
}

func TestPopulationLineIsTheInventoryOutputVerbatim(t *testing.T) {
	card := cardFor(t, build(t), "data-flow/gateway")

	if card.Population.Matched != 3 {
		t.Errorf("matched = %d, want the three collectors the selector matched", card.Population.Matched)
	}
	if card.Population.FloorSource != "declared" {
		t.Errorf("floor source = %q, want declared: no substrate answered, so min_expected stands in", card.Population.FloorSource)
	}
	if card.Population.Floor == nil || *card.Population.Floor != 3 {
		t.Errorf("floor = %v, want the Tier's declared min_expected", card.Population.Floor)
	}
}

func TestUngovernedCollectorsAreMarkedByHowTheyAreRead(t *testing.T) {
	b := build(t)

	var served, foreign, governed int
	for _, c := range b.Estate.Collectors {
		switch c.Ungoverned {
		case "served":
			served++
		case "foreign":
			foreign++
		case "":
			governed++
			if c.Tier == "" {
				t.Errorf("collector %q is neither governed nor marked ungoverned", c.ID)
			}
		default:
			t.Errorf("collector %q has ungoverned kind %q, want served or foreign", c.ID, c.Ungoverned)
		}
	}
	if served != 1 || foreign != 1 {
		t.Errorf("ungoverned split = %d served, %d foreign; want one of each", served, foreign)
	}
	if governed != 3 {
		t.Errorf("governed collectors = %d, want the three the Tier selector matched", governed)
	}
}

func TestAWaivedFindingKeepsItsDiagnosisAndGivesUpOnlyItsCount(t *testing.T) {
	b := build(t)
	card := cardFor(t, b, "data-flow/gateway")

	if card.WaivedCounts["conformance"] == 0 {
		t.Fatalf("no waived conformance count: the Exemption should waive the metrics finding")
	}

	drawer := b.Estate.Drawers["data-flow/gateway"]
	var waived *console.Finding
	for i := range drawer.Findings {
		if drawer.Findings[i].Dampening == "waived" {
			waived = &drawer.Findings[i]
		}
	}
	if waived == nil {
		t.Fatal("the waived finding is absent from the drawer: a waiver waives the count, never the diagnosis")
	}
	if !strings.Contains(waived.Summary, "metrics-delivered") {
		t.Errorf("waived finding summary = %q, want the requirement it diagnoses", waived.Summary)
	}
	if waived.Remediation == "" {
		t.Error("a finding without remediation tells the reader nothing to do")
	}
}

func TestTheFloorBreachRidesTheConformanceBand(t *testing.T) {
	b := build(t)
	card := cardFor(t, b, "data-flow/gateway")

	if card.Bands["conformance"].State != console.BandFinding {
		t.Fatalf("conformance band = %q, want a finding: the logs lane breaches the C1 floor", card.Bands["conformance"].State)
	}

	var found bool
	for _, f := range b.Estate.Drawers["data-flow/gateway"].Findings {
		if strings.Contains(f.Summary, "below the beta floor") {
			found = true
			if f.WhoActs.Lane != "logs" {
				t.Errorf("who-acts lane = %q, want the offending lane", f.WhoActs.Lane)
			}
			if f.WhoActs.Target.Kind != "blueprint" {
				t.Errorf("who-acts target kind = %q, want the Blueprint the fix lives in", f.WhoActs.Target.Kind)
			}
		}
	}
	if !found {
		t.Error("the render's floor finding is missing from the drawer")
	}
}

func TestEveryFindingCarriesRemediationAndAKnownSeverity(t *testing.T) {
	b := build(t)
	for tier, drawer := range b.Estate.Drawers {
		for _, f := range drawer.Findings {
			if f.Remediation == "" {
				t.Errorf("%s: finding %q carries no remediation: every finding tells the reader what to do", tier, f.ID)
			}
			switch f.Severity {
			case console.SeverityViolation, console.SeverityAdvisory, console.SeverityNone:
			default:
				t.Errorf("%s: finding %q has severity %q", tier, f.ID, f.Severity)
			}
			switch f.Kind {
			case "delivery", "expectation", "conformance":
			default:
				t.Errorf("%s: finding %q has kind %q: the bands are the finding kinds", tier, f.ID, f.Kind)
			}
		}
	}
}

func TestProvenanceIsFedFromTheRepositoryNeverReconstructed(t *testing.T) {
	b := build(t)
	drawer := b.Estate.Drawers["data-flow/gateway"]

	byKey := map[string]console.Provenance{}
	for _, p := range drawer.Provenance {
		byKey[p.Key] = p
		if p.SHA != fixtureCommit {
			t.Errorf("provenance %q carries SHA %q, want the commit judged at", p.Key, p.SHA)
		}
	}
	class, ok := byKey["service-class"]
	if !ok {
		t.Fatal("no service-class provenance: a derived value must be able to explain itself")
	}
	if class.Trace == nil || class.Trace.Service != "product/checkout" {
		t.Errorf("service-class trace = %v, want the Service whose Path imposed the class", class.Trace)
	}
	var cited bool
	for _, line := range class.Lines {
		if strings.HasSuffix(line.File, "services/checkout.yaml") && line.Line > 0 && line.Text != "" {
			cited = true
		}
	}
	if !cited {
		t.Error("the service-class derivation cites no authored line from the estate")
	}
}

func TestGovernanceAndCatalogueTravelWithTheSnapshot(t *testing.T) {
	b := build(t)

	if len(b.Estate.AllowLists) != 1 || b.Estate.AllowLists[0].Team != "data-flow" {
		t.Errorf("allow-lists = %v, want the one the estate authored", b.Estate.AllowLists)
	}
	if b.Catalogues.Active != "v1.0.0" {
		t.Errorf("active catalogue = %q, want the artefact the snapshot judged with", b.Catalogues.Active)
	}
	if len(b.Catalogues.Versions) != 1 || len(b.Catalogues.Versions[0].Components) == 0 {
		t.Errorf("catalogue versions = %v, want the installed artefact with its entries", b.Catalogues.Versions)
	}
	if len(b.Estate.Blueprints) != 1 {
		t.Fatalf("blueprints = %d, want the one the estate authored", len(b.Estate.Blueprints))
	}
	bp := b.Estate.Blueprints[0]
	if bp.Tier != "data-flow/gateway" {
		t.Errorf("blueprint tier = %q, want the Tier it is bound to", bp.Tier)
	}
	if len(bp.Lanes["logs"]) == 0 {
		t.Error("the logs lane is empty: lanes are explicitly ordered and never re-sorted")
	}
}

func TestAStaleRenderedTreeRefusesTheSnapshot(t *testing.T) {
	in := fixtureInputs()
	// A different commit renders different artefacts, so the committed
	// tree no longer matches the sources: the recompute invariant
	// (ADR-0028 §2), which a snapshot must never build over.
	in.Commit = "2222222222222222222222222222222222222222"

	_, err := console.Build(in)
	if err == nil {
		t.Fatal("a snapshot was built over a rendered tree that does not match the sources")
	}
	if !strings.Contains(err.Error(), "re-render") {
		t.Errorf("error = %v, want it to name the fix", err)
	}
}

func TestABuildWithoutACommitIsRefused(t *testing.T) {
	in := fixtureInputs()
	in.Commit = ""
	if _, err := console.Build(in); err == nil {
		t.Fatal("a snapshot was built with no commit to stamp claims and provenance with")
	}
}

func TestTheFaceCarriesTheVersionThreeContract(t *testing.T) {
	card := cardFor(t, build(t), "data-flow/gateway")

	if card.ContractVersion != 3 {
		t.Errorf("contract version = %d, want 3: the per-signal matrix with its lane states", card.ContractVersion)
	}
	if len(card.Signals) != 3 {
		t.Fatalf("signal rows = %d, want one per signal the seam covers", len(card.Signals))
	}
	for _, row := range card.Signals {
		if row.Lane == console.LaneNotApplicable {
			continue
		}
		// The estate declares arrivals, not flow: the metering readings are
		// derived on read from a backend a snapshot has none of, so they
		// must say "cannot see" rather than stand a zero in for it.
		if row.Volume.Known || row.Freshness.Known || row.Shape.Known {
			t.Errorf("%s row claims a known flow reading the estate never declared", row.Signal)
		}
		if row.Volume.Cause == "" || row.Volume.AsOf == "" {
			t.Errorf("%s volume reading carries no cause or no as-of: an unknown is still a statement with a timestamp", row.Signal)
		}
	}
	if card.Churn.Known {
		t.Error("the churn reading claims to be known without a backend to derive it from")
	}
}

// #98: gateway-standard wires logs and traces and no metrics lane, so the
// Tier's metrics row has no pipeline behind it. Before the lane state it
// metered `in 0 / out 0`, indistinguishable from a pipeline that has
// broken, which is the opposite finding.
func TestALaneTheBlueprintDoesNotWireReadsNotApplicable(t *testing.T) {
	card := cardFor(t, build(t), "data-flow/gateway")

	rows := map[string]console.SignalRow{}
	for _, row := range card.Signals {
		rows[row.Signal] = row
	}

	metrics, ok := rows["metrics"]
	if !ok {
		t.Fatal("the metrics row vanished: a lane with no pipeline is still a row on the matrix")
	}
	if metrics.Lane != console.LaneNotApplicable {
		t.Errorf("the metrics lane = %q, want not_applicable: gateway-standard wires no metrics pipeline", metrics.Lane)
	}
	if metrics.Volume != nil || metrics.Freshness != nil || metrics.Shape != nil {
		t.Errorf("the metrics row carries readings %+v, but there is no pipeline to have read", metrics)
	}

	for _, signal := range []string{"traces", "logs"} {
		row := rows[signal]
		if row.Lane != console.LanePresent {
			t.Errorf("the %s lane = %q, want present: gateway-standard wires it", signal, row.Lane)
		}
		if row.Volume == nil {
			t.Errorf("the %s row dropped its reading: only an unwired lane does that", signal)
		}
	}
}

func TestThePopulationStateNamesWhichSiblingHolds(t *testing.T) {
	card := cardFor(t, build(t), "data-flow/gateway")
	// Three collectors against a declared floor of three: nothing is short.
	if card.Population.State != console.PopulationOK {
		t.Errorf("population state = %q, want ok: the Tier meets its floor", card.Population.State)
	}
	if card.Population.StaleConfig {
		t.Error("a populated Tier is flagged as stale config")
	}
}

// Every list in a drawer is a list, including when it is empty.
//
// A nil slice marshals to `null`, and the console reads `findings.length`
// without first asking whether the list is there, correctly, because the
// card contract promises a list (ADR-0041). A Tier with nothing to report
// crashed the drawer it opened into, which is a state every healthy estate
// reaches and which no fixture whose Tiers all have findings can catch. So
// this asserts on the type's own guarantee rather than on the fixture.
func TestADrawerWithNothingToReportStillMarshalsLists(t *testing.T) {
	encoded, err := json.Marshal(console.CardDrawer{
		ContractVersion: console.ContractVersion,
		Tier:            "team/quiet",
	})
	if err != nil {
		t.Fatal(err)
	}

	var back map[string]any
	if err := json.Unmarshal(encoded, &back); err != nil {
		t.Fatal(err)
	}
	if _, ok := back["findings"].([]any); !ok {
		t.Errorf("findings marshalled as %T, and a reader taking its length gets a TypeError", back["findings"])
	}
	if _, ok := back["provenance"].([]any); !ok {
		t.Errorf("provenance marshalled as %T, and a reader taking its length gets a TypeError", back["provenance"])
	}
}

// And the whole snapshot the console actually fetches carries no null list
// in any drawer.
func TestNoDrawerInASnapshotMarshalsANullList(t *testing.T) {
	encoded, err := json.Marshal(build(t))
	if err != nil {
		t.Fatal(err)
	}

	var back struct {
		Estate struct {
			Drawers map[string]struct {
				Findings   *[]any `json:"findings"`
				Provenance *[]any `json:"provenance"`
			} `json:"drawers"`
		} `json:"estate"`
	}
	if err := json.Unmarshal(encoded, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Estate.Drawers) == 0 {
		t.Fatal("no drawers in the snapshot, so this proves nothing")
	}
	for id, drawer := range back.Estate.Drawers {
		if drawer.Findings == nil {
			t.Errorf("%s marshalled findings as null, and a reader taking its length gets a TypeError", id)
		}
		if drawer.Provenance == nil {
			t.Errorf("%s marshalled provenance as null, and a reader taking its length gets a TypeError", id)
		}
	}
}
