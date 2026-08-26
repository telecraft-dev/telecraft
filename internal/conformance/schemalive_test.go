package conformance

import (
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/livecheck"
	"github.com/telecraft-dev/telecraft/internal/livecheck/livechecktest"
	"github.com/telecraft-dev/telecraft/internal/ownership"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// liveRequirement builds one loaded-shaped schema-conformance requirement
// at placement live. Nothing loads one today (the loader still refuses
// placement: live), so these tests call the evaluator directly, which is
// the point of building the arm before the refusal lifts: fully tested
// while still inert.
func liveRequirement(signals ...requirements.SignalKind) requirements.Requirement {
	req := schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}}, signals...)
	req.Placement = requirements.Live
	return req
}

// liveEvidence files one live-check reading under the fixture window.
func liveEvidence(reading telemetry.LiveCheckFindings) Evidence {
	return Evidence{Schema: SchemaEvidence{
		Live: map[time.Duration]telemetry.LiveCheckFindings{schemaWindow: reading},
	}}
}

// liveReading builds a known findings leg over the given liveness leg.
func liveReading(liveness telemetry.LiveCheckLiveness, records ...telemetry.LiveCheckRecord) telemetry.LiveCheckFindings {
	return telemetry.LiveCheckFindings{
		Known:    true,
		AsOf:     time.Now(),
		Window:   schemaWindow,
		Records:  records,
		Liveness: liveness,
	}
}

// fedTap is the healthy liveness leg: items reached the tap and none
// failed to send.
func fedTap() telemetry.LiveCheckLiveness {
	return telemetry.LiveCheckLiveness{Known: true, Sent: 1200}
}

// Criterion: the liveness rule, passing direction. A tap that was fed in
// the window with no send failures and no findings is the one state that
// may read compliant: silence over a fed tap is a clean stream.
func TestSchemaLiveFedQuietTapIsCompliant(t *testing.T) {
	v := evaluateSchema(t, liveRequirement(), liveEvidence(liveReading(fedTap())))

	if len(v.Findings) != 1 {
		t.Fatalf("got %d findings, want the requirement's verdict alone: %v", len(v.Findings), details(v))
	}
	f := v.Findings[0]
	if f.Outcome != Compliant {
		t.Errorf("outcome = %q, want %q (detail: %v)", f.Outcome, Compliant, f.Detail)
	}
	if f.Weight() != ownership.Violation {
		t.Errorf("grade = %q, want the verdict violation-grade", f.Weight())
	}
	if !detailNames(f, "fed") {
		t.Errorf("the detail does not say the tap was fed: %v", f.Detail)
	}
}

// Criterion: the liveness rule, failing direction. A dead tap reads
// unknown, violation-grade, never passing: nothing was sent to the tap,
// so its silence proves nothing, and absence is never not_delivered here
// because a sampled tap not seeing a Service is weak evidence.
func TestSchemaLiveDeadTapIsUnknown(t *testing.T) {
	v := evaluateSchema(t, liveRequirement(),
		liveEvidence(liveReading(telemetry.LiveCheckLiveness{Known: true, Sent: 0})))

	if len(v.Findings) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(v.Findings), details(v))
	}
	f := v.Findings[0]
	if f.Outcome != Unknown {
		t.Errorf("outcome = %q, want %q", f.Outcome, Unknown)
	}
	if f.Outcome == NotDelivered {
		t.Error("a silent tap read as not_delivered: absence off a sampled tap is not that reading")
	}
	if f.Outcome.Passing() {
		t.Error("a dead tap read as passing")
	}
	if !f.Scored() || !f.Failing() {
		t.Error("a dead tap's unknown does not count against the row, so it quietly leaves the denominator")
	}
	if f.Remediation == "" {
		t.Error("no remediation names the tap")
	}
}

// Send failures poke a hole in the sample: part of the window never
// reached the tap, so its silence about that part is not knowledge.
func TestSchemaLiveSendFailuresReadUnknown(t *testing.T) {
	v := evaluateSchema(t, liveRequirement(),
		liveEvidence(liveReading(telemetry.LiveCheckLiveness{Known: true, Sent: 900, SendFailed: 40})))

	f := v.Findings[0]
	if f.Outcome != Unknown {
		t.Errorf("outcome = %q, want %q (detail: %v)", f.Outcome, Unknown, f.Detail)
	}
	if !detailNames(f, "failed to send") {
		t.Errorf("the detail does not say sends failed: %v", f.Detail)
	}
}

// A liveness leg nobody could read is not a fed tap: unknown, with the
// provider's cause said out loud.
func TestSchemaLiveUnreadableLivenessIsUnknown(t *testing.T) {
	v := evaluateSchema(t, liveRequirement(),
		liveEvidence(liveReading(telemetry.LiveCheckLiveness{Known: false, Cause: "the metrics index is unreachable"})))

	f := v.Findings[0]
	if f.Outcome != Unknown {
		t.Errorf("outcome = %q, want %q", f.Outcome, Unknown)
	}
	if !detailNames(f, "the metrics index is unreachable") {
		t.Errorf("the provider's cause did not reach the detail: %v", f.Detail)
	}
}

// Criterion: findings at required level flip the outcome to misconfigured,
// the same flip the landed arm makes: the telemetry reached the tap and is
// the wrong shape. The finding id is carried through rather than
// flattened, and repeated records collapse to one line with a count.
func TestSchemaLiveViolationIsMisconfigured(t *testing.T) {
	rec := livechecktest.Violation("type_mismatch", "span", "GET /api/users", "checkout",
		"Attribute 'http.request.method' has type 'int', expected 'string'.")
	v := evaluateSchema(t, liveRequirement(), liveEvidence(liveReading(fedTap(), rec, rec)))

	f := findingAt(t, v, schemaregistry.Required)
	if f.Outcome != Misconfigured {
		t.Errorf("outcome = %q, want %q (detail: %v)", f.Outcome, Misconfigured, f.Detail)
	}
	if !detailNames(f, "type_mismatch") {
		t.Errorf("the finding id was flattened out of the detail: %v", f.Detail)
	}
	if !detailNames(f, "(2 records)") {
		t.Errorf("repeated records did not collapse to one counted line: %v", f.Detail)
	}
	if f.Remediation == "" || !strings.Contains(f.Remediation, "Schema Registry") {
		t.Errorf("the remediation does not send the reader to the registry's declaration: %q", f.Remediation)
	}
}

// Findings are proof whatever the liveness leg says: they did not come
// from a tap nothing fed, so a violation outranks the unknown a silent
// liveness leg would otherwise read.
func TestSchemaLiveViolationOutranksDeadLiveness(t *testing.T) {
	v := evaluateSchema(t, liveRequirement(), liveEvidence(liveReading(
		telemetry.LiveCheckLiveness{Known: true, Sent: 0},
		livechecktest.Violation("unit_mismatch", "span", "db.query", "checkout", "wrong unit"))))

	if f := findingAt(t, v, schemaregistry.Required); f.Outcome != Misconfigured {
		t.Errorf("outcome = %q, want %q", f.Outcome, Misconfigured)
	}
}

// Criterion: a fed tap with advisory-level findings rides them alongside
// the way the landed arm does: the verdict stays compliant, and the
// improvement lands on its own advisory-grade finding that never feeds
// the binary.
func TestSchemaLiveImprovementsRideAlongside(t *testing.T) {
	v := evaluateSchema(t, liveRequirement(), liveEvidence(liveReading(fedTap(),
		livechecktest.Record(livechecktest.Finding{
			ID:         livecheck.RecommendedAttributeNotPresent,
			Level:      livecheck.LevelImprovement,
			SignalType: "span",
			SignalName: "db.query",
			Service:    "checkout",
			Message:    "Attribute 'server.port' is recommended and not present.",
		}))))

	verdictFinding := findingAt(t, v, schemaregistry.Required)
	if verdictFinding.Outcome != Compliant {
		t.Errorf("an improvement moved the verdict: outcome = %q, want %q", verdictFinding.Outcome, Compliant)
	}

	advisory := findingAt(t, v, schemaregistry.Recommended)
	if advisory.Weight() != ownership.Advisory || advisory.Scored() {
		t.Errorf("the improvement finding feeds the binary: grade %q", advisory.Weight())
	}
	if !detailNames(advisory, livecheck.RecommendedAttributeNotPresent) {
		t.Errorf("the advisory finding does not carry the finding id: %v", advisory.Detail)
	}

	score := v.Score()
	if score.Total != 1 || score.Passing != 1 || score.Advisory != 1 {
		t.Errorf("score = %+v, want one passing verdict with one advisory alongside", score)
	}
}

// Information-level findings land neutral: reported, and settling nothing.
func TestSchemaLiveInformationIsNeutral(t *testing.T) {
	v := evaluateSchema(t, liveRequirement(), liveEvidence(liveReading(fedTap(),
		livechecktest.Record(livechecktest.Finding{
			ID:         "template_attribute",
			Level:      livecheck.LevelInformation,
			SignalType: "span",
			Service:    "checkout",
			Message:    "Attribute matches a template.",
		}))))

	neutral := findingAt(t, v, schemaregistry.OptIn)
	if neutral.Weight() != ownership.Neutral {
		t.Errorf("grade = %q, want %q", neutral.Weight(), ownership.Neutral)
	}
	if s := v.Score(); s.Neutral != 1 {
		t.Errorf("score = %+v, want the information finding counted neutral", s)
	}
}

// The tap reports a conditionally required miss at its violation severity;
// the platform reads the level out of the finding's own name and demotes
// it, so both placements treat an unevaluable condition the same way and a
// live requirement cannot fail on one while its landed twin does not.
func TestSchemaLiveConditionallyRequiredDemotes(t *testing.T) {
	v := evaluateSchema(t, liveRequirement(), liveEvidence(liveReading(fedTap(),
		livechecktest.Violation(livecheck.ConditionallyRequiredAttributeNotPresent,
			"span", "db.query", "checkout", "Attribute 'db.operation.name' is conditionally required and not present."))))

	if f := findingAt(t, v, schemaregistry.Required); f.Outcome != Compliant {
		t.Errorf("a conditionally required miss flipped the verdict: outcome = %q", f.Outcome)
	}
	demoted := findingAt(t, v, schemaregistry.ConditionallyRequired)
	if demoted.Weight() != ownership.Advisory {
		t.Errorf("grade = %q, want %q", demoted.Weight(), ownership.Advisory)
	}
	if !detailNames(demoted, "tighten the level to required") {
		t.Errorf("the detail does not name the supported lever: %v", demoted.Detail)
	}
}

// A finding on a signal the requirement does not cover is another
// requirement's business; a finding the signal vocabulary cannot place (a
// resource, most often) belongs to every covered signal and rides through.
func TestSchemaLiveJudgesOnlyCoveredSignals(t *testing.T) {
	v := evaluateSchema(t, liveRequirement(requirements.Traces), liveEvidence(liveReading(fedTap(),
		livechecktest.Violation("unit_mismatch", "metric", "http.server.request.duration", "checkout", "wrong unit"),
		livechecktest.Violation("entity_required_attribute_not_present", "resource", "", "checkout",
			"Attribute 'service.namespace' is required and not present."))))

	f := findingAt(t, v, schemaregistry.Required)
	if f.Outcome != Misconfigured {
		t.Errorf("outcome = %q, want %q: the resource finding rides through", f.Outcome, Misconfigured)
	}
	if detailNames(f, "unit_mismatch") {
		t.Errorf("a finding on an uncovered signal was judged: %v", f.Detail)
	}
	if !detailNames(f, "entity_required_attribute_not_present") {
		t.Errorf("the resource finding did not ride through: %v", f.Detail)
	}
}

// A truncated reading cannot prove an all-clear: a finding could sit
// exactly where it stopped reading. Presence stays proof, so truncation
// with violations still reads misconfigured.
func TestSchemaLiveTruncationWithdrawsTheAllClear(t *testing.T) {
	t.Run("no violations read", func(t *testing.T) {
		reading := liveReading(fedTap())
		reading.Truncated = true
		f := evaluateSchema(t, liveRequirement(), liveEvidence(reading)).Findings[0]
		if f.Outcome != Unknown {
			t.Errorf("outcome = %q, want %q: a clipped reading cannot prove clean", f.Outcome, Unknown)
		}
	})
	t.Run("violations read", func(t *testing.T) {
		reading := liveReading(fedTap(),
			livechecktest.Violation("type_mismatch", "span", "db.query", "checkout", "wrong type"))
		reading.Truncated = true
		f := findingAt(t, evaluateSchema(t, liveRequirement(), liveEvidence(reading)), schemaregistry.Required)
		if f.Outcome != Misconfigured {
			t.Errorf("outcome = %q, want %q: presence is proof however clipped the set", f.Outcome, Misconfigured)
		}
	})
}

// No reading at all, and a reading nobody could take, both read unknown
// with the cause said out loud: a live requirement never passes on
// evidence nobody gathered.
func TestSchemaLiveMissingReadingIsUnknown(t *testing.T) {
	t.Run("no reading covers the window", func(t *testing.T) {
		v := evaluateSchema(t, liveRequirement(), Evidence{})
		if len(v.Findings) != 1 || v.Findings[0].Outcome != Unknown {
			t.Fatalf("findings = %v, want one unknown", details(v))
		}
	})
	t.Run("the reading is Known false", func(t *testing.T) {
		v := evaluateSchema(t, liveRequirement(), liveEvidence(
			telemetry.LiveCheckUnknown(time.Now(), schemaWindow, "the logs index is unreachable")))
		f := v.Findings[0]
		if f.Outcome != Unknown || !detailNames(f, "the logs index is unreachable") {
			t.Errorf("finding = %+v, want unknown carrying the provider's cause", f)
		}
	})
}

// The live plan is its own leg of the fetch plan: live requirements ask
// for one reading per distinct window, and contribute nothing to the
// landed plans, whose readings nothing would judge.
func TestSchemaLiveReadingsPlan(t *testing.T) {
	landed := schemaRequirement(requirements.Scope{Groups: []string{"span.db.client"}})
	live := liveRequirement()
	live.ID = "db-spans-conform-live"
	lib := requirements.Library{Requirements: map[string]requirements.Requirement{
		landed.ID: landed,
		live.ID:   live,
	}}

	if got := SchemaLiveReadings(lib, "production"); len(got) != 1 || got[0] != schemaWindow {
		t.Errorf("live plan = %v, want the one fixture window", got)
	}
	if got := SchemaReadings(lib, "production"); len(got) != 1 {
		t.Errorf("landed plan = %v, want the landed requirement's readings alone", got)
	}

	gathered := GatherSchema(lib, "production", SchemaSource{
		Live: func(window time.Duration) telemetry.LiveCheckFindings {
			return liveReading(fedTap())
		},
	})
	if reading, have := gathered.Live[schemaWindow]; !have || !reading.Known {
		t.Errorf("GatherSchema did not file the live reading under its window: %+v", gathered.Live)
	}
}

// A library holding only live requirements still gathers: the landed plans
// are empty, and the live reading is the whole of the evidence.
func TestGatherSchemaWithOnlyLiveRequirements(t *testing.T) {
	live := liveRequirement()
	lib := requirements.Library{Requirements: map[string]requirements.Requirement{live.ID: live}}

	gathered := GatherSchema(lib, "production", SchemaSource{
		Live: func(time.Duration) telemetry.LiveCheckFindings { return liveReading(fedTap()) },
	})
	if _, have := gathered.Live[schemaWindow]; !have {
		t.Errorf("a live-only library gathered nothing: %+v", gathered)
	}
}
