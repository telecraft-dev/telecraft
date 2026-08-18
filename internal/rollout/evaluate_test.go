package rollout

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/renderer"
)

var evalNow = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

// evalTier is the target Tier: a selector so the population is scoped.
func evalTier() renderer.Tier {
	return renderer.Tier{
		Name: "gateway", Team: "pipelines", Owner: "pipelines-lead",
		Selector: map[string]string{"telecraft.tier": "gateway"},
	}
}

// evalRollout encloses hosts member-0..member-(n-1) in the stage-0 cohort.
func evalRollout(members int) renderer.Rollout {
	values := make([]string, members)
	for i := range values {
		values[i] = fmt.Sprintf("member-%d", i)
	}
	return renderer.Rollout{
		Name: "canary", Team: "pipelines", Tier: "pipelines/gateway",
		From: "pipelines/flow@1", To: "pipelines/flow-next@1",
		Stage: 0,
		Stages: []renderer.RolloutStage{
			{Cohort: renderer.CohortSpec{Hosts: &renderer.HostSet{Attribute: "host.name", Values: values}},
				Soak: renderer.Duration(24 * time.Hour)},
			{Cohort: renderer.CohortSpec{Hosts: &renderer.HostSet{Attribute: "host.name", Values: values}}},
		},
	}
}

// collector builds one member reading. fresh readings are stamped at
// evalNow; a stale one at evalNow minus an hour, far past the horizon of
// the declared one-minute cadence.
func collector(host string, running Running, remote estate.DeliveryStatus, fresh bool, from, to Artefact) estate.Collector {
	asOf := evalNow
	if !fresh {
		asOf = evalNow.Add(-time.Hour)
	}
	c := estate.Collector{
		Identity: map[string]string{"telecraft.tier": "gateway", "host.name": host},
	}
	switch running {
	case RunningTo:
		c.Effective = estate.Effective{Known: true, AsOf: asOf, Pipelines: to.Pipelines}
	case RunningFrom:
		c.Effective = estate.Effective{Known: true, AsOf: asOf, Pipelines: from.Pipelines}
	}
	remote.AsOf = asOf
	c.DeliveryStatus = remote
	return c
}

func applied(hash []byte) estate.DeliveryStatus {
	return estate.DeliveryStatus{Known: true, State: estate.DeliveryApplied, ConfigHash: hash}
}

func evalInputs(t *testing.T, members int, collectors []estate.Collector, soaked time.Duration) Inputs {
	t.Helper()
	from, to := artefacts(t)
	return Inputs{
		Rollout: evalRollout(members),
		Tier:    evalTier(),
		Estate: estate.Estate{
			Declaration: estate.Declaration{
				Readings: map[estate.ReadingKind]bool{
					estate.EffectiveKind: true, estate.HealthKind: true, estate.DeliveryStatusKind: true,
				},
				RefreshCadence: time.Minute,
			},
			AsOf:       evalNow,
			Collectors: collectors,
		},
		From: from, To: to,
		StageStarted: evalNow.Add(-soaked),
		Now:          evalNow,
	}
}

// The soaked, fully applied stage advances; a member still on from is lag
// and never blocks (ADR-0029 §5, §7).
func TestEvaluateAdvancesWithLagNeverBlocking(t *testing.T) {
	from, to := artefacts(t)
	in := evalInputs(t, 3, []estate.Collector{
		collector("member-0", RunningTo, applied(to.Hash), true, from, to),
		collector("member-1", RunningTo, applied(to.Hash), true, from, to),
		// Still on from with no delivery reading: the Foreign leg — lag,
		// never failure (ADR-0029 §7).
		collector("member-2", RunningFrom, estate.DeliveryStatus{Cause: "the provider cannot report delivery"}, true, from, to),
		// Outside the cohort entirely.
		collector("bystander", RunningFrom, applied(from.Hash), true, from, to),
	}, 25*time.Hour)

	v, err := Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if v.Decision != DecisionAdvance {
		t.Fatalf("decision = %s (%s), want advance", v.Decision, v.Reason)
	}
	e := v.Evidence
	if e.MembersSeen != 3 || e.RunningTo != 2 || e.RunningFrom != 1 {
		t.Errorf("evidence = %+v, want 3 members, 2 on to, 1 lagging on from", e)
	}
	if !strings.Contains(e.Summary(), "lag, never failure") {
		t.Errorf("the evidence does not display the lag: %s", e.Summary())
	}
}

// An unelapsed soak holds; so does a cohort nothing has been observed
// running the to artefact in — evidence is computed over collectors
// actually running it (ADR-0029 §5, §7). Holding proposes nothing:
// waiting is the resting state.
func TestEvaluateHolds(t *testing.T) {
	from, to := artefacts(t)

	in := evalInputs(t, 2, []estate.Collector{
		collector("member-0", RunningTo, applied(to.Hash), true, from, to),
	}, time.Hour)
	v, err := Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if v.Decision != DecisionHold || !strings.Contains(v.Reason, "soaked") {
		t.Errorf("decision = %s (%s), want a soak hold", v.Decision, v.Reason)
	}

	in = evalInputs(t, 2, []estate.Collector{
		collector("member-0", RunningFrom, applied(from.Hash), true, from, to),
	}, 25*time.Hour)
	v, err = Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if v.Decision != DecisionHold || !strings.Contains(v.Reason, "actually running") {
		t.Errorf("decision = %s (%s), want a no-evidence hold", v.Decision, v.Reason)
	}
}

// One FAILED for the to artefact's hash below the abort threshold blocks
// the advance — passively: the verdict is the withheld proposal, no active
// step exists (ADR-0029 §6). A FAILED for some other hash is not this
// rollout's signal.
func TestEvaluateBlocksOnFailedForTo(t *testing.T) {
	from, to := artefacts(t)
	var cs []estate.Collector
	for i := 0; i < 11; i++ {
		cs = append(cs, collector(fmt.Sprintf("member-%d", i), RunningTo, applied(to.Hash), true, from, to))
	}
	cs = append(cs, collector("member-11", RunningFrom, estate.DeliveryStatus{
		Known: true, State: estate.DeliveryFailed, ConfigHash: to.Hash, Error: "collector crash-looped",
	}, true, from, to))

	v, err := Evaluate(evalInputs(t, 12, cs, 25*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if v.Decision != DecisionBlocked {
		t.Fatalf("decision = %s (%s), want blocked", v.Decision, v.Reason)
	}
	if len(v.Evidence.Halted) != 1 || v.Evidence.Halted[0].Condition != "failed" {
		t.Errorf("halted = %+v, want the one failed member", v.Evidence.Halted)
	}

	// The same FAILED against an unrelated hash halts nothing.
	cs[11] = collector("member-11", RunningFrom, estate.DeliveryStatus{
		Known: true, State: estate.DeliveryFailed, ConfigHash: []byte("some other artefact"),
	}, true, from, to)
	v, err = Evaluate(evalInputs(t, 12, cs, 25*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if v.Decision != DecisionAdvance {
		t.Errorf("decision = %s (%s) — a FAILED for another artefact's hash is not this rollout's halt signal", v.Decision, v.Reason)
	}
}

// Past the threshold — default: 10% of the seen cohort failed or dark —
// the verdict is the abort proposal (ADR-0029 §6). Went-dark-after-apply
// is the crash-loop signature that never reports FAILED: last seen running
// the to artefact, silent past the staleness horizon.
func TestEvaluateAbortsPastThresholdWithDarkMembers(t *testing.T) {
	from, to := artefacts(t)
	var cs []estate.Collector
	for i := 0; i < 8; i++ {
		cs = append(cs, collector(fmt.Sprintf("member-%d", i), RunningTo, applied(to.Hash), true, from, to))
	}
	cs = append(cs,
		collector("member-8", RunningTo, applied(to.Hash), false, from, to), // went dark
		collector("member-9", RunningFrom, estate.DeliveryStatus{
			Known: true, State: estate.DeliveryFailed, ConfigHash: to.Hash,
		}, true, from, to),
	)

	v, err := Evaluate(evalInputs(t, 10, cs, 25*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if v.Decision != DecisionAbort {
		t.Fatalf("decision = %s (%s), want abort at 2/10 failed-or-dark", v.Decision, v.Reason)
	}
	conditions := map[string]bool{}
	for _, h := range v.Evidence.Halted {
		conditions[h.Condition] = true
	}
	if !conditions["failed"] || !conditions["went_dark"] {
		t.Errorf("halted = %+v, want both v1 halt signals present", v.Evidence.Halted)
	}
}

// The halt-condition set is explicitly extensible (ADR-0029 §6): a caller
// can wire additional signals without amendment.
func TestEvaluateAcceptsExtensibleConditions(t *testing.T) {
	from, to := artefacts(t)
	in := evalInputs(t, 1, []estate.Collector{
		collector("member-0", RunningTo, applied(to.Hash), true, from, to),
	}, 25*time.Hour)
	in.Conditions = append(DefaultConditions(in.To.Hash), Condition{
		Name: "expectation_regression",
		Halt: func(o Observation) (string, bool) {
			return "applied fine, traces stopped", true
		},
	})

	v, err := Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if v.Decision != DecisionAbort {
		t.Fatalf("decision = %s, want abort — the added condition halts every member", v.Decision)
	}
	if v.Evidence.Halted[0].Condition != "expectation_regression" {
		t.Errorf("halted = %+v, want the added condition named", v.Evidence.Halted)
	}
}
