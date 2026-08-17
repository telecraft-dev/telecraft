package conformance

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// Evidence is everything known about one row at one moment: its Effective
// reading and its Observed readings, both scoped to the row's Environment by
// whoever gathered them — evidence for two environments never meets in one
// Evidence value (ADR-0033).
//
// Observed is keyed by window because requirements legitimately disagree
// about timescale: "traces in the last hour" and "a deployment marker in the
// last 30 days" are both reasonable, and judging the first against 30 days
// of data would pass a Service whose tracing died this morning. The caller
// reads each distinct window once and the evaluator selects per requirement.
type Evidence struct {
	Effective Effective
	Observed  map[time.Duration]telemetry.Observed
}

// ObservedIn returns the reading covering a window, and whether one exists.
func (e Evidence) ObservedIn(w time.Duration) (telemetry.Observed, bool) {
	o, ok := e.Observed[w]
	return o, ok
}

// Evaluate judges one row: every requirement that applies in the row's
// Environment (ADR-0033 — an env-scoped requirement simply produces no
// finding elsewhere), crossed against the row's evidence. Verdicts for the
// same Service in two environments come from two calls over two Evidence
// values; nothing here can blend them.
func Evaluate(row Row, lib requirements.Library, ev Evidence, now time.Time) Verdict {
	v := Verdict{Row: row, EvaluatedAt: now}
	for _, req := range lib.Sorted() {
		if !req.AppliesTo(row.Environment) {
			continue
		}
		v.Findings = append(v.Findings, judge(req, ev))
	}
	return v
}

// judge evaluates one requirement and performs the cross.
func judge(req requirements.Requirement, ev Evidence) Finding {
	f := Finding{Requirement: req}

	cfgOK, cfgKnown, cfgDetail := checkConfig(req.Config, ev.Effective)
	sigOK, sigKnown, sigDetail := checkSignal(req.Signal, ev)
	f.Detail = append(append([]string{}, cfgDetail...), sigDetail...)

	switch {
	// Nothing to go on. Not a pass and not a failure (ADR-0008).
	case !cfgKnown && !sigKnown:
		f.Outcome = Unknown
		if len(f.Detail) == 0 {
			f.Detail = []string{"no evidence available from any reading"}
		}

	// Both readings available: this is the cross, and the reason the
	// platform exists. Four inputs, four genuinely different diagnoses.
	case cfgKnown && sigKnown:
		switch {
		case cfgOK && sigOK:
			f.Outcome = Compliant
		case cfgOK && !sigOK:
			f.Outcome = BrokenPipeline
		case !cfgOK && sigOK:
			f.Outcome = Ungoverned
		default:
			f.Outcome = NotConfigured
		}

	// Observed only. Presence is proof; absence cannot name a cause.
	case sigKnown:
		if sigOK {
			f.Outcome = Compliant
		} else {
			f.Outcome = NotDelivered
		}

	// Effective only. Weakest evidence there is: it describes intent.
	default:
		if cfgOK {
			f.Outcome = Compliant
		} else {
			f.Outcome = Misconfigured
		}
	}
	return f
}

// checkConfig returns (satisfied, evaluable, detail). A nil assertion and an
// unavailable Effective reading are both "not evaluable" — the cross decides
// what that means, this function never does.
func checkConfig(a *requirements.ConfigAssertion, eff Effective) (bool, bool, []string) {
	if a == nil {
		return false, false, nil
	}
	if !eff.Known {
		cause := eff.Cause
		if cause == "" {
			cause = "no effective config reported"
		}
		return false, false, []string{"effective reading unavailable: " + cause}
	}

	ok := true
	var detail []string
	for _, group := range []struct {
		label string
		want  []string
		have  []string
	}{
		{"receiver", a.HasReceiver, eff.componentsOf(func(p Pipeline) []string { return p.Receivers })},
		{"processor", a.HasProcessor, eff.componentsOf(func(p Pipeline) []string { return p.Processors })},
		{"exporter", a.HasExporter, eff.componentsOf(func(p Pipeline) []string { return p.Exporters })},
	} {
		if len(group.want) == 0 {
			continue
		}
		if anyPresent(group.want, group.have) {
			continue
		}
		ok = false
		detail = append(detail, fmt.Sprintf("no %s of type %s in any pipeline (found: %s)",
			group.label, strings.Join(group.want, " or "), joinOrNone(group.have)))
	}
	return ok, true, detail
}

// checkSignal returns (satisfied, evaluable, detail). Each requirement is
// judged against the window it asked for, never against whatever window
// happened to be read; a signal whose reading is Known false is not
// evaluable, with the provider's cause preserved in the detail.
func checkSignal(a *requirements.SignalAssertion, ev Evidence) (bool, bool, []string) {
	if a == nil {
		return false, false, nil
	}
	obs, have := ev.ObservedIn(a.Window.Std())
	if !have {
		return false, false, []string{fmt.Sprintf("no observed reading covers the %s window", a.Window.Std())}
	}
	sig, seen := obs.Signals[a.Kind]
	if !seen || !sig.Known {
		cause := sig.Cause
		if cause == "" {
			cause = "reading absent"
		}
		return false, false, []string{fmt.Sprintf("%s reading unavailable: %s", a.Kind, cause)}
	}

	if a.Present && !sig.Present {
		return false, true, []string{fmt.Sprintf("no %s received in the last %s", a.Kind, obs.Window)}
	}

	ok := true
	var detail []string
	if a.MinVolume > 0 && sig.Volume < a.MinVolume {
		ok = false
		detail = append(detail, fmt.Sprintf("%s volume %d is below the minimum of %d over %s",
			a.Kind, sig.Volume, a.MinVolume, obs.Window))
	}

	want := a.Coverage()
	for _, attr := range a.RequiredAttributes {
		got := sig.AttributeCoverage[attr]
		if got+1e-9 < want {
			ok = false
			detail = append(detail, fmt.Sprintf("attribute %q present on %.0f%% of %s records, need %.0f%%",
				attr, got*100, a.Kind, want*100))
		}
	}
	return ok, true, detail
}

func joinOrNone(v []string) string {
	if len(v) == 0 {
		return "none"
	}
	s := append([]string{}, v...)
	sort.Strings(s)
	return strings.Join(s, ", ")
}
