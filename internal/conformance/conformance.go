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

	"github.com/telecraft-dev/telecraft/internal/ownership"
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

	// Grade is the weight the finding carries, in the one grade vocabulary
	// the platform has (ownership.Grade). It answers a different question
	// from Outcome: Outcome is the diagnosis, Grade is whether the
	// diagnosis is allowed to fail the row.
	//
	// Every outcome the Effective × Observed cross produces is
	// violation-grade, which is why the zero value reads as violation:
	// an ungraded finding fails closed rather than slipping out of a
	// denominator. Schema conformance is the one producer that grades
	// below that, mapping the Schema Registry's own requirement levels
	// onto weights (ADR-0034 §3): required stays violation,
	// conditionally_required and recommended demote to advisory
	// (ADR-0034's "improvement"), and opt_in to neutral (its
	// "information"). Grade is never Pass here: whether a finding passed
	// is what Outcome says.
	Grade ownership.Grade

	// Coverage is the fraction of what the finding demanded that the
	// reading found, set on a schema-conformance finding at the
	// recommended level and nil everywhere else (ADR-0034 §3: a
	// recommended finding carries a coverage ratio rather than a bare
	// miss, because partial adoption is the normal state of a recommended
	// attribute and "3 of 8" is the actionable form of it). Nil means no
	// ratio was computed, which is not the same as a ratio of zero.
	Coverage *float64

	Waived       WaiverKind
	WaiverReason string

	// Detail explains the verdict in terms a human can act on.
	Detail []string

	// Remediation is the fix, where the evaluator can write a better one
	// than the requirement's author could. It is empty on every finding
	// the Effective × Observed cross produces, because there the authored
	// remediation on the Requirement is the whole answer: the requirement
	// asserts one fixed thing, so its author can say what closes it.
	//
	// A schema-conformance finding fills it (ADR-0034 §7). That
	// requirement is a reference to a scope rather than an assertion about
	// one attribute, so what is missing is only known at evaluation, and
	// the fix names the group, the attribute, its declared type and level,
	// and upstream's migration note where the registry carries one. A
	// surface reads this first and falls back to the authored line.
	Remediation string

	// Missing names the attributes a schema-conformance finding demanded
	// and the reading did not find, in stable order, and is empty on every
	// other finding. Detail says the same thing in a sentence; this is the
	// same fact in a form something other than a reader can use, which is
	// what lets an activation impact report say which attribute a Service
	// newly fails on rather than only that it newly fails (ADR-0034's
	// Consequences). Nothing here is a second judgement: the list is the
	// one the finding was built from.
	Missing []string
}

// Weight is the finding's grade with the zero value resolved: an ungraded
// finding is violation-grade, because everything the cross produces is.
func (f Finding) Weight() ownership.Grade {
	if f.Grade == "" {
		return ownership.Violation
	}
	return f.Grade
}

// Scored reports whether this finding feeds the row's binary. Only
// violation-grade findings do (ADR-0034 §3): improvement and information
// findings ride alongside, visible on the Service and counted in their own
// tallies, but the ratio and the worst-outcome badge are decided by
// violations alone. Promoting an improvement to a violation is deferred
// escalation (ADR-0022 §4) and is not a lever this package offers; the
// supported one is tightening the level in the Schema Registry.
func (f Finding) Scored() bool { return f.Weight() == ownership.Violation }

// Counts reports whether this finding counts against the row's score. A
// waived finding is still reported, still visible, and still diagnosed.
func (f Finding) Counts() bool { return f.Waived == WaiverNone }

// Failing reports a finding that both fails and counts: the unit the CI
// check mode gates on (REQ-024). A failing improvement never reaches it:
// the gate is violations alone.
func (f Finding) Failing() bool { return !f.Outcome.Passing() && f.Counts() && f.Scored() }

// CoverageRatio returns the finding's coverage ratio and whether one was
// computed.
func (f Finding) CoverageRatio() (float64, bool) {
	if f.Coverage == nil {
		return 0, false
	}
	return *f.Coverage, true
}

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

// Score is the counting summary for one row's verdict. Total, Passing,
// Waived and Failing count violation-grade findings alone, which is what
// makes the ratio and the badge a statement about the floor rather than
// about advice. Advisory and Neutral count the findings that ride alongside
// so that they are visible in a roll-up without moving a score (ADR-0034
// §3).
type Score struct {
	Total   int
	Passing int
	Waived  int
	Failing int

	// Advisory counts improvement-grade findings, Neutral information-grade
	// ones. Neither enters a denominator; both are reported, because a
	// finding nobody can see is a finding nobody acts on.
	Advisory int
	Neutral  int
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
		if !f.Scored() {
			switch f.Weight() {
			case ownership.Neutral:
				s.Neutral++
			default:
				s.Advisory++
			}
			continue
		}
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
// counting violation-grade finding: an improvement never darkens a badge
// (ADR-0034 §3). Counting findings always outrank waived ones, however
// severe the waived diagnosis: a waived problem has an owner and an expiry
// date, so it is already someone's business and should not outrank a live
// one. A verdict with no counting failure is compliant at worst.
func (v Verdict) Worst() Outcome {
	worst := Compliant
	for _, f := range v.Findings {
		if !f.Counts() || !f.Scored() {
			continue
		}
		if f.Outcome.Severity() > worst.Severity() {
			worst = f.Outcome
		}
	}
	return worst
}
