package conformance

import (
	"strings"
	"testing"
	"time"
)

// exemptCheckout is a live service-scoped exemption for req()'s requirement.
func exemptCheckout() Exemption {
	return Exemption{
		ID:          "checkout-migration",
		Requirement: "logs-delivered",
		Owner:       "platform-observability",
		Expires:     Date(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)),
		Service:     "checkout",
		Reason:      "migration in progress",
	}
}

// brokenVerdict evaluates checkout/production into one broken_pipeline
// finding, the raw diagnosis every waiver test starts from.
func brokenVerdict(t *testing.T) Verdict {
	t.Helper()
	v := Evaluate(Row{Service: "checkout", Environment: "production"}, lib(req()),
		Evidence{Effective: logsPipeline(), Observed: observedLogs(time.Hour, false)}, evalAt)
	if len(v.Findings) != 1 || v.Findings[0].Outcome != BrokenPipeline {
		t.Fatalf("verdict = %+v, want one broken_pipeline finding", v.Findings)
	}
	return v
}

func checkoutRow() EstateRow {
	return EstateRow{Row: Row{Service: "checkout", Environment: "production"}}
}

// Criterion: a waived finding keeps its diagnosis. The exemption waives the
// count and nothing else — outcome and detail survive verbatim, and the
// waiver names the exemption, its owner and its expiry (ADR-0004, ADR-0037).
func TestExemptionWaivesTheCountNeverTheDiagnosis(t *testing.T) {
	v := brokenVerdict(t)
	detailBefore := strings.Join(v.Findings[0].Detail, "\n")

	w := Waivers{Exemptions: []Exemption{exemptCheckout()}}
	if err := w.Apply(&v, checkoutRow(), evalAt); err != nil {
		t.Fatal(err)
	}

	f := v.Findings[0]
	if f.Waived != WaiverExempt {
		t.Fatalf("waived = %q, want exempt", f.Waived)
	}
	if f.Outcome != BrokenPipeline || strings.Join(f.Detail, "\n") != detailBefore {
		t.Errorf("waiving altered the diagnosis: %+v", f)
	}
	for _, want := range []string{"checkout-migration", "platform-observability", "2026-09-01", "migration in progress"} {
		if !strings.Contains(f.WaiverReason, want) {
			t.Errorf("waiver reason %q does not carry %q", f.WaiverReason, want)
		}
	}
	if s := v.Score(); s.Failing != 0 || s.Waived != 1 {
		t.Errorf("score = %+v, want the failure waived and counted as such", s)
	}
}

func TestExemptionScopeIsExact(t *testing.T) {
	cases := []struct {
		name     string
		mutilate func(*Exemption)
	}{
		{"another service", func(e *Exemption) { e.Service = "billing" }},
		{"another requirement", func(e *Exemption) { e.Requirement = "traces-delivered" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := brokenVerdict(t)
			e := exemptCheckout()
			tc.mutilate(&e)
			if err := (Waivers{Exemptions: []Exemption{e}}).Apply(&v, checkoutRow(), evalAt); err != nil {
				t.Fatal(err)
			}
			if v.Findings[0].Waived != WaiverNone {
				t.Errorf("exemption %+v waived a finding it does not cover", e)
			}
		})
	}
}

// Criterion: an expired Exemption reverts to the raw finding with no manual
// step — expiry is a property of the clock alone, so the identical call
// that waived before the expiry waives nothing after it.
func TestExpiredExemptionRevertsAutomatically(t *testing.T) {
	w := Waivers{Exemptions: []Exemption{exemptCheckout()}}

	before := brokenVerdict(t)
	if err := w.Apply(&before, checkoutRow(), evalAt); err != nil {
		t.Fatal(err)
	}
	if before.Findings[0].Waived != WaiverExempt {
		t.Fatalf("before expiry: waived = %q, want exempt", before.Findings[0].Waived)
	}

	after := brokenVerdict(t)
	expiry := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err := w.Apply(&after, checkoutRow(), expiry); err != nil {
		t.Fatal(err)
	}
	f := after.Findings[0]
	if f.Waived != WaiverNone || f.WaiverReason != "" {
		t.Errorf("at expiry: %+v — the raw finding must be back untouched", f)
	}
	if s := after.Score(); s.Failing != 1 || s.Waived != 0 {
		t.Errorf("score after expiry = %+v, want the failure counting again", s)
	}
}

// A passing finding is never waived: there is nothing to forgive, and a
// waived-passing count would inflate the visibility number roll-ups rely on.
func TestPassingFindingsAreNeverWaived(t *testing.T) {
	v := Evaluate(Row{Service: "checkout", Environment: "production"}, lib(req()),
		Evidence{Effective: logsPipeline(), Observed: observedLogs(time.Hour, true)}, evalAt)
	if v.Findings[0].Outcome != Compliant {
		t.Fatalf("outcome = %q, want compliant", v.Findings[0].Outcome)
	}
	w := Waivers{Exemptions: []Exemption{exemptCheckout()}}
	if err := w.Apply(&v, checkoutRow(), evalAt); err != nil {
		t.Fatal(err)
	}
	if f := v.Findings[0]; f.Waived != WaiverNone || f.WaiverReason != "" {
		t.Errorf("a compliant finding was waived: %+v", f)
	}
}

// A team-scoped exemption covers exactly the subtree the hook says it does
// (ADR-0037 §2), and with no ownership model wired it errors rather than
// silently never applying.
func TestTeamScopedExemption(t *testing.T) {
	teamScoped := exemptCheckout()
	teamScoped.Service = ""
	teamScoped.Team = "payments"

	cases := []struct {
		name string
		in   bool
		want WaiverKind
	}{
		{"service in subtree", true, WaiverExempt},
		{"service outside subtree", false, WaiverNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := brokenVerdict(t)
			w := Waivers{
				Exemptions: []Exemption{teamScoped},
				InSubtree: func(service, team string) (bool, error) {
					if service != "checkout" || team != "payments" {
						t.Errorf("resolved (%q, %q), want (checkout, payments)", service, team)
					}
					return tc.in, nil
				},
			}
			if err := w.Apply(&v, checkoutRow(), evalAt); err != nil {
				t.Fatal(err)
			}
			if v.Findings[0].Waived != tc.want {
				t.Errorf("waived = %q, want %q", v.Findings[0].Waived, tc.want)
			}
		})
	}

	t.Run("no ownership model wired", func(t *testing.T) {
		v := brokenVerdict(t)
		err := (Waivers{Exemptions: []Exemption{teamScoped}}).Apply(&v, checkoutRow(), evalAt)
		if err == nil || !strings.Contains(err.Error(), "no ownership model") {
			t.Fatalf("err = %v, want a refusal naming the missing ownership model", err)
		}
	})
}

// Criterion: Grace computation varies by Service Class per a fixture table.
// One table, one onboarding date, one clock — only the class differs, and
// with it whether the window still holds (REQ-014).
func TestGraceScalesWithServiceClass(t *testing.T) {
	estate, err := LoadEstate(writeEstate(t, `
grace:
  - class: C1
    window: 24h
  - class: C2
    window: 168h
  - class: C3
    window: 720h
services:
  - name: pay
    class: C1
    onboarded: 2026-08-14
    environments:
      - name: production
        pipelines:
          - name: logs
            receivers: [filelog]
            exporters: [otlphttp]
  - name: search
    class: C2
    onboarded: 2026-08-14
    environments:
      - name: production
  - name: wiki
    class: C3
    onboarded: 2026-08-14
    environments:
      - name: production
  - name: legacy
    environments:
      - name: production
  - name: future
    class: C2
    onboarded: 2026-09-14
    environments:
      - name: production
`))
	if err != nil {
		t.Fatal(err)
	}

	// evalAt is 2026-08-17T12:00Z — 84h after onboarding: past C1's 24h
	// window, inside C2's 168h and C3's 720h.
	want := map[string]WaiverKind{
		"pay":    WaiverNone,  // highest class, shortest grace, already over
		"search": WaiverGrace, // mid class, still inside its window
		"wiki":   WaiverGrace, // lowest class, longest window
		"legacy": WaiverNone,  // no class: Grace never applies
		"future": WaiverNone,  // onboarding has not begun
	}
	w := Waivers{Grace: estate.Grace}
	for _, row := range estate.Rows {
		v := Evaluate(row.Row, lib(req()),
			Evidence{Effective: row.Effective, Observed: observedLogs(time.Hour, false)}, evalAt)
		if err := w.Apply(&v, row, evalAt); err != nil {
			t.Fatal(err)
		}
		f := v.Findings[0]
		if f.Outcome.Passing() {
			t.Fatalf("%s: outcome %q — the fixture is built to fail raw", row.Service, f.Outcome)
		}
		if f.Waived != want[row.Service] {
			t.Errorf("%s (class %q): waived = %q, want %q", row.Service, row.Class, f.Waived, want[row.Service])
		}
		if f.Waived == WaiverGrace && !strings.Contains(f.WaiverReason, "Service Class "+row.Class) {
			t.Errorf("%s: waiver reason %q does not name the class", row.Service, f.WaiverReason)
		}
	}
}

// Where an Exemption and Grace cover the same finding, the Exemption is
// credited: it is the authored object with a named owner answering for the
// loosening; Grace is only a platform-applied window.
func TestExemptionOutranksGrace(t *testing.T) {
	v := brokenVerdict(t)
	row := checkoutRow()
	row.Class = "C2"
	row.Onboarded = evalAt.Add(-time.Hour)
	w := Waivers{
		Exemptions: []Exemption{exemptCheckout()},
		Grace:      GracePolicy{{Class: "C2", Window: 168 * time.Hour}},
	}
	if err := w.Apply(&v, row, evalAt); err != nil {
		t.Fatal(err)
	}
	if v.Findings[0].Waived != WaiverExempt {
		t.Errorf("waived = %q, want exempt to outrank grace", v.Findings[0].Waived)
	}
}
