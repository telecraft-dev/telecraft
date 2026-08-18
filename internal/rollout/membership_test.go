package rollout

import (
	"fmt"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/renderer"
)

// fractionalRollout is a two-stage fractional rollout, 5% then 50%,
// hashing over host.name.
func fractionalRollout(stage int) renderer.Rollout {
	return renderer.Rollout{
		Name: "canary", Team: "pipelines", Tier: "pipelines/gateway",
		HashAttributes: []string{"host.name"},
		Stage:          stage,
		Stages: []renderer.RolloutStage{
			{Cohort: renderer.CohortSpec{Percent: 5}},
			{Cohort: renderer.CohortSpec{Percent: 50}},
		},
	}
}

// Membership is deterministic and pure (the first acceptance criterion):
// identical inputs yield identical membership on any call, and widening a
// fraction admits a strict superset — the bucket is fixed per collector,
// only the cut moves (ADR-0029 §4).
func TestFractionalMembershipIsDeterministicAndWidens(t *testing.T) {
	five, fifty := 0, 0
	for i := 0; i < 1000; i++ {
		attrs := map[string]string{"host.name": fmt.Sprintf("host-%03d", i), "other": "noise"}
		atFive := Member(fractionalRollout(0), attrs)
		atFifty := Member(fractionalRollout(1), attrs)
		if Member(fractionalRollout(0), attrs) != atFive {
			t.Fatalf("host %d: membership varies between identical calls", i)
		}
		if atFive && !atFifty {
			t.Fatalf("host %d is in the 5%% cohort but not the 50%% one — widening must be a strict superset, or collectors flap backwards (ADR-0029 §4)", i)
		}
		if atFive {
			five++
		}
		if atFifty {
			fifty++
		}
	}
	// Statistically 5%, not exactly 5% — accepted openly (§4). The counts
	// are deterministic, so loose bounds cannot flake.
	if five == 0 || fifty <= five || fifty >= 1000 {
		t.Errorf("cohort sizes five=%d fifty=%d out of 1000 — the fractions should carve real, growing, partial cohorts", five, fifty)
	}
	if five < 20 || five > 100 {
		t.Errorf("5%% of 1000 hosts admitted %d — far from the statistical target", five)
	}
}

// A collector missing a pinned hash attribute has no node-stable identity
// to hash: deterministically outside the fractional cohort, never randomly
// inside.
func TestMissingHashAttributeIsNeverFractionalMember(t *testing.T) {
	if Member(fractionalRollout(1), map[string]string{"region": "eu-west-1"}) {
		t.Error("a collector without the pinned attribute joined a fractional cohort")
	}
}

// The three spec forms are mixable per stage, membership their union, and
// stages accumulate: once admitted at stage N a collector stays admitted
// at every later stage (ADR-0029 §4).
func TestFormsUnionAndStagesAccumulate(t *testing.T) {
	r := renderer.Rollout{
		Name: "canary", Team: "pipelines", Tier: "pipelines/gateway",
		HashAttributes: []string{"host.name"},
		Stage:          0,
		Stages: []renderer.RolloutStage{
			{Cohort: renderer.CohortSpec{
				Hosts: &renderer.HostSet{Attribute: "host.name", Values: []string{"trusted-1", "trusted-2"}},
				Match: map[string]string{"region": "eu-west-1"},
			}},
			{Cohort: renderer.CohortSpec{Match: map[string]string{"region": "us-east-1"}}},
		},
	}

	enumerated := map[string]string{"host.name": "trusted-2"}
	byAttribute := map[string]string{"host.name": "any", "region": "eu-west-1"}
	laterStage := map[string]string{"host.name": "any", "region": "us-east-1"}

	if !Member(r, enumerated) || !Member(r, byAttribute) {
		t.Error("stage 0 membership is the union of its forms — both should be members")
	}
	if Member(r, laterStage) {
		t.Error("a stage-1 cohort member is admitted while stage 0 is active")
	}

	r.Stage = 1
	for name, attrs := range map[string]map[string]string{
		"enumerated": enumerated, "by attribute": byAttribute, "later stage": laterStage,
	} {
		if !Member(r, attrs) {
			t.Errorf("at stage 1 the %s member is out — advancing only ever widens", name)
		}
	}
}

// Preview runs the same function against an estate snapshot — information
// for the reviewer, never the authoritative decision (ADR-0029 §4). The
// population is scoped by the Tier's selector.
func TestPreviewScopesToTheTierPopulation(t *testing.T) {
	r := renderer.Rollout{
		Name: "canary", Team: "pipelines", Tier: "pipelines/gateway",
		Stage: 0,
		Stages: []renderer.RolloutStage{
			{Cohort: renderer.CohortSpec{Hosts: &renderer.HostSet{Attribute: "host.name", Values: []string{"a"}}}},
		},
	}
	tier := renderer.Tier{Name: "gateway", Team: "pipelines", Selector: map[string]string{"telecraft.tier": "gateway"}}
	est := estate.Estate{
		Declaration: estate.Declaration{RefreshCadence: time.Minute},
		Collectors: []estate.Collector{
			{Identity: map[string]string{"telecraft.tier": "gateway", "host.name": "a"}},
			{Identity: map[string]string{"telecraft.tier": "gateway", "host.name": "b"}},
			{Identity: map[string]string{"telecraft.tier": "edge", "host.name": "a"}},
		},
	}

	preview := Preview(r, tier, est)
	if len(preview) != 2 {
		t.Fatalf("previewed %d collectors, want the 2 in the Tier's population", len(preview))
	}
	members := 0
	for _, m := range preview {
		if m.Member {
			members++
			if m.Identity["host.name"] != "a" {
				t.Errorf("member %v is not the enumerated host", m.Identity)
			}
		}
	}
	if members != 1 {
		t.Errorf("%d members, want exactly the enumerated host", members)
	}
}
