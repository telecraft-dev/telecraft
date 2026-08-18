package conformance

import (
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

var evalAt = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

// req builds one config-and-signal requirement: filelog-or-otlp receiver,
// logs present within the hour. Mutate the copy per case.
func req() requirements.Requirement {
	return requirements.Requirement{
		ID:      "logs-delivered",
		Title:   "Logs are delivered",
		Version: 1,
		Level:   requirements.Required,
		Owner:   "platform-observability",
		Config:  &requirements.ConfigAssertion{HasReceiver: []string{"filelog", "otlp"}},
		Signal: &requirements.SignalAssertion{
			Kind:    requirements.Logs,
			Present: true,
			Window:  requirements.Duration(time.Hour),
		},
		Remediation: "add a filelog receiver",
	}
}

func lib(reqs ...requirements.Requirement) requirements.Library {
	l := requirements.Library{Requirements: map[string]requirements.Requirement{}}
	for _, r := range reqs {
		l.Requirements[r.ID] = r
	}
	return l
}

// logsPipeline is an Effective reading with a logs pipeline that satisfies
// req's config assertion.
func logsPipeline() Effective {
	return Effective{Known: true, Pipelines: []Pipeline{{
		Name:      "logs",
		Receivers: []string{"filelog"},
		Exporters: []string{"otlphttp"},
	}}}
}

// tracesOnlyPipeline wires a receiver of the wanted type into a traces
// pipeline and nothing into logs — the ordered-pipelines case ADR-0004
// singles out.
func tracesOnlyPipeline() Effective {
	return Effective{Known: true, Pipelines: []Pipeline{{
		Name:      "traces",
		Receivers: []string{"filelog"},
		Exporters: []string{"otlphttp"},
	}}}
}

func observedLogs(window time.Duration, present bool) map[time.Duration]telemetry.Observed {
	return map[time.Duration]telemetry.Observed{
		window: {
			AsOf:   evalAt,
			Window: window,
			Signals: map[requirements.SignalKind]telemetry.SignalObservation{
				requirements.Logs: {Known: true, Present: present, Volume: boolVolume(present)},
			},
		},
	}
}

func boolVolume(present bool) int64 {
	if present {
		return 10
	}
	return 0
}

func observedUnknown(window time.Duration) map[time.Duration]telemetry.Observed {
	return map[time.Duration]telemetry.Observed{
		window: telemetry.Unknown(evalAt, window, "backend unreachable: connection refused"),
	}
}

// Criterion: table-driven coverage of all seven outcomes.
func TestCrossProducesAllSevenOutcomes(t *testing.T) {
	misconfigOnly := req()
	misconfigOnly.Signal = nil
	signalOnly := req()
	signalOnly.Config = nil

	cases := []struct {
		name    string
		req     requirements.Requirement
		ev      Evidence
		want    Outcome
		passing bool
		detail  string // a fragment the finding's detail must carry, "" for none required
	}{
		{
			name:    "effective yes observed yes is compliant",
			req:     req(),
			ev:      Evidence{Effective: logsPipeline(), Observed: observedLogs(time.Hour, true)},
			want:    Compliant,
			passing: true,
		},
		{
			name:   "effective yes observed no is broken_pipeline",
			req:    req(),
			ev:     Evidence{Effective: logsPipeline(), Observed: observedLogs(time.Hour, false)},
			want:   BrokenPipeline,
			detail: "no logs received",
		},
		{
			name:    "effective no observed yes is ungoverned and passes",
			req:     req(),
			ev:      Evidence{Effective: Effective{Known: true}, Observed: observedLogs(time.Hour, true)},
			want:    Ungoverned,
			passing: true,
			detail:  "no receiver of type filelog or otlp",
		},
		{
			name: "effective no observed no is not_configured",
			req:  req(),
			ev:   Evidence{Effective: Effective{Known: true}, Observed: observedLogs(time.Hour, false)},
			want: NotConfigured,
		},
		{
			name:   "observed absence alone is not_delivered",
			req:    signalOnly,
			ev:     Evidence{Effective: logsPipeline(), Observed: observedLogs(time.Hour, false)},
			want:   NotDelivered,
			detail: "no logs received",
		},
		{
			name:   "failed config assertion alone is misconfigured",
			req:    misconfigOnly,
			ev:     Evidence{Effective: Effective{Known: true}, Observed: observedLogs(time.Hour, true)},
			want:   Misconfigured,
			detail: "no receiver of type filelog or otlp",
		},
		{
			name:   "no evidence from any reading is unknown",
			req:    req(),
			ev:     Evidence{Effective: Effective{Known: false, Cause: "collector not reporting"}, Observed: observedUnknown(time.Hour)},
			want:   Unknown,
			detail: "collector not reporting",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := Evaluate(Row{Service: "checkout", Environment: "production"}, lib(tc.req), tc.ev, evalAt)
			if len(v.Findings) != 1 {
				t.Fatalf("got %d findings, want 1", len(v.Findings))
			}
			f := v.Findings[0]
			if f.Outcome != tc.want {
				t.Fatalf("outcome = %s, want %s (detail: %v)", f.Outcome, tc.want, f.Detail)
			}
			if f.Outcome.Passing() != tc.passing {
				t.Errorf("%s.Passing() = %v, want %v", f.Outcome, f.Outcome.Passing(), tc.passing)
			}
			if tc.detail != "" && !strings.Contains(strings.Join(f.Detail, "\n"), tc.detail) {
				t.Errorf("detail %v does not mention %q", f.Detail, tc.detail)
			}
		})
	}
}

// Criterion: the severity ordering. Broken pipelines lead — configured with
// intent and silently not working — and unknown outranks ungoverned because
// not being able to see is worse than seeing something unexpected.
func TestSeverityOrdering(t *testing.T) {
	want := []Outcome{BrokenPipeline, NotConfigured, NotDelivered, Misconfigured, Unknown, Ungoverned, Compliant}
	got := Outcomes()
	if len(got) != 7 {
		t.Fatalf("Outcomes() returned %d outcomes, want the seven", len(got))
	}
	for i, o := range got {
		if o != want[i] {
			t.Fatalf("Outcomes()[%d] = %s, want %s", i, o, want[i])
		}
		if !o.Valid() {
			t.Errorf("%s does not report itself valid", o)
		}
		if i > 0 && got[i-1].Severity() <= o.Severity() {
			t.Errorf("severity(%s)=%d is not strictly above severity(%s)=%d",
				got[i-1], got[i-1].Severity(), o, o.Severity())
		}
	}
	for _, o := range got {
		if wantPass := o == Compliant || o == Ungoverned; o.Passing() != wantPass {
			t.Errorf("%s.Passing() = %v, want %v", o, o.Passing(), wantPass)
		}
	}
}

// The ADR-0004 headline case: a receiver of the wanted type wired only into
// a traces pipeline. The bare assertion matches any pipeline (the documented
// migration path), so the config reads as satisfied — and the absence of
// logs then diagnoses as broken_pipeline, the finding a flat component list
// could never produce the evidence for.
func TestOrderedPipelinesCarryTheBrokenPipelineCase(t *testing.T) {
	v := Evaluate(Row{Service: "checkout", Environment: "production"}, lib(req()),
		Evidence{Effective: tracesOnlyPipeline(), Observed: observedLogs(time.Hour, false)}, evalAt)
	if got := v.Findings[0].Outcome; got != BrokenPipeline {
		t.Fatalf("outcome = %s, want broken_pipeline", got)
	}
}

// Component matching is by type prefix: "otlp/onramp" satisfies a
// requirement for "otlp", which is what anyone writing the requirement meant.
func TestConfigMatchesQualifiedComponentNames(t *testing.T) {
	eff := Effective{Known: true, Pipelines: []Pipeline{{
		Name:      "logs",
		Receivers: []string{"otlp/onramp"},
	}}}
	v := Evaluate(Row{Service: "checkout", Environment: "production"}, lib(req()),
		Evidence{Effective: eff, Observed: observedLogs(time.Hour, true)}, evalAt)
	if got := v.Findings[0].Outcome; got != Compliant {
		t.Fatalf("outcome = %s, want compliant — otlp/onramp satisfies otlp (detail: %v)", got, v.Findings[0].Detail)
	}
}

// Criterion (ADR-0033): the same Service in two environments yields
// independent verdicts. Each row is judged in a separate call over its own
// Evidence, and an environment-scoped requirement produces no finding at all
// outside its list — so no blending is possible by construction.
func TestEnvironmentsYieldIndependentVerdicts(t *testing.T) {
	prodOnly := req()
	prodOnly.ID = "logs-recent-production"
	prodOnly.Environments = []string{"production"}
	everywhere := req()

	l := lib(everywhere, prodOnly)

	prod := Evaluate(Row{Service: "checkout", Environment: "production"}, l,
		Evidence{Effective: logsPipeline(), Observed: observedLogs(time.Hour, false)}, evalAt)
	staging := Evaluate(Row{Service: "checkout", Environment: "staging"}, l,
		Evidence{Effective: logsPipeline(), Observed: observedLogs(time.Hour, true)}, evalAt)

	if got := len(prod.Findings); got != 2 {
		t.Fatalf("production row has %d findings, want 2", got)
	}
	if got := len(staging.Findings); got != 1 {
		t.Fatalf("staging row has %d findings, want 1 — the production-scoped requirement must not apply", got)
	}
	for _, f := range prod.Findings {
		if f.Outcome != BrokenPipeline {
			t.Errorf("production %s = %s, want broken_pipeline", f.Requirement.ID, f.Outcome)
		}
	}
	if got := staging.Findings[0].Outcome; got != Compliant {
		t.Errorf("staging outcome = %s, want compliant — staging evidence judged the staging row", got)
	}
	if prod.Score().Failing != 2 || staging.Score().Failing != 0 {
		t.Errorf("scores blended across environments: production %+v, staging %+v", prod.Score(), staging.Score())
	}
	if prod.Worst() != BrokenPipeline || staging.Worst() != Compliant {
		t.Errorf("worst badges blended: production %s, staging %s", prod.Worst(), staging.Worst())
	}
}

// Each requirement is judged against the window it asked for, not against
// whatever window happened to be read: an hour of silence diagnoses even
// when the daily reading has records.
func TestRequirementJudgedAgainstItsOwnWindow(t *testing.T) {
	hourly := req()
	hourly.ID = "logs-recent"
	hourly.Signal.Window = requirements.Duration(time.Hour)
	daily := req()
	daily.ID = "logs-daily"
	daily.Signal.Window = requirements.Duration(24 * time.Hour)

	ev := Evidence{Effective: logsPipeline(), Observed: map[time.Duration]telemetry.Observed{}}
	for w, present := range map[time.Duration]bool{time.Hour: false, 24 * time.Hour: true} {
		for k, v := range observedLogs(w, present) {
			ev.Observed[k] = v
		}
	}

	v := Evaluate(Row{Service: "checkout", Environment: "production"}, lib(hourly, daily), ev, evalAt)
	byID := map[string]Outcome{}
	for _, f := range v.Findings {
		byID[f.Requirement.ID] = f.Outcome
	}
	if byID["logs-daily"] != Compliant {
		t.Errorf("logs-daily = %s, want compliant against the daily reading", byID["logs-daily"])
	}
	if byID["logs-recent"] != BrokenPipeline {
		t.Errorf("logs-recent = %s, want broken_pipeline against the hourly reading", byID["logs-recent"])
	}
}

// Volume and attribute-coverage floors fail a signal that presence alone
// would pass.
func TestSignalFloors(t *testing.T) {
	r := req()
	r.Signal.MinVolume = 100
	r.Signal.RequiredAttributes = []string{"resource.attributes.service.name"}
	coverage := 0.99
	r.Signal.AttributeCoverage = &coverage

	obs := observedLogs(time.Hour, true)
	sig := obs[time.Hour].Signals[requirements.Logs]
	sig.Volume = 40
	sig.AttributeCoverage = map[string]float64{"resource.attributes.service.name": 0.5}
	obs[time.Hour].Signals[requirements.Logs] = sig

	v := Evaluate(Row{Service: "checkout", Environment: "production"}, lib(r),
		Evidence{Effective: logsPipeline(), Observed: obs}, evalAt)
	f := v.Findings[0]
	if f.Outcome != BrokenPipeline {
		t.Fatalf("outcome = %s, want broken_pipeline", f.Outcome)
	}
	joined := strings.Join(f.Detail, "\n")
	for _, want := range []string{"volume 40 is below the minimum of 100", `attribute "resource.attributes.service.name" present on 50%`} {
		if !strings.Contains(joined, want) {
			t.Errorf("detail %v does not mention %q", f.Detail, want)
		}
	}
}

// A per-signal degradation is not a verdict: a requirement whose signal
// reading is unavailable falls back to the evidence that exists, with the
// provider's cause preserved in the detail (ADR-0008).
func TestDegradedSignalReadingFallsBackToEffective(t *testing.T) {
	v := Evaluate(Row{Service: "checkout", Environment: "production"}, lib(req()),
		Evidence{Effective: logsPipeline(), Observed: observedUnknown(time.Hour)}, evalAt)
	f := v.Findings[0]
	if f.Outcome != Compliant {
		t.Fatalf("outcome = %s, want compliant on the effective evidence alone", f.Outcome)
	}
	if !strings.Contains(strings.Join(f.Detail, "\n"), "logs reading unavailable") {
		t.Errorf("detail %v does not preserve the degraded reading's cause", f.Detail)
	}
}

// Waivers count out, never erase: the diagnosis and detail survive, the
// score moves from failing to waived, and the worst badge prefers a live
// finding over a waived one however severe.
func TestWaivedFindingsStopCountingButStayDiagnosed(t *testing.T) {
	signalOnly := req()
	signalOnly.ID = "logs-arriving"
	signalOnly.Config = nil

	v := Evaluate(Row{Service: "checkout", Environment: "production"}, lib(req(), signalOnly),
		Evidence{Effective: logsPipeline(), Observed: observedLogs(time.Hour, false)}, evalAt)
	if s := v.Score(); s.Failing != 2 {
		t.Fatalf("score before waiver = %+v, want 2 failing", s)
	}
	if v.Worst() != BrokenPipeline {
		t.Fatalf("worst before waiver = %s, want broken_pipeline", v.Worst())
	}

	for i, f := range v.Findings {
		if f.Outcome == BrokenPipeline {
			v.Findings[i].Waived = WaiverExempt
			v.Findings[i].WaiverReason = "migration in progress (owner platform-observability, expires 2026-09-01)"
		}
	}

	s := v.Score()
	if s.Total != 2 || s.Failing != 1 || s.Waived != 1 {
		t.Fatalf("score after waiver = %+v, want total 2, failing 1, waived 1", s)
	}
	if v.Worst() != NotDelivered {
		t.Errorf("worst = %s, want not_delivered — a waived broken_pipeline must not outrank a live finding", v.Worst())
	}
	for _, f := range v.Findings {
		if f.Waived != WaiverNone && f.Outcome != BrokenPipeline {
			t.Errorf("waiver landed on the wrong finding: %+v", f)
		}
		if f.Waived != WaiverNone && len(f.Detail) == 0 {
			t.Errorf("waiving erased the diagnosis: %+v", f)
		}
	}

	// A row with every failure waived scores 1.0 — nothing is known to be
	// wrong — with the waived count alongside so it cannot hide.
	for i := range v.Findings {
		if v.Findings[i].Failing() {
			v.Findings[i].Waived = WaiverGrace
		}
	}
	if got := v.Score(); got.Ratio() != 1.0 || got.Waived != 2 {
		t.Errorf("all-waived score = %+v ratio %v, want ratio 1.0 with 2 waived", got, got.Ratio())
	}
}
