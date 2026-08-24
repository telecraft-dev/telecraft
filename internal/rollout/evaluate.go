package rollout

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/renderer"
)

// Observation is one cohort member as the evaluation sees it: identity,
// which artefact it runs, its raw remote reading, and whether it has gone
// silent past the staleness horizon.
type Observation struct {
	Identity map[string]string
	Running  Running

	// Remote is the member's RemoteConfigStatus reading, verbatim
	// (ADR-0004). Where delivery status is permanently unavailable
	// (Foreign paths, ADR-0008) it is Known false. The FAILED signal is
	// honestly unavailable there (ADR-0029 §7), and its absence never
	// reads as failure.
	Remote estate.DeliveryStatus

	// Silent: the member reported once, but every reading its provider is
	// capable of has aged past the staleness horizon (ADR-0036 §3).
	Silent bool
}

// Condition is one halt signal over a cohort member (ADR-0029 §6). The set
// is explicitly extensible: later signals, such as expectation regressions
// ("applied fine, traces stopped"), plug in as further Conditions without
// amendment.
type Condition struct {
	Name string

	// Halt returns the reason this observation halts the rollout, if it
	// does.
	Halt func(o Observation) (string, bool)
}

// FailedForTo is v1 halt signal (a): a cohort member reporting FAILED for
// the *to* artefact's hash. It took the offer, the apply failed, the
// Supervisor has already self-reverted it (ADR-0010) and the report is the
// evidence. A FAILED for any other hash is some other delivery's problem,
// never this rollout's.
func FailedForTo(toHash []byte) Condition {
	return Condition{
		Name: "failed",
		Halt: func(o Observation) (string, bool) {
			r := o.Remote
			if r.Known && r.State == estate.DeliveryFailed && bytes.Equal(r.ConfigHash, toHash) {
				return "reported FAILED for the new version: " + orNone(r.Error), true
			}
			return "", false
		},
	}
}

// WentDarkAfterApply is v1 halt signal (b): the member was reporting, took
// the new config, and went silent within the soak window, the crash-loop
// signature that never reports FAILED (ADR-0029 §6).
func WentDarkAfterApply() Condition {
	return Condition{
		Name: "went_dark",
		Halt: func(o Observation) (string, bool) {
			if o.Silent && o.Running == RunningTo {
				return "took the new version, then went silent past the staleness horizon", true
			}
			return "", false
		},
	}
}

// DefaultConditions is the v1 halt-condition set (ADR-0029 §6).
func DefaultConditions(toHash []byte) []Condition {
	return []Condition{FailedForTo(toHash), WentDarkAfterApply()}
}

// Thresholds is the configurable halt policy (ADR-0029 §6). The defaults
// are the ADR's: any halted cohort member blocks the advance; at or past
// AbortFraction of the seen cohort halted, the abort is proposed.
type Thresholds struct {
	// AbortFraction is the halted share of the seen cohort at which the
	// evaluation proposes the abort. Zero means the default, 0.10.
	AbortFraction float64
}

// DefaultThresholds returns the ADR-0029 §6 defaults.
func DefaultThresholds() Thresholds { return Thresholds{AbortFraction: 0.10} }

func (t Thresholds) abortFraction() float64 {
	if t.AbortFraction <= 0 {
		return DefaultThresholds().AbortFraction
	}
	return t.AbortFraction
}

// Decision is the evaluation's verdict on one stage.
type Decision string

const (
	// DecisionHold: criteria not yet met: soak still running, or no
	// evidence yet. Nothing is proposed and nothing needs to be: waiting
	// is the resting state.
	DecisionHold Decision = "hold"

	// DecisionBlocked: a halt signal is present below the abort threshold.
	// The advance is simply never proposed, because halting is passive, there is
	// no active step and nothing to race (ADR-0029 §6).
	DecisionBlocked Decision = "blocked"

	// DecisionAdvance: the stage's exit criteria are met, so propose the
	// advance for a human to merge (ADR-0029 §5).
	DecisionAdvance Decision = "advance"

	// DecisionAbort: halted past the threshold, so propose the abort,
	// reverting the Tier to single-bound *from* (ADR-0029 §6).
	DecisionAbort Decision = "abort"
)

// Halt is one halted member in the evidence.
type Halt struct {
	Identity  string // estate.Fingerprint of the member
	Condition string
	Reason    string
}

// Evidence is what the verdict rests on: the numbers the advance or abort
// proposal carries in its body ("soaked 24h, 213/213 APPLIED, 0 FAILED",
// ADR-0029 §5), computed over collectors actually running the *to*
// artefact on either path. Members still on *from* are lag: displayed,
// never blocking (§7).
type Evidence struct {
	Stage  int // active stage, 0-based
	Stages int

	// MembersSeen is the cohort members present in the estate reading.
	MembersSeen int

	RunningTo, RunningFrom, RunningOther, Unknown int

	Halted []Halt

	// Soaked is how long the stage has been active; MinSoak the stage's
	// authored floor.
	Soaked  time.Duration
	MinSoak time.Duration
}

// Summary renders the evidence as one line for proposal bodies and logs.
func (e Evidence) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "stage %d of %d soaked %s of the %s minimum: %s seen, %d running the new version, %d halted",
		e.Stage+1, e.Stages, e.Soaked.Round(time.Minute), e.MinSoak, members(e.MembersSeen), e.RunningTo, len(e.Halted))
	if e.RunningFrom > 0 {
		fmt.Fprintf(&b, "; %d still on the previous version", e.RunningFrom)
	}
	if e.RunningOther > 0 {
		fmt.Fprintf(&b, "; %d on another configuration", e.RunningOther)
	}
	if e.Unknown > 0 {
		fmt.Fprintf(&b, "; %d unknown", e.Unknown)
	}
	return b.String()
}

// Verdict is one evaluation's outcome.
type Verdict struct {
	Decision Decision

	// Reason is the one-line why.
	Reason string

	Evidence Evidence
}

// Inputs is everything one evaluation reads. Time arrives as data.
// StageStarted is the commit instant of the change that activated the
// stage (git history answers it), Now is the caller's clock, so the
// evaluation itself stays a pure function, recomputable anywhere.
type Inputs struct {
	Rollout renderer.Rollout
	Tier    renderer.Tier

	// Estate is the estate reading the evidence is computed over.
	Estate estate.Estate

	// From and To are the Tier's two rendered artefacts at head.
	From, To Artefact

	StageStarted time.Time
	Now          time.Time

	Thresholds Thresholds

	// Conditions is the halt-condition set; nil means
	// DefaultConditions(To.Hash).
	Conditions []Condition
}

// Evaluate judges the active stage (ADR-0029 §5, §6): a pure function of
// its inputs, so racing replicas evaluating the same head and reading
// reach the same verdict. It never proposes anything itself. Propose acts
// on the verdict, and a verdict that is not an advance or abort proposes
// nothing at all, which is what makes halting passive.
func Evaluate(in Inputs) (Verdict, error) {
	r := in.Rollout
	if len(r.Stages) == 0 || r.Stage < 0 || r.Stage >= len(r.Stages) {
		return Verdict{}, fmt.Errorf("rollout %s has no valid active stage (%d of %d)", r.ID(), r.Stage, len(r.Stages))
	}
	conditions := in.Conditions
	if conditions == nil {
		conditions = DefaultConditions(in.To.Hash)
	}

	stage := r.Stages[r.Stage]
	ev := Evidence{
		Stage:   r.Stage,
		Stages:  len(r.Stages),
		Soaked:  in.Now.Sub(in.StageStarted),
		MinSoak: time.Duration(stage.Soak),
	}

	halted := map[string]bool{}
	for _, c := range in.Estate.Collectors {
		if len(in.Tier.Selector) > 0 && !satisfies(in.Tier.Selector, c.Identity) {
			continue
		}
		if !Member(r, c.Identity) {
			continue
		}
		ev.MembersSeen++
		o := Observation{
			Identity: c.Identity,
			Running:  RunningArtefact(c, in.From, in.To),
			Remote:   c.DeliveryStatus,
			Silent:   silent(c, in.Estate.Declaration, in.Now),
		}
		switch o.Running {
		case RunningTo:
			ev.RunningTo++
		case RunningFrom:
			ev.RunningFrom++
		case RunningOther:
			ev.RunningOther++
		default:
			ev.Unknown++
		}
		key := estate.Fingerprint(o.Identity)
		for _, cond := range conditions {
			if reason, halt := cond.Halt(o); halt {
				ev.Halted = append(ev.Halted, Halt{Identity: key, Condition: cond.Name, Reason: reason})
				halted[key] = true
			}
		}
	}

	switch {
	case ev.MembersSeen > 0 && float64(len(halted))/float64(ev.MembersSeen) >= in.Thresholds.abortFraction():
		return Verdict{
			Decision: DecisionAbort,
			Reason:   fmt.Sprintf("%d of %s halted, at or past the abort threshold. Telecraft proposes returning the Tier to the previous version.", len(halted), members(ev.MembersSeen)),
			Evidence: ev,
		}, nil
	case len(halted) > 0:
		return Verdict{
			Decision: DecisionBlocked,
			Reason:   fmt.Sprintf("%s halted, below the abort threshold. Telecraft holds the advance.", members(len(halted))),
			Evidence: ev,
		}, nil
	case ev.Soaked < ev.MinSoak:
		return Verdict{
			Decision: DecisionHold,
			Reason:   fmt.Sprintf("The stage has soaked %s of its %s minimum.", ev.Soaked.Round(time.Minute), ev.MinSoak),
			Evidence: ev,
		}, nil
	case ev.RunningTo == 0:
		return Verdict{
			Decision: DecisionHold,
			Reason:   "No cohort member is running the new version yet. Telecraft holds the advance until one does.",
			Evidence: ev,
		}, nil
	}
	return Verdict{
		Decision: DecisionAdvance,
		Reason:   "Every condition is met: " + ev.Summary(),
		Evidence: ev,
	}, nil
}

// silent reports whether a collector that was reporting has aged past the
// staleness horizon on every reading it carried: last seen, and no longer
// fresh (ADR-0036 §3). A collector with no known reading at all is simply
// unknown, never silent, never dark.
func silent(c estate.Collector, decl estate.Declaration, now time.Time) bool {
	hadKnown := c.Effective.Known || c.Health.Known || c.DeliveryStatus.Known
	if !hadKnown {
		return false
	}
	demoted := c.ForEvaluation(decl, now)
	return !demoted.Effective.Known && !demoted.Health.Known && !demoted.DeliveryStatus.Known
}

// members renders a cohort-member count for a reader: one member is
// never "1 member(s)".
func members(n int) string {
	if n == 1 {
		return "1 cohort member"
	}
	return fmt.Sprintf("%d cohort members", n)
}

func orNone(s string) string {
	if s == "" {
		return "(no error detail reported)"
	}
	return s
}
