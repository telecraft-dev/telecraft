// Package rollout is the staged-rollout machinery over the authored Rollout
// object (ADR-0029, REQ-043): cohort membership as a pure function, the
// advisory reading of the Foreign population, the halt/advance evaluation,
// and the platform-proposed advance and abort changes.
//
// Membership is never stored (§4): the server computes it per connect, CI
// and the console compute the same function against an estate snapshot as a
// preview: information for the reviewer, never the authoritative decision.
// Rejected alternatives (materialised membership lists in git) are rejected
// here too: nothing in this package persists anything.
//
// Halting is passive (§6): the evaluation returns a verdict, and a verdict
// that is not an advance or an abort proposes nothing, so there is no control
// loop to race. The halt-condition set is explicitly extensible: the two v1
// conditions ship here, and later signals (expectation regressions) plug in
// as further Conditions without amendment.
//
// The Foreign population reads everything and blocks nothing (§7): the
// advisory reading renders a member still on the *from* artefact as lag,
// never failure, because foreign delivery timing was never ours.
package rollout

import (
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"sort"

	"github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/renderer"
)

// buckets is the resolution of the fractional form: membership is judged
// per 1/100th of a percent, so integer percents are exact over the hash
// space (the population is still statistically 5%, never exactly 5%, per §4).
const buckets = 10000

// Member reports whether reported identifying attributes fall in the active
// cohort of r: the union of the stage cohorts up to and including the
// active stage, so advancing only ever widens and no collector flaps
// backwards (§4). A pure function of (rollout, attributes): identical
// inputs yield identical membership on any server instance, which is why N
// replicas need no coordination (ADR-0032 §2).
func Member(r renderer.Rollout, attrs map[string]string) bool {
	for i := 0; i <= r.Stage && i < len(r.Stages); i++ {
		if specMember(r.Stages[i].Cohort, r.HashAttributes, attrs) {
			return true
		}
	}
	return false
}

// specMember judges one stage's cohort spec: the three forms are mixable
// and membership is their union (§4).
func specMember(c renderer.CohortSpec, hashAttrs []string, attrs map[string]string) bool {
	if h := c.Hosts; h != nil {
		if v, ok := attrs[h.Attribute]; ok && slices.Contains(h.Values, v) {
			return true
		}
	}
	if len(c.Match) > 0 && satisfies(c.Match, attrs) {
		return true
	}
	if c.Percent > 0 {
		if b, ok := bucket(hashAttrs, attrs); ok && b < c.Percent*(buckets/100) {
			return true
		}
	}
	return false
}

// bucket hashes the pinned identifying attributes to [0, buckets). The
// pinned set and its authored order are part of the function's identity:
// changing either mid-rollout would reshuffle every fractional cohort,
// which is why the Rollout pins them (§4). Widening percent admits a
// strict superset: the bucket is fixed per collector, only the cut moves.
// A collector missing a pinned attribute has no node-stable identity to
// hash and is never a fractional member: deterministically outside, not
// randomly inside.
func bucket(keys []string, attrs map[string]string) (int, bool) {
	if len(keys) == 0 {
		return 0, false
	}
	h := sha256.New()
	for _, k := range keys {
		v, ok := attrs[k]
		if !ok || v == "" {
			return 0, false
		}
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(v))
		h.Write([]byte{0})
	}
	sum := h.Sum(nil)
	return int(binary.BigEndian.Uint64(sum[:8]) % buckets), true
}

// satisfies reports whether every asked pair equals the reported attribute,,
// the Tier selector's equality semantics (ADR-0007).
func satisfies(selector, attrs map[string]string) bool {
	for k, v := range selector {
		if attrs[k] != v {
			return false
		}
	}
	return true
}

// Membership is one collector's advisory membership line in a preview.
type Membership struct {
	Identity map[string]string
	Member   bool
}

// Preview computes membership for every collector of the target Tier's
// population in an estate reading, the preview-in-PR and console reading
// (§4): the same pure function the server evaluates per connect, run
// against a snapshot as information for the reviewer, never the
// authoritative decision. The population is the collectors satisfying the
// Tier's selector; a selector-less Tier (git-delivered only) has no
// platform-known population boundary, so every collector in the reading is
// previewed, and membership on the Foreign path is advisory either way (§7).
func Preview(r renderer.Rollout, tier renderer.Tier, est estate.Estate) []Membership {
	var out []Membership
	for _, c := range est.Collectors {
		if len(tier.Selector) > 0 && !satisfies(tier.Selector, c.Identity) {
			continue
		}
		out = append(out, Membership{Identity: c.Identity, Member: Member(r, c.Identity)})
	}
	sort.Slice(out, func(i, j int) bool {
		return estate.Fingerprint(out[i].Identity) < estate.Fingerprint(out[j].Identity)
	})
	return out
}
