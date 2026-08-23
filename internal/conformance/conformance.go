// Package conformance is the verdict cross: Effective × Observed, judged per
// requirement, producing the seven outcomes with their severity ordering
// (ADR-0004, REQ-020).
//
// The unit of evaluation is the row: one Service in one Environment
// (ADR-0033). Nothing in this package blends across environments: every
// Evaluate call judges one row against exactly the requirements that apply
// in its Environment, over evidence read for that Environment alone, so
// staging telemetry can never mask a production outage and one verdict never
// judges two configs. A Service simply has no row in an environment where it
// runs nothing. Absence of an environment is not a finding.
//
// The outcome set is deliberately larger than pass/fail, because "no logs
// arrived" is not one situation. A configured pipeline delivering nothing is
// a defect for the platform team; an unconfigured Service delivering nothing
// is a governance gap for the workload owner. The same score, two different
// people, two different fixes. Collapsing them would make the tool honest
// but useless. Waivers (exemption, grace; ADR-0037) are applied after the
// diagnosis, never instead of it: a waived finding keeps its outcome and its
// detail and gives up only its count.
//
// One outcome in the vocabulary never comes from the cross: library_drift is
// judged from the Intended reading (the config in git) by internal/drift
// (ADR-0004, ADR-0026). It lives here so every finding, whichever reading
// produced it, ranks on one severity ordering.
package conformance

import (
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
)

// Outcome is the diagnosis for one requirement on one row.
type Outcome string

const (
	// Compliant: the requirement is met.
	Compliant Outcome = "compliant"

	// NotConfigured: Effective says no, Observed says no. An unmet
	// requirement, and the owner needs to instrument.
	NotConfigured Outcome = "not_configured"

	// BrokenPipeline: Effective says yes, Observed says no. Someone
	// configured this and it is not working. The most valuable finding the
	// platform produces, and invisible to any tool that reads config alone.
	BrokenPipeline Outcome = "broken_pipeline"

	// NotDelivered: Observed says no, with no Effective evidence to explain
	// why. Honest about the limit of what one reading can tell you.
	NotDelivered Outcome = "not_delivered"

	// Ungoverned: Observed says yes, Effective says no. Telemetry is
	// arriving from something nobody configured. The requirement is met, so
	// this passes, but it is surfaced because an estate that cannot account
	// for its own data has a governance problem regardless of the score.
	Ungoverned Outcome = "ungoverned"

	// Misconfigured: a config assertion failed with no signal reading to
	// cross it against.
	Misconfigured Outcome = "misconfigured"

	// Unknown: no evidence available from any reading. Never silently
	// treated as a pass or a failure (ADR-0008: not knowing is a normal
	// state, and it is reported as itself).
	Unknown Outcome = "unknown"

	// LibraryDrift: the config in git passes the requirement version it
	// claims or pins but fails the current one: the goalposts moved and
	// the subject has not caught up (ADR-0026 §6). The one per-requirement
	// outcome the Effective × Observed cross never produces: it is judged
	// from the Intended reading by the drift detection (internal/drift,
	// ADR-0004), owned by the repo, and its remediation is the version
	// diff: review what moved and open a PR, never re-instrument.
	LibraryDrift Outcome = "library_drift"
)

// Outcomes returns the eight outcomes in severity order, worst first: the
// seven the cross produces (ADR-0004) plus library_drift, judged from the
// Intended reading (ADR-0026).
func Outcomes() []Outcome {
	return []Outcome{
		BrokenPipeline,
		NotConfigured,
		NotDelivered,
		Misconfigured,
		LibraryDrift,
		Unknown,
		Ungoverned,
		Compliant,
	}
}

func (o Outcome) Valid() bool {
	switch o {
	case Compliant, NotConfigured, BrokenPipeline, NotDelivered,
		Ungoverned, Misconfigured, Unknown, LibraryDrift:
		return true
	}
	return false
}

// Passing reports whether the outcome satisfies the requirement.
func (o Outcome) Passing() bool {
	return o == Compliant || o == Ungoverned
}

// Severity ranks outcomes by how much a human should care, highest first:
// the ordering behind every worst-first badge and shelf sort (ADR-0004,
// ADR-0017).
//
// Broken pipelines lead because they are the most actionable finding the
// platform produces: someone configured this with intent and it is silently
// not working, and nobody would otherwise know. Unknown outranks ungoverned
// because not being able to see is worse than seeing something unexpected.
// Library drift sits just below misconfigured: both fail the current
// assertion, but a drifted subject did comply once (the bar moved under it,
// ADR-0026 §6), which is a gentler diagnosis than never having complied.
func (o Outcome) Severity() int {
	switch o {
	case BrokenPipeline:
		return 7
	case NotConfigured:
		return 6
	case NotDelivered:
		return 5
	case Misconfigured:
		return 4
	case LibraryDrift:
		return 3
	case Unknown:
		return 2
	case Ungoverned:
		return 1
	default:
		return 0
	}
}

// WaiverKind records why a failing finding does not count, without erasing
// the finding itself. An exempted broken pipeline is still broken, and the
// platform keeps saying so (ADR-0004: waivers are applied after the
// diagnosis, never instead of it).
type WaiverKind string

const (
	WaiverNone   WaiverKind = ""
	WaiverExempt WaiverKind = "exempt"
	WaiverGrace  WaiverKind = "grace"
)

// Finding is one requirement's verdict on one row. Nothing in this slice
// sets Waived yet (the Exemption and Grace machinery arrives with the
// ADR-0037 work), but the counting seam exists now so the CI gate is honest
// about what it will and will not fail on.
type Finding struct {
	Requirement requirements.Requirement
	Outcome     Outcome

	Waived       WaiverKind
	WaiverReason string

	// Detail explains the verdict in terms a human can act on.
	Detail []string
}

// Counts reports whether this finding counts against the row's score. A
// waived finding is still reported, still visible, and still diagnosed.
func (f Finding) Counts() bool { return f.Waived == WaiverNone }

// Failing reports a finding that both fails and counts: the unit the CI
// check mode gates on (REQ-024).
func (f Finding) Failing() bool { return !f.Outcome.Passing() && f.Counts() }

// Row is the unit of conformance evaluation: one Service in one Environment
// (ADR-0033). Roll-ups count rows, and estate views default to the
// production lens.
type Row struct {
	// Service is the Service's service.name (ADR-0015).
	Service string

	Environment string
}

// Verdict is one row's evaluation: every applicable requirement judged
// against that row's evidence at one instant.
type Verdict struct {
	Row         Row
	EvaluatedAt time.Time
	Findings    []Finding
}

// Score is the counting summary for one row's verdict.
type Score struct {
	Total   int
	Passing int
	Waived  int
	Failing int
}

// Ratio is passing over counted. A row with every requirement waived scores
// 1.0, which is correct: nothing is known to be wrong. The waived count is
// reported alongside so that a perfect score built entirely on exemptions
// cannot hide (ADR-0017).
func (s Score) Ratio() float64 {
	counted := s.Total - s.Waived
	if counted <= 0 {
		return 1
	}
	return float64(s.Passing) / float64(counted)
}

// Score derives the counting summary from the findings, so a waiver applied
// after evaluation is reflected without re-judging anything.
func (v Verdict) Score() Score {
	var s Score
	for _, f := range v.Findings {
		s.Total++
		switch {
		case f.Outcome.Passing():
			s.Passing++
		case !f.Counts():
			s.Waived++
		default:
			s.Failing++
		}
	}
	return s
}

// Worst names the outcome a human should look at first: the highest-severity
// counting finding. Counting findings always outrank waived ones, however
// severe the waived diagnosis: a waived problem has an owner and an expiry
// date, so it is already someone's business and should not outrank a live
// one. A verdict with no counting failure is compliant at worst.
func (v Verdict) Worst() Outcome {
	worst := Compliant
	for _, f := range v.Findings {
		if !f.Counts() {
			continue
		}
		if f.Outcome.Severity() > worst.Severity() {
			worst = f.Outcome
		}
	}
	return worst
}
