package serving

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/renderer"
)

// rolloutFixture is the fixture estate with an active Rollout on the
// gateway Tier: a sibling candidate Blueprint (no batcher) and a
// two-stage Rollout whose first cohort enumerates the one canary host.
func rolloutFixture(t *testing.T) (root string, res renderer.Result) {
	root, _ = fixtureEstate(t)
	writeFile(t, root, "teams/pipelines/blueprints/flow-next.yaml", `
name: flow-next
version: 1
owner: pipelines-lead
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
    config:
      protocols:
        grpc: {}
  - name: out
    class: exporter
    type: otlphttp
    version: 1
    config:
      endpoint: https://gateway.internal:4318
pipelines:
  traces:
    - component: otlp-in
    - component: out
`)
	writeFile(t, root, "teams/pipelines/rollouts/canary.yaml", `
owner: pipelines-lead
tier: pipelines/gateway
from: pipelines/flow@1
to: pipelines/flow-next@1
stage: 0
hash_attributes: [host.name]
stages:
  - cohort:
      hosts:
        attribute: host.name
        values: [canary-1]
    soak: 24h
  - cohort:
      percent: 100
    soak: 24h
`)
	return root, renderFixture(t, root)
}

// On the served path, cohort members receive the new artefact and the
// remainder the old, both stamped at head (ADR-0029 §4): membership is a
// pure function of (head, attributes) computed per connect: same inputs,
// same serve, on any replica.
func TestMatchServesTheCohortTheNextArtefact(t *testing.T) {
	root, res := rolloutFixture(t)
	snap, err := LoadSnapshot(root, fixtureCommit)
	if err != nil {
		t.Fatal(err)
	}

	member := gatewayAttrs()
	member["host.name"] = "canary-1"
	remainder := gatewayAttrs()
	remainder["host.name"] = "steady-1"

	got := snap.Match(member)
	if got.Tier != "pipelines/gateway" || !got.Cohort || got.Rollout != "pipelines/canary" {
		t.Fatalf("member match = %+v, want the gateway Tier's cohort", got)
	}
	if !bytes.Equal(got.Artefact, res.Artefacts["rendered/pipelines/gateway@next.yaml"]) {
		t.Error("the cohort member is not served the @next artefact")
	}

	rest := snap.Match(remainder)
	if rest.Tier != "pipelines/gateway" || rest.Cohort {
		t.Fatalf("remainder match = %+v, want the gateway Tier outside the cohort", rest)
	}
	if rest.Rollout != "pipelines/canary" {
		t.Errorf("the remainder's match does not name the active rollout: %+v", rest)
	}
	if !bytes.Equal(rest.Artefact, res.Artefacts["rendered/pipelines/gateway.yaml"]) {
		t.Error("the remainder is not served the base artefact")
	}

	for name, artefact := range map[string][]byte{"base": rest.Artefact, "@next": got.Artefact} {
		if !strings.Contains(string(artefact), fixtureCommit) {
			t.Errorf("the served %s artefact carries no commit stamp", name) // ADR-0013
		}
	}

	// Pure and deterministic: the same attributes match the same artefact
	// on a rebuilt snapshot: nothing was remembered in between.
	again, err := LoadSnapshot(root, fixtureCommit)
	if err != nil {
		t.Fatal(err)
	}
	if rematch := again.Match(member); !rematch.Cohort || !bytes.Equal(rematch.Artefact, got.Artefact) {
		t.Error("membership varies across snapshot rebuilds: it must be a pure function of (head, attributes)") // ADR-0032 §2
	}
}

// A Tier with an active Rollout but no servable @next artefact refuses
// the snapshot: the server stays on the previous head. Staleness is
// bounded, and no cohort member is ever served a lie (ADR-0029 §3, ADR-0032 §1).
func TestSnapshotFailsClosedOnMissingNextArtefact(t *testing.T) {
	root, _ := rolloutFixture(t)
	if err := os.Remove(filepath.Join(root, "rendered", "pipelines", "gateway@next.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSnapshot(root, fixtureCommit); err == nil || !strings.Contains(err.Error(), "@next") {
		t.Fatalf("snapshot loaded without the @next artefact (err %v): it must fail closed", err)
	}
}
