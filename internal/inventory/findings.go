package inventory

import (
	"fmt"
	"time"

	"github.com/telecraft-dev/telecraft/internal/ownership"
)

// Class names one population finding class (ADR-0035). never_seen and
// under_populated are siblings, never degrees of each other: "12 of 40
// running" and "nothing exists" are different situations with different
// fixes, and conflating them would misdescribe both (§5).
type Class string

const (
	// NeverSeen is a Tier no collector has ever matched into (ADR-0030).
	// Neutral without a floor (a freshly authored Tier awaiting its
	// workload is a normal Tuesday) and violation-grade only when a
	// floor > 0 and zero matches persist past the grace window (§4).
	NeverSeen Class = "never_seen"

	// UnderPopulated is collectors present but below the floor,
	// persisting: "expected ≥40, seen 12" (§5).
	UnderPopulated Class = "under_populated"

	// FloorConflict is the two sources disagreeing: a declared floor
	// above the live derived count. Resolution prefers derived, but the
	// comparison is never silent: a declared floor above live reality
	// usually means the estate shrank and someone should notice (§2).
	FloorConflict Class = "floor_conflict"
)

// Grade is a population finding's weight. Neutral is not a pass: a
// neutral finding is excluded from every denominator (P2's rule, ADR-0035
// §6), where a pass would count in one.
type Grade string

const (
	Neutral   Grade = "neutral"
	Advisory  Grade = "advisory"
	Violation Grade = "violation"
)

// Finding is one population finding, Tier-attached (ADR-0035 §6): it
// routes to the Tier's owner and joins the delivery finding kind in the
// roll-up, because a population shortfall is a delivery problem, never a
// conformance problem with any Service.
type Finding struct {
	Class Class
	Tier  string // team-qualified Tier id
	Grade Grade

	// Floor is the resolved floor the finding was judged against; absent
	// on a neutral never_seen with no floor.
	Floor Floor

	// Seen is how many collectors are currently matched into the Tier.
	Seen int

	// Since is when the condition began: the shortfall onset on a toothed
	// finding, the start of watching on a neutral never_seen. Zero when
	// unknown.
	Since time.Time

	// StaleConfig marks an aged neutral never_seen (ADR-0035 §7): the
	// platform's stale-config signal, an authored Tier never used, a
	// candidate for deletion. A presentation affordance, never a new
	// finding class, so the Grade stays Neutral.
	StaleConfig bool

	// Detail explains the finding in terms a human can act on.
	Detail string
}

// Config tunes the population judgement. Zero values take the defaults.
type Config struct {
	// Grace is the persistence window (ADR-0035 §3): a shortfall must
	// persist this long before any finding raises, because scale events
	// have an honest transient where seen < expected for minutes.
	Grace time.Duration

	// StaleConfigAge is how long a neutral never_seen persists before it
	// is flagged as the stale-config signal (ADR-0035 §7).
	StaleConfigAge time.Duration
}

// DefaultGrace is the default persistence window: order of minutes
// (ADR-0035 §3): long enough for nodes to join and workload pods to
// schedule, short enough that a real shortfall is not sat on.
const DefaultGrace = 5 * time.Minute

// DefaultStaleConfigAge is the default age at which a neutral never_seen
// reads as stale config, the "never matched in 90 days" surface of
// ADR-0035 §7.
const DefaultStaleConfigAge = 90 * 24 * time.Hour

// Population is one Tier's population evidence at one instant: what the
// substrate expects, what git declares, and what the estate shows.
type Population struct {
	// Tier is the team-qualified Tier id the findings attach to.
	Tier string

	// Derived is the InventoryProvider's answer for the Tier's selector,
	// already passed through ForEvaluation. The zero value (Known false)
	// is the no-provider case: the derived source is simply absent.
	Derived Count

	// Declared is the Tier's authored min_expected; zero means none.
	Declared int

	// Seen is how many collectors are currently matched into the Tier by
	// its selector.
	Seen int

	// EverSeen reports whether any collector has ever matched this Tier.
	// False keeps never_seen's exact G4 meaning (ADR-0030): a Tier that
	// once had collectors and dropped to zero is under-populated, never
	// never_seen.
	EverSeen bool

	// FirstWatched is when the platform started watching this Tier, the
	// age base for the never_seen stale-config signal (§7). Zero means
	// the age is unknown and no stale-config flag can raise.
	FirstWatched time.Time

	// ShortfallSince is when the current sub-floor condition began, as
	// tracked by a Damper; zero when seen meets the floor. The judgement
	// never raises a toothed finding from a single instant (§3).
	ShortfallSince time.Time
}

// Findings judges one Tier's population (ADR-0035). The floor is resolved
// derived > declared > absent; with no floor there are no teeth, so the
// only possible finding is the neutral never_seen. Toothed findings
// require the shortfall to have persisted past the grace window. Surplus
// is never a finding.
func (p Population) Findings(cfg Config, now time.Time) []Finding {
	grace := cfg.Grace
	if grace <= 0 {
		grace = DefaultGrace
	}
	staleAge := cfg.StaleConfigAge
	if staleAge <= 0 {
		staleAge = DefaultStaleConfigAge
	}

	floor := ResolveFloor(p.Derived, p.Declared)
	var out []Finding

	// §2: when both sources exist they are compared, not silently
	// resolved.
	if p.Derived.Known && p.Declared > p.Derived.Instances {
		out = append(out, Finding{
			Class: FloorConflict,
			Tier:  p.Tier,
			Grade: Advisory,
			Floor: floor,
			Seen:  p.Seen,
			Detail: fmt.Sprintf("declared floor min_expected %d is above the derived count %d. The estate has probably shrunk: check the declared floor",
				p.Declared, p.Derived.Instances),
		})
	}

	toothed := floor.Source != FloorAbsent && floor.Min > 0
	persisted := !p.ShortfallSince.IsZero() && now.Sub(p.ShortfallSince) >= grace

	switch {
	case !p.EverSeen:
		f := Finding{Class: NeverSeen, Tier: p.Tier, Grade: Neutral, Floor: floor, Since: p.FirstWatched}
		if toothed && persisted {
			// §4: the one escalation rule, floor > 0 and zero matches
			// persisting past the window. Neutrality is otherwise
			// untouched.
			f.Grade = Violation
			f.Since = p.ShortfallSince
			f.Detail = fmt.Sprintf("expected ≥%d (%s floor), seen 0 for longer than the %s grace window",
				floor.Min, floor.Source, grace)
			out = append(out, f)
			break
		}
		f.Detail = "no collector has ever matched this Tier's selector. This is normal for a newly authored Tier that is still waiting for its workload"
		if !p.FirstWatched.IsZero() && now.Sub(p.FirstWatched) >= staleAge {
			f.StaleConfig = true
			f.Detail = fmt.Sprintf("no collector has matched this Tier's selector in %d days. A Tier that is never used may be stale configuration: consider deleting it",
				int(now.Sub(p.FirstWatched).Hours()/24))
		}
		out = append(out, f)

	case toothed && p.Seen < floor.Min && persisted:
		// §5: collectors present but below the floor, persisting. This
		// arm also owns the dropped-to-zero case: the Tier has readings
		// and history, and calling it never_seen would make "0 of 40
		// running" read as "nothing ever existed".
		out = append(out, Finding{
			Class: UnderPopulated,
			Tier:  p.Tier,
			Grade: Violation,
			Floor: floor,
			Seen:  p.Seen,
			Since: p.ShortfallSince,
			Detail: fmt.Sprintf("expected ≥%d (%s floor), seen %d for longer than the %s grace window",
				floor.Min, floor.Source, p.Seen, grace),
		})
	}
	return out
}

// Damper tracks when each Tier's shortfall began, so the judgement is
// persistence-dampened (ADR-0035 §3): scale events have an honest
// transient (nodes joined, workload pods still scheduling) where
// seen < expected for minutes, and a finding raised from one instant
// would page someone for the autoscaler breathing.
type Damper struct {
	onset map[string]time.Time
}

// NewDamper builds an empty Damper. One Damper serves the whole estate,
// keyed by Tier id.
func NewDamper() *Damper {
	return &Damper{onset: map[string]time.Time{}}
}

// Observe notes one observation of a Tier's population against its
// resolved floor and returns when the current shortfall began, or zero when
// the Tier meets its floor or has none. The first sub-floor observation
// starts the clock; recovery clears it, so a fresh shortfall always
// serves its full grace window.
func (d *Damper) Observe(tier string, seen int, floor Floor, now time.Time) time.Time {
	short := floor.Source != FloorAbsent && floor.Min > 0 && seen < floor.Min
	if !short {
		delete(d.onset, tier)
		return time.Time{}
	}
	if since, held := d.onset[tier]; held {
		return since
	}
	d.onset[tier] = now
	return now
}

// DeliveryFindings converts population findings into ADR-0017 roll-up
// findings: Tier-attached, routed to the Tier's owner, delivery-kind, because
// a population shortfall is a delivery problem, never a conformance
// problem with any Service (ADR-0035 §6). Escalated findings enter the
// denominator; neutral ones are excluded entirely (P2's rule), which is
// why a neutral never_seen produces nothing here rather than a pass.
// Exemptions apply as everywhere: the caller sets Waived on the returned
// findings when an owned, expiring Exemption covers the gap.
func DeliveryFindings(findings []Finding) []ownership.Finding {
	var out []ownership.Finding
	for _, f := range findings {
		var grade ownership.Grade
		switch f.Grade {
		case Violation:
			grade = ownership.Violation
		case Advisory:
			grade = ownership.Advisory
		default:
			continue
		}
		out = append(out, ownership.Finding{
			Kind:    ownership.Delivery,
			Subject: ownership.Subject{Kind: ownership.KindTier, ID: f.Tier},
			Grade:   grade,
			Detail:  f.Detail,
		})
	}
	return out
}
