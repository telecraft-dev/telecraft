package rollout

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/forge"
	"github.com/telecraft-dev/telecraft/internal/renderer"
)

// fakeForge records what was proposed.
type fakeForge struct {
	proposed []forge.Change
}

func (f *fakeForge) Name() string { return "fake/forge" }

func (f *fakeForge) Capabilities() forge.Capabilities {
	return forge.Capabilities{Proposals: true}
}

func (f *fakeForge) Propose(_ context.Context, c forge.Change) (forge.Proposal, error) {
	f.proposed = append(f.proposed, c)
	return forge.Proposal{ID: "1", URL: "https://forge.example/1", Branch: c.Branch}, nil
}

const proposeRolloutYAML = `# The canary rollout: comments survive the platform's edit.
owner: pipelines-lead
tier: pipelines/gateway
from: pipelines/flow@1
to: pipelines/flow-next@1
stage: 0
hash_attributes: [host.name]
stages:
  - cohort:
      percent: 5
    soak: 24h
  - cohort:
      percent: 50
    soak: 24h
`

const proposeTierYAML = `# The production gateway.
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
`

// proposeRoot writes the authored files an advance or abort edits.
func proposeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range map[string]string{
		"teams/pipelines/rollouts/canary.yaml": proposeRolloutYAML,
		"teams/pipelines/tiers/gateway.yaml":   proposeTierYAML,
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func proposeRollout(stage int) renderer.Rollout {
	r := fractionalRollout(stage)
	r.Owner = "pipelines-lead"
	r.From = "pipelines/flow@1"
	r.To = "pipelines/flow-next@1"
	return r
}

func proposeTier() renderer.Tier {
	return renderer.Tier{Name: "gateway", Team: "pipelines", Owner: "pipelines-lead"}
}

var proposeAuthor = forge.Identity{Name: "Robin Estate", Email: "robin@example.test"}

// renderOK is the render the submission flow runs (ADR-0028 §1).
func renderOK(context.Context) (map[string][]byte, error) {
	return map[string][]byte{"rendered/pipelines/gateway.yaml": []byte("service: {}\n")}, nil
}

// A mid-rollout advance is one reviewed edit of the Rollout file (the
// stage bump) on the deterministic branch (ADR-0029 §5, §8), submitted
// through the render-in-PR flow so the proposal carries the refreshed
// rendered tree.
func TestProposeAdvanceBumpsTheStage(t *testing.T) {
	f := &fakeForge{}
	v := Verdict{Decision: DecisionAdvance, Evidence: Evidence{Stage: 0, Stages: 2, MembersSeen: 3, RunningTo: 3}}

	p, proposed, err := Propose(context.Background(), f, renderOK, forge.Retry{}, proposeRoot(t), proposeRollout(0), proposeTier(), v, proposeAuthor)
	if err != nil {
		t.Fatal(err)
	}
	if !proposed || len(f.proposed) != 1 {
		t.Fatal("no proposal opened for an advance verdict")
	}
	if p.Branch != "telecraft/rollout/pipelines/canary/advance-2" {
		t.Errorf("branch = %q: deterministic names are what make racing replicas converge", p.Branch) // ADR-0029 §8
	}

	change := f.proposed[0]
	bumped := string(change.Files["teams/pipelines/rollouts/canary.yaml"])
	if !strings.Contains(bumped, "stage: 1") {
		t.Errorf("the rollout file was not bumped to stage 1:\n%s", bumped)
	}
	if !strings.Contains(bumped, "comments survive") {
		t.Errorf("the platform's edit rewrote the human-owned file:\n%s", bumped)
	}
	if _, touched := change.Files["teams/pipelines/tiers/gateway.yaml"]; touched {
		t.Error("a mid-rollout advance touched the Tier file")
	}
	if _, rendered := change.Files["rendered/pipelines/gateway.yaml"]; !rendered {
		t.Error("the proposal carries no refreshed rendered tree: it must go through the render-in-PR flow") // ADR-0028 §1
	}
	if !strings.Contains(change.Body, "3 running the to artefact") {
		t.Errorf("the body carries no evidence: %s", change.Body)
	}
}

// The final advance completes the Rollout (ADR-0029 §5): the Tier flips to
// single-bound *to*, the file is deleted, and the `@next` artefact retires
// with it in the same render.
func TestProposeFinalAdvanceCompletes(t *testing.T) {
	f := &fakeForge{}
	v := Verdict{Decision: DecisionAdvance, Evidence: Evidence{Stage: 1, Stages: 2, MembersSeen: 40, RunningTo: 40}}

	p, proposed, err := Propose(context.Background(), f, renderOK, forge.Retry{}, proposeRoot(t), proposeRollout(1), proposeTier(), v, proposeAuthor)
	if err != nil {
		t.Fatal(err)
	}
	if !proposed {
		t.Fatal("no proposal opened")
	}
	if p.Branch != "telecraft/rollout/pipelines/canary/advance-3" {
		t.Errorf("branch = %q", p.Branch)
	}

	change := f.proposed[0]
	if content, ok := change.Files["teams/pipelines/rollouts/canary.yaml"]; !ok || content != nil {
		t.Error("completion must delete the Rollout file: closed, the @next artefact retired")
	}
	tier := string(change.Files["teams/pipelines/tiers/gateway.yaml"])
	if !strings.Contains(tier, "blueprint: pipelines/flow-next@1") {
		t.Errorf("the Tier was not rebound to the to binding:\n%s", tier)
	}
	if !strings.Contains(tier, "The production gateway.") {
		t.Errorf("the platform's edit rewrote the human-owned Tier file:\n%s", tier)
	}
}

// The abort proposal deletes the Rollout file alone: the Tier's own
// binding never moved, so deleting the file reverts it to single-bound
// *from* (ADR-0029 §6).
func TestProposeAbortDeletesTheRollout(t *testing.T) {
	f := &fakeForge{}
	v := Verdict{
		Decision: DecisionAbort,
		Reason:   "2 of 10 cohort members halted",
		Evidence: Evidence{Stage: 0, Stages: 2, MembersSeen: 10, RunningTo: 8},
	}

	p, proposed, err := Propose(context.Background(), f, renderOK, forge.Retry{}, proposeRoot(t), proposeRollout(0), proposeTier(), v, proposeAuthor)
	if err != nil {
		t.Fatal(err)
	}
	if !proposed {
		t.Fatal("no proposal opened for an abort verdict")
	}
	if p.Branch != "telecraft/rollout/pipelines/canary/abort-1" {
		t.Errorf("branch = %q", p.Branch)
	}
	change := f.proposed[0]
	if content, ok := change.Files["teams/pipelines/rollouts/canary.yaml"]; !ok || content != nil {
		t.Error("the abort must delete the Rollout file")
	}
	if _, touched := change.Files["teams/pipelines/tiers/gateway.yaml"]; touched {
		t.Error("the abort touched the Tier file: the Tier's binding never moved")
	}
}

// Halting is passive (ADR-0029 §6): a hold or blocked verdict proposes
// nothing: no forge call, no active step, nothing to race.
func TestProposeIsPassiveOnHoldAndBlocked(t *testing.T) {
	for _, v := range []Verdict{
		{Decision: DecisionHold, Reason: "soaking"},
		{Decision: DecisionBlocked, Reason: "one member FAILED"},
	} {
		f := &fakeForge{}
		_, proposed, err := Propose(context.Background(), f, renderOK, forge.Retry{}, proposeRoot(t), proposeRollout(0), proposeTier(), v, proposeAuthor)
		if err != nil {
			t.Fatal(err)
		}
		if proposed || len(f.proposed) != 0 {
			t.Errorf("verdict %s opened a proposal: halting is the withheld proposal, never an action", v.Decision) // ADR-0029 §6
		}
	}
}
