package console

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/renderer"
)

// The rollout ledger over a hand-built estate reading. Every assertion is
// about the document carrying what internal/rollout decided: the counts,
// the verdict and the halts are its return values, and the point of the
// tests is that nothing here fills one in.

// The two artefacts a staged Tier renders. They wire different components,
// so an acknowledgement of one is never an acknowledgement of the other.
const (
	fromArtefact = "service:\n  pipelines:\n    traces:\n      receivers: [otlp]\n      exporters: [otlphttp]\n"
	toArtefact   = "service:\n  pipelines:\n    traces:\n      receivers: [otlp]\n      processors: [filter/noise-gate]\n      exporters: [otlphttp]\n"
)

// stamp is the hash the serving path offers an artefact under, which is
// what a collector's acknowledgement names.
func stamp(artefact string) string {
	sum := sha256.Sum256([]byte(artefact))
	return hex.EncodeToString(sum[:])
}

var rolloutNow = time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)

// applied is a fresh acknowledgement of one artefact.
func applied(hash string) *DeliveryStatusReading {
	return &DeliveryStatusReading{State: "APPLIED", ConfigHash: hash}
}

// bridge builds the ledger for one Rollout over the given population. The
// Rollout stages two boxes by name, then a quarter of the population.
func bridge(t *testing.T, collectors []CollectorReading) RolloutDoc {
	t.Helper()

	tier := renderer.Tier{
		Name:        "kafka-bridge",
		Team:        "data-flow",
		Owner:       "gateway-owners",
		Environment: "production",
		Selector:    map[string]string{"telecraft.tier": "kafka-bridge"},
	}
	r := renderer.Rollout{
		Name:           "bridge-canary",
		Team:           "data-flow",
		Owner:          "gateway-owners",
		Tier:           tier.ID(),
		From:           "data-flow/bridge-standard@2",
		To:             "data-flow/bridge-next@1",
		Stage:          1,
		HashAttributes: []string{"service.instance.id"},
		Stages: []renderer.RolloutStage{
			{
				Cohort: renderer.CohortSpec{
					Hosts: &renderer.HostSet{Attribute: "host.name", Values: []string{"bridge-a1", "bridge-a2"}},
				},
				Soak: renderer.Duration(2 * time.Hour),
			},
			{Cohort: renderer.CohortSpec{Percent: 25}, Soak: renderer.Duration(24 * time.Hour)},
			{Cohort: renderer.CohortSpec{Percent: 100}, Soak: renderer.Duration(24 * time.Hour)},
		},
	}

	b := &builder{
		in: Inputs{Commit: "1111111111111111111111111111111111111111"},
		topo: renderer.Topology{
			Tiers:    map[string]renderer.Tier{tier.ID(): tier},
			Rollouts: map[string]renderer.Rollout{r.ID(): r},
		},
		artefacts: map[string][]byte{
			renderer.ArtefactPath(tier):     []byte(fromArtefact),
			renderer.NextArtefactPath(tier): []byte(toArtefact),
		},
		now: rolloutNow,
	}
	views := map[string]*tierView{tier.ID(): {tier: tier, matched: collectors}}

	docs, err := b.rollouts(views)
	if err != nil {
		t.Fatalf("projecting the rollout ledger: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("rollouts = %d, want the one authored Rollout", len(docs))
	}
	return docs[0]
}

// member is one enumerated-cohort collector, so every test below controls
// membership by name rather than by hash.
func member(id string, edit func(*CollectorReading)) CollectorReading {
	c := CollectorReading{
		ID: id,
		Attributes: map[string]string{
			"telecraft.tier":      "kafka-bridge",
			"host.name":           id,
			"service.instance.id": id,
		},
		State:          "reporting",
		Delivery:       "served",
		LastSeen:       rolloutNow.Add(-30 * time.Second),
		AppliedAt:      rolloutNow.Add(-3 * time.Hour),
		DeliveryStatus: nil,
	}
	if edit != nil {
		edit(&c)
	}
	return c
}

// A member that acknowledged the candidate artefact is running it, and its
// apply instant is what the stage's soak is counted from.
func TestRolloutCountsAMemberOnTheCandidate(t *testing.T) {
	doc := bridge(t, []CollectorReading{
		member("bridge-a1", func(c *CollectorReading) { c.DeliveryStatus = applied(stamp(toArtefact)) }),
	})

	if doc.Evidence.MembersSeen != 1 || doc.Evidence.RunningTo != 1 {
		t.Errorf("evidence = %+v, want one member seen and one on the new version", doc.Evidence)
	}
	if doc.Evidence.Soaked != "3h" || doc.Evidence.MinSoak != "24h" {
		t.Errorf("soak = %q of %q, want 3h of 24h", doc.Evidence.Soaked, doc.Evidence.MinSoak)
	}
	if doc.Decision != "hold" {
		t.Errorf("decision = %q, want the advance held while the stage is short of its soak", doc.Decision)
	}
	if doc.Cohorts[0].Served.To != 1 {
		t.Errorf("stage 1 served split = %+v, want the member counted on the new version", doc.Cohorts[0].Served)
	}
}

// A member still on the previous artefact is lag: counted, and never a
// halt.
func TestRolloutCountsAMemberStillOnTheFrom(t *testing.T) {
	doc := bridge(t, []CollectorReading{
		member("bridge-a1", func(c *CollectorReading) { c.DeliveryStatus = applied(stamp(fromArtefact)) }),
	})

	if doc.Evidence.RunningFrom != 1 || doc.Evidence.RunningTo != 0 {
		t.Errorf("evidence = %+v, want the member on the previous version", doc.Evidence)
	}
	if len(doc.Halts) != 0 {
		t.Errorf("halts = %+v, want none: lag is never failure", doc.Halts)
	}
	// Nothing has taken the candidate, so no soak has been established.
	if doc.Evidence.Soaked != "0s" {
		t.Errorf("soaked = %q, want no soak counted before a member takes the new version", doc.Evidence.Soaked)
	}
}

// A member whose reading names no artefact is unknown. Not knowing is a
// normal state, and it counts towards nothing.
func TestRolloutLeavesAnUnacknowledgedMemberUnknown(t *testing.T) {
	doc := bridge(t, []CollectorReading{
		member("bridge-a1", nil),
		// A commit stamp names neither artefact, so it identifies nothing.
		member("bridge-a2", func(c *CollectorReading) {
			c.RunningSHA = "4f1b7c0d2e9a6b8c1d3e5f70a2b4c6d8e0f1a3b5"
		}),
	})

	if doc.Evidence.Unknown != 2 || doc.Evidence.RunningFrom != 0 || doc.Evidence.RunningTo != 0 {
		t.Errorf("evidence = %+v, want both members unknown", doc.Evidence)
	}
	if doc.Decision != "hold" {
		t.Errorf("decision = %q, want the advance held with no evidence", doc.Decision)
	}
}

// The running stamp names an artefact when it is one, abbreviated or whole:
// this is the field an estate already carries, and a rollout reads it.
func TestRolloutReadsAnAbbreviatedRunningStamp(t *testing.T) {
	doc := bridge(t, []CollectorReading{
		member("bridge-a1", func(c *CollectorReading) { c.RunningSHA = stamp(toArtefact)[:12] }),
	})

	if doc.Evidence.RunningTo != 1 {
		t.Errorf("evidence = %+v, want the abbreviated stamp read as the new version", doc.Evidence)
	}
}

// A member reporting FAILED for the candidate's hash halts the rollout, and
// the halt names the collector a reader recognises.
func TestRolloutHaltsOnAFailedApply(t *testing.T) {
	doc := bridge(t, []CollectorReading{
		member("bridge-a1", func(c *CollectorReading) { c.DeliveryStatus = applied(stamp(toArtefact)) }),
		member("bridge-a2", func(c *CollectorReading) {
			c.DeliveryStatus = &DeliveryStatusReading{
				State:      "FAILED",
				ConfigHash: stamp(toArtefact),
				Error:      "otlphttp exporter: connection refused",
			}
		}),
	})

	if len(doc.Halts) != 1 {
		t.Fatalf("halts = %+v, want the refused apply", doc.Halts)
	}
	halt := doc.Halts[0]
	if halt.Collector != "bridge-a2" || halt.Condition != "failed" || halt.Path != PathServed {
		t.Errorf("halt = %+v, want bridge-a2 failed on the served path", halt)
	}
	// One halted of two is at or past the abort threshold, so the abort is
	// what the evaluation proposes.
	if doc.Decision != "abort" {
		t.Errorf("decision = %q, want the abort proposed past the threshold", doc.Decision)
	}
}

// A member that took the candidate and then aged past the staleness horizon
// is the crash-loop signature that never reports FAILED.
func TestRolloutHaltsOnAMemberGoneDark(t *testing.T) {
	doc := bridge(t, []CollectorReading{
		member("bridge-a1", func(c *CollectorReading) { c.DeliveryStatus = applied(stamp(toArtefact)) }),
		member("bridge-a2", func(c *CollectorReading) {
			c.State = "stale"
			c.LastSeen = rolloutNow.Add(-2 * time.Hour)
			c.DeliveryStatus = applied(stamp(toArtefact))
		}),
	})

	if len(doc.Halts) != 1 || doc.Halts[0].Condition != "went_dark" {
		t.Fatalf("halts = %+v, want the member that went silent after applying", doc.Halts)
	}
	if doc.Halts[0].Collector != "bridge-a2" {
		t.Errorf("halt names %q, want bridge-a2", doc.Halts[0].Collector)
	}
}

// A collector that has never reported has no reading to age, so it is
// unknown rather than dark.
func TestRolloutNeverSeenMemberIsUnknownNotDark(t *testing.T) {
	doc := bridge(t, []CollectorReading{
		member("bridge-a1", func(c *CollectorReading) {
			c.State = "never_seen"
			c.LastSeen = time.Time{}
		}),
	})

	if doc.Evidence.Unknown != 1 {
		t.Errorf("evidence = %+v, want the member unknown", doc.Evidence)
	}
	if len(doc.Halts) != 0 {
		t.Errorf("halts = %+v, want none: a collector that never reported never went dark", doc.Halts)
	}
}

// On the git-delivered path delivery status is unavailable, so the member
// is read and counted, blocks nothing, and its silence is never a halt.
func TestRolloutForeignMemberIsAdvisory(t *testing.T) {
	doc := bridge(t, []CollectorReading{
		member("bridge-a1", func(c *CollectorReading) { c.DeliveryStatus = applied(stamp(toArtefact)) }),
		member("bridge-a2", func(c *CollectorReading) {
			c.Delivery = "git"
			c.State = "stale"
			c.LastSeen = rolloutNow.Add(-2 * time.Hour)
		}),
	})

	if doc.Cohorts[0].Foreign.Members != 1 || doc.Cohorts[0].Foreign.Unknown != 1 {
		t.Errorf("foreign split = %+v, want the member counted and unknown", doc.Cohorts[0].Foreign)
	}
	if doc.Cohorts[0].Served.Members != 1 {
		t.Errorf("served split = %+v, want only the served member", doc.Cohorts[0].Served)
	}
	if len(doc.Halts) != 0 {
		t.Errorf("halts = %+v, want none: the failure signal is unavailable on that path", doc.Halts)
	}
	if doc.Evidence.MembersSeen != 2 || doc.Evidence.RunningTo != 1 || doc.Evidence.Unknown != 1 {
		t.Errorf("evidence = %+v, want both members seen and only the served one on the new version", doc.Evidence)
	}
}

// Cohorts accumulate: each row is the union of the stages up to it, and a
// stage past the active one carries membership alone.
func TestRolloutCohortsAccumulateAndPreview(t *testing.T) {
	doc := bridge(t, []CollectorReading{
		member("bridge-a1", func(c *CollectorReading) { c.DeliveryStatus = applied(stamp(toArtefact)) }),
		member("bridge-a2", func(c *CollectorReading) { c.DeliveryStatus = applied(stamp(toArtefact)) }),
		member("bridge-b1", func(c *CollectorReading) { c.DeliveryStatus = applied(stamp(fromArtefact)) }),
	})

	if len(doc.Cohorts) != 3 {
		t.Fatalf("cohorts = %d, want one row per stage", len(doc.Cohorts))
	}
	states := []string{CohortEntered, CohortActive, CohortPending}
	for i, want := range states {
		if doc.Cohorts[i].State != want {
			t.Errorf("cohort %d state = %q, want %q", i, doc.Cohorts[i].State, want)
		}
	}
	if doc.Cohorts[0].Cohort != "host.name ∈ {bridge-a1, bridge-a2}" {
		t.Errorf("cohort spec = %q, want the enumerated hosts", doc.Cohorts[0].Cohort)
	}
	if doc.Cohorts[2].Cohort != "100% of the population" {
		t.Errorf("cohort spec = %q, want the fractional form", doc.Cohorts[2].Cohort)
	}
	// The last stage admits everyone, so it previews the whole population
	// and counts nothing running: no member of it has been offered the
	// candidate yet.
	last := doc.Cohorts[2]
	if last.Served.Members != 3 || last.Served.To != 0 || last.Served.Unknown != 0 {
		t.Errorf("pending cohort = %+v, want membership alone", last.Served)
	}
	if last.Widens != 3-(doc.Cohorts[1].Served.Members+doc.Cohorts[1].Foreign.Members) {
		t.Errorf("widens = %d, want the members this stage admits beyond the previous cohort", last.Widens)
	}
}

// The document carries the authored facts and the "why?" chain the panel
// reads, and its id is the one the ledger deep-links by.
func TestRolloutDocCarriesTheAuthoredFacts(t *testing.T) {
	doc := bridge(t, []CollectorReading{member("bridge-a1", nil)})

	if doc.ID != "data-flow/bridge-canary" || doc.Name != "bridge-canary" {
		t.Errorf("id = %q, name = %q, want the team-qualified Rollout id", doc.ID, doc.Name)
	}
	if doc.Tier != "data-flow/kafka-bridge" || doc.TierName != "kafka-bridge" || doc.Environment != "production" {
		t.Errorf("tier = %q/%q in %q, want the staged Tier", doc.Tier, doc.TierName, doc.Environment)
	}
	if doc.Owner != "gateway-owners" || doc.Stage != 1 {
		t.Errorf("owner = %q, stage = %d, want the authored owner and active stage", doc.Owner, doc.Stage)
	}
	keys := map[string]bool{}
	for _, p := range doc.Provenance {
		keys[p.Key] = true
	}
	if !keys["stage"] || !keys["bindings"] {
		t.Errorf("provenance keys = %v, want the stage and the bindings", keys)
	}
}

// An estate with no Rollout has an empty ledger, not a missing one.
func TestRolloutLedgerIsEmptyWithoutARollout(t *testing.T) {
	b := &builder{topo: renderer.Topology{Rollouts: map[string]renderer.Rollout{}}}
	docs, err := b.rollouts(map[string]*tierView{})
	if err != nil {
		t.Fatalf("projecting an empty ledger: %v", err)
	}
	if docs == nil || len(docs) != 0 {
		t.Errorf("rollouts = %v, want an empty list", docs)
	}
}
