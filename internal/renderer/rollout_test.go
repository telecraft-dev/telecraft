package renderer

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// scratchRolloutBlueprint is the base Blueprint of the scratch rollout
// estate; scratchRolloutCandidate is the sibling Blueprint a Rollout
// stages the Tier onto, with distinct content, so the dual artefacts differ.
const scratchRolloutBlueprint = `
name: flow
version: 1
owner: pipelines-lead
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: to-out
    class: exporter
    type: otlphttp
    version: 1
pipelines:
  traces:
    - component: otlp-in
    - component: to-out
`

const scratchRolloutCandidate = `
name: flow-next
version: 1
owner: pipelines-lead
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: batch
    class: processor
    type: batch
    version: 1
  - name: to-out
    class: exporter
    type: otlphttp
    version: 1
pipelines:
  traces:
    - component: otlp-in
    - component: batch
    - component: to-out
`

const scratchRollout = `
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

// rolloutEstate is the scratch estate with an active Rollout on the
// gateway Tier: base Blueprint, sibling candidate, one rollout file.
func rolloutEstate(t *testing.T) string {
	t.Helper()
	return scratchEstate(t, scratchRolloutBlueprint, scratchTier, map[string]string{
		"teams/pipelines/blueprints/flow-next.yaml": scratchRolloutCandidate,
		"teams/pipelines/rollouts/canary.yaml":      scratchRollout,
	})
}

// A Tier with an active Rollout is dual-bound (ADR-0029 §3): the render
// emits the base artefact from the *from* binding and `<tier>@next.yaml`
// from the *to* binding, both stamped at head, deterministically.
func TestRolloutRendersDualArtefacts(t *testing.T) {
	in := estateInputs(t, rolloutEstate(t))
	res, err := Render(in)
	if err != nil {
		t.Fatal(err)
	}

	base, ok := res.Artefacts["rendered/pipelines/gateway.yaml"]
	if !ok {
		t.Fatal("no base artefact rendered")
	}
	next, ok := res.Artefacts["rendered/pipelines/gateway@next.yaml"]
	if !ok {
		t.Fatal("no @next artefact rendered, but a dual-bound Tier renders both artefacts at head") // ADR-0029 §3
	}
	if bytes.Equal(base, next) {
		t.Error("base and @next artefacts are identical, so the rollout stages nothing")
	}
	// The candidate's processor renders as batch/batch, a discriminator
	// the base cannot carry (the self-telemetry emission uses a bare batch
	// processor in every artefact, so the bare word discriminates nothing).
	if !strings.Contains(string(next), "batch/batch") || strings.Contains(string(base), "batch/batch") {
		t.Error("the @next artefact should carry the candidate Blueprint's content and the base should not")
	}
	for name, artefact := range map[string][]byte{"base": base, "@next": next} {
		if !strings.Contains(string(artefact), fixtureCommit) {
			t.Errorf("the %s artefact carries no commit stamp, but both artefacts carry their own identity", name) // ADR-0013
		}
	}

	again, err := Render(estateInputs(t, rolloutEstate(t)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(next, again.Artefacts["rendered/pipelines/gateway@next.yaml"]) {
		t.Error("the @next artefact is not deterministic: identical inputs must produce byte-identical artefacts") // ADR-0028 §2
	}
}

// Without a Rollout no `@next` artefact exists: completion and abort both
// delete the rollout file, and the render retires the artefact with it
// (ADR-0029 §5, §6).
func TestNoRolloutNoNextArtefact(t *testing.T) {
	root := scratchEstate(t, scratchRolloutBlueprint, scratchTier, nil)
	res, err := Render(estateInputs(t, root))
	if err != nil {
		t.Fatal(err)
	}
	for rel := range res.Artefacts {
		if strings.Contains(rel, "@next") {
			t.Errorf("artefact %s rendered with no active Rollout", rel)
		}
	}
}

// The loaded Rollout carries its parsed bindings, stage arithmetic and
// soak durations.
func TestRolloutLoads(t *testing.T) {
	topo, err := LoadTopology(rolloutEstate(t))
	if err != nil {
		t.Fatal(err)
	}
	r, ok := topo.RolloutFor("pipelines/gateway")
	if !ok {
		t.Fatal("no rollout loaded for the gateway Tier")
	}
	if r.ID() != "pipelines/canary" || r.Owner != "pipelines-lead" {
		t.Errorf("rollout = %s owned by %s, want pipelines/canary owned by pipelines-lead", r.ID(), r.Owner)
	}
	if r.FromBinding().String() != "pipelines/flow@1" || r.ToBinding().String() != "pipelines/flow-next@1" {
		t.Errorf("bindings = %s → %s", r.FromBinding(), r.ToBinding())
	}
	if r.Final() {
		t.Error("stage 0 of 2 reads as final")
	}
	if time.Duration(r.Stages[0].Soak) != 24*time.Hour {
		t.Errorf("soak = %s, want 24h", time.Duration(r.Stages[0].Soak))
	}
}

// Everything that invalidates a Rollout fails the load: a rollout the
// render half-honoured would serve a population nobody reviewed.
func TestRolloutValidationFailsClosed(t *testing.T) {
	cases := []struct {
		name    string
		rollout string
		tier    string
		extra   map[string]string
		want    string
	}{
		{
			name: "unknown tier",
			rollout: `
owner: pipelines-lead
tier: pipelines/absent
from: pipelines/flow@1
to: pipelines/flow-next@1
stages:
  - cohort: {percent: 5}
hash_attributes: [host.name]
`,
			want: "not an authored Tier",
		},
		{
			name: "direct rebind while active",
			rollout: `
owner: pipelines-lead
tier: pipelines/gateway
from: pipelines/flow@1
to: pipelines/flow-next@1
stages:
  - cohort: {percent: 5}
hash_attributes: [host.name]
`,
			tier: `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@2
`,
			want: "the Rollout file is the only way to rebind",
		},
		{
			name: "same blueprint on both sides",
			rollout: `
owner: pipelines-lead
tier: pipelines/gateway
from: pipelines/flow@1
to: pipelines/flow@2
stages:
  - cohort: {percent: 5}
hash_attributes: [host.name]
`,
			want: "same Blueprint",
		},
		{
			name: "owner is not the Tier's owner",
			rollout: `
owner: somebody-else
tier: pipelines/gateway
from: pipelines/flow@1
to: pipelines/flow-next@1
stages:
  - cohort: {percent: 5}
hash_attributes: [host.name]
`,
			want: "a Rollout's owner is the Tier's owner",
		},
		{
			name: "fractional without pinned hash attributes",
			rollout: `
owner: pipelines-lead
tier: pipelines/gateway
from: pipelines/flow@1
to: pipelines/flow-next@1
stages:
  - cohort: {percent: 5}
`,
			want: "hash_attributes",
		},
		{
			name: "stage out of range",
			rollout: `
owner: pipelines-lead
tier: pipelines/gateway
from: pipelines/flow@1
to: pipelines/flow-next@1
stage: 2
stages:
  - cohort: {percent: 5}
  - cohort: {percent: 50}
hash_attributes: [host.name]
`,
			want: "stage 2 of 2",
		},
		{
			name: "empty cohort spec",
			rollout: `
owner: pipelines-lead
tier: pipelines/gateway
from: pipelines/flow@1
to: pipelines/flow-next@1
stages:
  - cohort: {}
`,
			want: "empty cohort spec",
		},
		{
			name:    "second rollout on the same Tier",
			rollout: scratchRollout,
			extra: map[string]string{
				"teams/pipelines/rollouts/second.yaml": scratchRollout,
			},
			want: "one active Rollout per Tier",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier := scratchTier
			if tc.tier != "" {
				tier = tc.tier
			}
			extra := map[string]string{
				"teams/pipelines/blueprints/flow-next.yaml": scratchRolloutCandidate,
				"teams/pipelines/rollouts/canary.yaml":      tc.rollout,
			}
			for rel, content := range tc.extra {
				extra[rel] = content
			}
			root := scratchEstate(t, scratchRolloutBlueprint, tier, extra)
			_, err := LoadTopology(root)
			if err == nil {
				t.Fatal("load succeeded, but the problem must fail the load")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error does not name the problem %q:\n%v", tc.want, err)
			}
		})
	}
}
