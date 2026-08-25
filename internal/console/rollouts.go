package console

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/internal/rollout"
)

// The rollout ledger's documents (ADR-0029). Nothing here decides anything
// about a rollout: membership is internal/rollout's pure function, which
// artefact a member runs is internal/rollout's advisory reading, and the
// verdict, its evidence and every halt are internal/rollout's evaluation.
// This file loads the two artefacts, shapes the declared collector estate
// into the EstateProvider seam the evaluation reads, and projects what
// comes back.
//
// The two readings the evaluation needs and a repository cannot hold
// arrive the way every other runtime reading does (see Readings): the
// estate declares them and this package plays them back through the seam.

// collectorRefreshCadence is the cadence the declared collector estate is
// played back under: the OpAMP default heartbeat, the interval at which a
// quiet collector re-affirms its readings (matching the direct provider's
// own declaration). Freshness is the platform's arithmetic, never the
// reading's claim (ADR-0036 §3), so a declared reading ages exactly as a
// live one does: past the horizon it demotes to Known false, and a member
// that took the candidate artefact and then aged out is the went-dark
// signal ADR-0029 §6 halts on.
const collectorRefreshCadence = 30 * time.Second

// minStampHex is the shortest acknowledged-configuration stamp a reading
// may abbreviate to. Below it a stamp names too little to identify an
// artefact, and a guess is worse than an unknown.
const minStampHex = 8

// rollouts projects every authored Rollout's cohort progress. The order is
// by Rollout id, so the ledger is stable across snapshots.
func (b *builder) rollouts(views map[string]*tierView) ([]RolloutDoc, error) {
	ids := make([]string, 0, len(b.topo.Rollouts))
	for id := range b.topo.Rollouts {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := []RolloutDoc{}
	for _, id := range ids {
		r := b.topo.Rollouts[id]
		v := views[r.Tier]
		if v == nil {
			return nil, fmt.Errorf("rollout %q stages tier %q, which the topology does not hold", id, r.Tier)
		}
		doc, err := b.rolloutDoc(r, v)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, nil
}

// rolloutMember is one collector of the staged Tier's population as the
// rollout reading sees it: the seam reading the evaluation judges, plus the
// two facts the ledger presents it by.
type rolloutMember struct {
	// id names the collector in the ledger and in a halt line. Identity
	// for matching is the reading's own attributes (ADR-0013).
	id string

	// foreign marks the git-delivered path, whose members are advisory:
	// displayed, never blocking (ADR-0029 §7).
	foreign bool

	// applied is when the collector reports its running artefact went
	// APPLIED, the only instant in the reading that dates the stage.
	applied time.Time

	reading estate.Collector
}

// rolloutDoc assembles one Rollout's ledger document.
func (b *builder) rolloutDoc(r renderer.Rollout, v *tierView) (RolloutDoc, error) {
	from, err := b.artefact(renderer.ArtefactPath(v.tier))
	if err != nil {
		return RolloutDoc{}, fmt.Errorf("rollout %q: %w", r.ID(), err)
	}
	to, err := b.artefact(renderer.NextArtefactPath(v.tier))
	if err != nil {
		return RolloutDoc{}, fmt.Errorf("rollout %q: %w", r.ID(), err)
	}

	members := b.members(v, from, to)
	est := estate.Estate{Declaration: collectorDeclaration(), AsOf: b.now}
	for _, m := range members {
		est.Collectors = append(est.Collectors, m.reading)
	}

	verdict, err := rollout.Evaluate(rollout.Inputs{
		Rollout:      r,
		Tier:         v.tier,
		Estate:       est,
		From:         from,
		To:           to,
		StageStarted: stageStarted(r, members, from, to, b.now),
		Now:          b.now,
	})
	if err != nil {
		return RolloutDoc{}, err
	}

	ev := verdict.Evidence
	return RolloutDoc{
		ID:          r.ID(),
		Name:        r.Name,
		Team:        r.Team,
		Owner:       r.Owner,
		Tier:        v.tier.ID(),
		TierName:    v.tier.Name,
		Environment: v.tier.Environment,
		From:        r.From,
		To:          r.To,
		Stage:       r.Stage,
		Decision:    string(verdict.Decision),
		Reason:      verdict.Reason,
		Evidence: RolloutEvidence{
			MembersSeen:  ev.MembersSeen,
			RunningTo:    ev.RunningTo,
			RunningFrom:  ev.RunningFrom,
			RunningOther: ev.RunningOther,
			Unknown:      ev.Unknown,
			Soaked:       rollout.FormatDuration(ev.Soaked),
			MinSoak:      rollout.FormatDuration(ev.MinSoak),
		},
		Cohorts:    cohortProgress(r, members, from, to),
		Halts:      haltLines(ev.Halted, members),
		Provenance: b.rolloutProvenance(r, v.tier),
	}, nil
}

// artefact parses one rendered artefact out of the tree this snapshot was
// rendered from. Both of a dual-bound Tier's artefacts exist at head while
// its Rollout is active (ADR-0029 §3), so a missing one is a broken render
// rather than a state to project.
func (b *builder) artefact(rel string) (rollout.Artefact, error) {
	raw, ok := b.artefacts[rel]
	if !ok {
		return rollout.Artefact{}, fmt.Errorf("%s is not in the rendered tree", rel)
	}
	return rollout.ParseArtefact(raw)
}

// members reads the staged Tier's population into the seam's shape. The
// population is the collectors the serving index matched to this Tier, so
// membership is computed over the serving decision rather than over a
// second reading of the selector (ADR-0007).
func (b *builder) members(v *tierView, from, to rollout.Artefact) []rolloutMember {
	out := make([]rolloutMember, 0, len(v.matched))
	for _, c := range v.matched {
		out = append(out, rolloutMember{
			id:      c.ID,
			foreign: c.Delivery != "served",
			applied: c.AppliedAt,
			reading: estate.Collector{
				Identity:       c.Attributes,
				DeliveryStatus: deliveryStatus(c, from, to, b.now),
			},
		})
	}
	return out
}

// collectorDeclaration is the static capability declaration the declared
// collector estate is played back under (ADR-0036 §1). The readings file
// carries the delivery status and nothing else, so the Effective config and
// the component-health tree are declared incapable: absent with a
// declaration, never a silent gap. A member whose artefact neither its
// acknowledgement nor its declaration names therefore reads unknown, which
// is a normal state (ADR-0008).
func collectorDeclaration() estate.Declaration {
	return estate.Declaration{
		Readings: map[estate.ReadingKind]bool{
			estate.EffectiveKind:      false,
			estate.HealthKind:         false,
			estate.DeliveryStatusKind: true,
		},
		RefreshCadence: collectorRefreshCadence,
	}
}

// deliveryStatus reads one collector's declared delivery status into the
// seam's RemoteConfigStatus shape, verbatim (ADR-0004). Nothing is filled
// in for a reading the estate did not declare: an unacknowledged collector
// is Known false with the cause said out loud, never one defaulted onto an
// artefact because that would be tidier.
func deliveryStatus(c CollectorReading, from, to rollout.Artefact, asOf time.Time) estate.DeliveryStatus {
	at := c.LastSeen
	if at.IsZero() {
		at = asOf
	}

	// A collector that has never reported has no reading to age, and one
	// that never reported is never dark (ADR-0029 §6).
	if c.State == "never_seen" {
		return estate.DeliveryStatus{
			Cause: "this collector has never reported, so nothing it runs is known",
			AsOf:  asOf,
		}
	}

	// The git-delivered path acknowledges nothing over the serving wire, so
	// its delivery status is permanently unavailable and the FAILED signal
	// with it (ADR-0029 §7). Absence there never reads as failure.
	if c.Delivery != "served" {
		return estate.DeliveryStatus{
			Cause: "config on this collector is delivered by the estate's own tooling, which reports no delivery status",
			AsOf:  at,
		}
	}

	if d := c.DeliveryStatus; d != nil {
		hash, ok := acknowledged(d.ConfigHash, from, to)
		if !ok {
			// A stamp naming neither artefact is carried as read: it names
			// some other delivery, and saying which is not this reading's
			// to decide.
			hash, _ = hex.DecodeString(strings.ToLower(d.ConfigHash))
		}
		return estate.DeliveryStatus{
			Known:      true,
			AsOf:       at,
			State:      estate.DeliveryState(strings.ToUpper(d.State)),
			ConfigHash: hash,
			Error:      d.Error,
		}
	}

	// The running stamp names an artefact only when it matches one. A
	// commit stamp, which is what the field carries where no rollout is
	// staged, matches neither and leaves the reading unknown.
	if hash, ok := acknowledged(c.RunningSHA, from, to); ok {
		return estate.DeliveryStatus{
			Known:      true,
			AsOf:       at,
			State:      estate.DeliveryApplied,
			ConfigHash: hash,
		}
	}
	return estate.DeliveryStatus{
		Cause: "the reading declares no configuration this collector has acknowledged",
		AsOf:  at,
	}
}

// acknowledged resolves a declared stamp to the artefact it names. A stamp
// may be abbreviated, as a reading written down by hand usually is, so the
// full hash of the artefact it prefixes is what the seam carries: the
// comparison that decides which artefact runs is then the evaluation's own,
// over two hashes of equal length.
func acknowledged(stamp string, from, to rollout.Artefact) ([]byte, bool) {
	if len(stamp) < minStampHex {
		return nil, false
	}
	raw, err := hex.DecodeString(strings.ToLower(stamp))
	if err != nil {
		return nil, false
	}
	for _, a := range []rollout.Artefact{to, from} {
		if len(a.Hash) > 0 && bytes.HasPrefix(a.Hash, raw) {
			return a.Hash, true
		}
	}
	return nil, false
}

// stageStarted is the instant the active stage's soak runs from. The
// evaluation takes it as data, because git history answers it in
// production; a snapshot has the readings instead, and the earliest instant
// a member of the active cohort reports having applied the candidate
// artefact is what they can answer with. The stage cannot have started
// after its first member took the candidate, so the soak this yields is a
// floor and never over-claims one. Where no member reports applying it, no
// soak is established and the clock reads zero.
func stageStarted(r renderer.Rollout, members []rolloutMember, from, to rollout.Artefact, now time.Time) time.Time {
	var started time.Time
	for _, m := range members {
		if m.applied.IsZero() || !rollout.Member(r, m.reading.Identity) {
			continue
		}
		if rollout.RunningArtefact(m.reading, from, to) != rollout.RunningTo {
			continue
		}
		if started.IsZero() || m.applied.Before(started) {
			started = m.applied
		}
	}
	if started.IsZero() {
		return now
	}
	return started
}

// cohortProgress is each stage's cumulative membership, split by delivery
// path. Entered cohorts accumulate, because advancing only ever widens
// (ADR-0029 §4), so each row is the union of the stages up to and including
// it. A stage past the active one carries membership alone: it is the
// preview a reviewer reads, and no member of it has been offered the
// candidate artefact yet.
func cohortProgress(r renderer.Rollout, members []rolloutMember, from, to rollout.Artefact) []RolloutCohortProgress {
	out := make([]RolloutCohortProgress, 0, len(r.Stages))
	previous := 0
	for i, stage := range r.Stages {
		at := r
		at.Stage = i

		row := RolloutCohortProgress{
			Index:  i,
			Cohort: cohortLabel(stage.Cohort),
			Soak:   rollout.FormatDuration(time.Duration(stage.Soak)),
			State:  cohortState(i, r.Stage),
		}
		for _, m := range members {
			if !rollout.Member(at, m.reading.Identity) {
				continue
			}
			split := &row.Served
			if m.foreign {
				split = &row.Foreign
			}
			split.Members++
			if i > r.Stage {
				continue
			}
			switch rollout.RunningArtefact(m.reading, from, to) {
			case rollout.RunningTo:
				split.To++
			case rollout.RunningFrom:
				split.From++
			case rollout.RunningOther:
				split.Other++
			default:
				split.Unknown++
			}
		}

		total := row.Served.Members + row.Foreign.Members
		row.Widens = total - previous
		previous = total
		out = append(out, row)
	}
	return out
}

// cohortState places a stage against the active one.
func cohortState(index, active int) string {
	switch {
	case index < active:
		return CohortEntered
	case index == active:
		return CohortActive
	}
	return CohortPending
}

// cohortLabel renders one cohort spec for reading. The three forms are
// mixable and membership is their union, so a stage carrying more than one
// reads as the sum of them.
func cohortLabel(c renderer.CohortSpec) string {
	var parts []string
	if h := c.Hosts; h != nil {
		parts = append(parts, fmt.Sprintf("%s ∈ {%s}", h.Attribute, strings.Join(h.Values, ", ")))
	}
	if len(c.Match) > 0 {
		keys := make([]string, 0, len(c.Match))
		for k := range c.Match {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, k+"="+c.Match[k])
		}
		parts = append(parts, strings.Join(pairs, ", "))
	}
	if c.Percent > 0 {
		parts = append(parts, fmt.Sprintf("%d%% of the population", c.Percent))
	}
	return strings.Join(parts, " + ")
}

// haltLines names each halted member in the ledger's own terms. The
// evaluation keys a halt on the member's identity; the ledger shows the id
// a reader recognises and which path the member takes its config by.
func haltLines(halted []rollout.Halt, members []rolloutMember) []RolloutHalt {
	index := make(map[string]rolloutMember, len(members))
	for _, m := range members {
		index[estate.Fingerprint(m.reading.Identity)] = m
	}

	out := []RolloutHalt{}
	for _, h := range halted {
		line := RolloutHalt{Collector: h.Identity, Path: PathServed, Condition: h.Condition, Reason: h.Reason}
		if m, ok := index[h.Identity]; ok {
			line.Collector = m.id
			if m.foreign {
				line.Path = PathForeign
			}
		}
		out = append(out, line)
	}
	return out
}

// rolloutProvenance feeds the "why?" popover on the Rollout panel: the
// authored lines behind the two facts a reader asks about, read from the
// checkout the snapshot is taken from (ADR-0041 §3).
func (b *builder) rolloutProvenance(r renderer.Rollout, tier renderer.Tier) []Provenance {
	file := fmt.Sprintf("teams/%s/rollouts/%s.yaml", r.Team, r.Name)

	// A derivation with no visible cause shows no lines, and shows them as
	// the empty list the contract promises: a reader takes the list's
	// length without first asking whether the list is there.
	lines := func(keys ...string) []ProvenanceLine {
		out := []ProvenanceLine{}
		for _, key := range keys {
			out = append(out, b.locate(file, key)...)
		}
		return out
	}
	return []Provenance{
		{
			Key:   "stage",
			Claim: fmt.Sprintf("stage %d of %d is the active one, set on the Rollout", r.Stage+1, len(r.Stages)),
			Lines: lines("stage"),
			SHA:   b.in.Commit,
		},
		{
			Key:   "bindings",
			Claim: fmt.Sprintf("%s is bound to %s and to %s while this Rollout is active", tier.ID(), r.From, r.To),
			Lines: lines("from", "to"),
			SHA:   b.in.Commit,
		},
	}
}
