package expectation

import (
	"fmt"
	"strings"
	"time"

	"github.com/telecraft-dev/telecraft/internal/inventory"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/selftelemetry"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
	"github.com/telecraft-dev/telecraft/pkg/ownership"
)

// Status is one claim's judged state. The vocabulary is a band state, not
// a verdict word (ADR-0038 §1): data-claim reds surface through the
// outcome cross or an expectation finding, pipeline-claim reds through
// Tier-attached expectation findings. Status itself never becomes an
// eighth outcome.
type Status string

const (
	// StatusGreen: the claim is met, so the config worked.
	StatusGreen Status = "green"

	// StatusPending: neutral-pending: inside the settle window after
	// APPLIED at a new SHA, or a shortfall still inside the dampening
	// grace window (ADR-0038 §4b, §4c). Never red, never green.
	StatusPending Status = "pending"

	// StatusUnknown: the evidence cannot say: a Known false reading, no
	// stamped SHA, nothing landed to check an enrichment against.
	// Unknown never reads as red (ADR-0008, ADR-0038 §4d).
	StatusUnknown Status = "unknown"

	// StatusRed: the shortfall persisted past the grace window against a
	// known reading, so the config didn't work.
	StatusRed Status = "red"
)

// SettleWindows are the per-claim-kind periods after a config goes
// APPLIED at a new SHA during which its claims read neutral-pending
// (ADR-0038 §4b): self-telemetry settles in seconds, arrival and
// enrichment longer. Zero values take the defaults.
type SettleWindows struct {
	Arrival       time.Duration
	Enrichment    time.Duration
	SelfTelemetry time.Duration
}

// The default settle windows: self-telemetry is emitted by the collector
// process itself and settles in seconds; arrival and enrichment wait on
// workloads emitting through the new config, which takes longer.
const (
	DefaultArrivalSettle       = 30 * time.Minute
	DefaultEnrichmentSettle    = 30 * time.Minute
	DefaultSelfTelemetrySettle = 30 * time.Second
)

func (s SettleWindows) forKind(k Kind) time.Duration {
	switch k {
	case Arrival:
		if s.Arrival > 0 {
			return s.Arrival
		}
		return DefaultArrivalSettle
	case Enrichment:
		if s.Enrichment > 0 {
			return s.Enrichment
		}
		return DefaultEnrichmentSettle
	default:
		if s.SelfTelemetry > 0 {
			return s.SelfTelemetry
		}
		return DefaultSelfTelemetrySettle
	}
}

// DefaultObservationWindow is the default window a claim looks back over
// (adopter-overridable, and generous enough to survive overnight quiet,
// (ADR-0038 §4d). The caller reads Observed over this window and the
// judgement takes the reading as given.
const DefaultObservationWindow = 24 * time.Hour

// Config tunes the judgement. Zero values take the defaults everywhere:
// one knob vocabulary, no parallel invention (ADR-0038 §4c): Grace is
// ADR-0035's persistence window, reused verbatim, and Population carries
// the inventory judgement's own knobs.
type Config struct {
	Settle SettleWindows

	// Grace is the shortfall persistence window (ADR-0035 §3): a red
	// requires the shortfall to have persisted this long. Defaults to
	// inventory.DefaultGrace.
	Grace time.Duration

	// Population tunes the ADR-0035 population judgement EvaluateTier
	// runs over the InventoryProvider slice.
	Population inventory.Config
}

func (c Config) grace() time.Duration {
	if c.Grace > 0 {
		return c.Grace
	}
	return inventory.DefaultGrace
}

// claimFloor is the floor a claim's dampening observes: a claim expects
// the thing to exist at all ("at least one"), so the shortfall machinery
// of ADR-0035 applies with a floor of one and no new mechanism.
var claimFloor = inventory.Floor{Source: inventory.FloorDeclared, Min: 1}

// ClaimResult is one claim's judgement.
type ClaimResult struct {
	Claim  Claim
	Status Status

	// Backed reports whether a Requirement demands what the data claim
	// asserts (ADR-0038 §5a): a backed red raises no expectation finding
	// because it is the machinery behind the cross, whose severity and routing
	// the Requirement world already owns. Always false on pipeline
	// claims.
	Backed bool

	// Since is the shortfall onset a red persisted from.
	Since time.Time

	// Detail explains the status in terms a human can act on.
	Detail string
}

// RowEvidence is everything EvaluateRow needs about one (Service,
// Environment) row: when the row's serving artefacts last went APPLIED at
// the claims' SHA (zero when unknown, treated as settled), and the
// Observed reading for the row over the observation window, with the
// claims' enrichment attributes measured (Set.RowAttributes).
type RowEvidence struct {
	AppliedAt time.Time
	Observed  telemetry.Observed
}

// RowResult is one row's expectation judgement: per-claim statuses, plus
// the expectation findings for unbacked data claims that went red:
// Service-attached, advisory-grade, never violation (ADR-0038 §5b): no
// human demanded the signal, so it cannot fail compliance.
type RowResult struct {
	Service     string
	Environment string
	Claims      []ClaimResult
	Findings    []ownership.Finding
}

// EvaluateRow judges one row's data claims against its Observed reading.
// Requirement-backed claims produce no finding of their own: their red
// is the Observed leg the cross already diagnoses (not_delivered,
// broken_pipeline); the result's Backed flag says which claims those are.
// Unbacked reds raise the advisory expectation finding whose remediation
// is honest about the fork: fix the pipeline, or delete the dead lane, which
// doubles as dead-config detection (ADR-0035 §7's aged-never_seen move).
func EvaluateRow(set Set, service, environment string, lib requirements.Library,
	ev RowEvidence, damper *inventory.Damper, cfg Config, now time.Time) RowResult {

	out := RowResult{Service: service, Environment: environment}
	for _, claim := range set.ForRow(service, environment) {
		res := ClaimResult{Claim: claim, Backed: backed(claim, lib)}
		res.Status, res.Since, res.Detail = judgeDataClaim(claim, ev, damper, cfg, now)
		out.Claims = append(out.Claims, res)

		if res.Status == StatusRed && !res.Backed {
			out.Findings = append(out.Findings, ownership.Finding{
				Kind:    ownership.Expectation,
				Subject: ownership.Subject{Kind: ownership.KindService, ID: claim.Service},
				Grade:   ownership.Advisory,
				Detail: fmt.Sprintf("%s. No Requirement demands it, so compliance is unaffected. Fix the pipeline, or delete the dead lane. To find where on the Path the data stopped, see the pipeline findings on %s",
					res.Detail, strings.Join(claim.Tiers, ", ")),
			})
		}
	}
	return out
}

// judgeDataClaim runs one arrival or enrichment claim through the timing
// semantics of ADR-0038 §4: settle first, then knowledge, then the
// persistence-dampened shortfall.
func judgeDataClaim(claim Claim, ev RowEvidence, damper *inventory.Damper,
	cfg Config, now time.Time) (Status, time.Time, string) {

	key := claim.Key()
	if settling(ev.AppliedAt, cfg.Settle.forKind(claim.Kind), now) {
		clearShortfall(damper, key, now)
		return StatusPending, time.Time{}, fmt.Sprintf("inside the %s settle window after APPLIED, so the claim is not judged yet", cfg.Settle.forKind(claim.Kind))
	}

	sig, seen := ev.Observed.Signals[claim.Signal]
	if !seen || !sig.Known {
		// Known false ⇒ unknown, never red (ADR-0038 §4d), and the
		// shortfall clock must not run on evidence nobody has.
		clearShortfall(damper, key, now)
		cause := sig.Cause
		if cause == "" {
			cause = "no reading for the signal"
		}
		return StatusUnknown, time.Time{}, fmt.Sprintf("%s reading unavailable: %s", claim.Signal, cause)
	}

	met := false
	var gap string
	switch claim.Kind {
	case Arrival:
		met = sig.Present
		gap = fmt.Sprintf("expected %s never landed for %s in %s over %s. The config says this signal should land, and nothing arrived",
			claim.Signal, claim.Service, claim.Environment, ev.Observed.Window)
	case Enrichment:
		if !sig.Present {
			// Nothing landed to check the enrichment against: the
			// arrival claim is the one failing, and judging this one too
			// would report one gap twice.
			clearShortfall(damper, key, now)
			return StatusUnknown, time.Time{}, fmt.Sprintf("no %s landed to check %s against. See the arrival claim for this gap", claim.Signal, claim.Attribute)
		}
		coverage, measured := sig.AttributeCoverage[claim.Attribute]
		if !measured {
			clearShortfall(damper, key, now)
			// Integrators: coverage is measured only when Set.RowAttributes
			// is passed to the provider's Observe.
			return StatusUnknown, time.Time{}, fmt.Sprintf("the reading did not measure attribute %q", claim.Attribute)
		}
		met = coverage > 0
		gap = fmt.Sprintf("the config literally inserts %s=%q on %s, and no landed record carries the attribute",
			claim.Attribute, claim.Value, claim.Signal)
	}

	if met {
		clearShortfall(damper, key, now)
		return StatusGreen, time.Time{}, "the config worked: the claim is met"
	}

	since := damper.Observe(key, 0, claimFloor, now)
	if now.Sub(since) < cfg.grace() {
		return StatusPending, since, fmt.Sprintf("shortfall inside the %s grace window, so the claim is not red yet", cfg.grace())
	}
	return StatusRed, since, gap
}

// backed reports whether a Requirement applicable in the claim's
// Environment demands what the claim asserts (ADR-0038 §5a): for an
// arrival claim, a signal assertion on the claim's signal; for an
// enrichment claim, a required attribute of that name on that signal.
func backed(claim Claim, lib requirements.Library) bool {
	for _, req := range lib.Requirements {
		if req.Signal == nil || !req.AppliesTo(claim.Environment) || req.Signal.Kind != claim.Signal {
			continue
		}
		switch claim.Kind {
		case Arrival:
			return true
		case Enrichment:
			for _, attr := range req.Signal.RequiredAttributes {
				if attr == claim.Attribute {
					return true
				}
			}
		}
	}
	return false
}

// TierEvidence is everything EvaluateTier needs about one Tier: the
// artefact its collectors report running (the stamped SHA, never head),
// when it went APPLIED, the Tier's self-telemetry reading, and the
// population evidence from the InventoryProvider slice (ADR-0035),
// already passed through ForEvaluation.
type TierEvidence struct {
	// RunningSHA is the commit stamp the Tier's collectors report, the
	// artefact the claims must derive from (ADR-0038 §4a). Empty means
	// the running artefact is unknown.
	RunningSHA string

	// AppliedAt is when the running artefact went APPLIED; zero when
	// unknown, which is treated as settled.
	AppliedAt time.Time

	Self telemetry.SelfObserved

	// Population is the Tier's population evidence: the derived count
	// from the InventoryProvider, the declared floor, and what the estate
	// shows. Floors escalate through it (ADR-0035 §4, §5) and gate the
	// pipeline claims: when nothing runs, silence is a delivery finding,
	// never an expectation red.
	Population inventory.Population
}

// TierResult is one Tier's expectation judgement: per-claim statuses,
// the ADR-0035 population findings judged from the InventoryProvider
// slice, and the routed findings: Tier-attached, Tier-owner-routed
// expectation findings for pipeline claims that went red
// (violation-capable after dampening, ADR-0038 §5c), plus the population
// findings converted to delivery-kind findings, so never_seen and
// under_populated escalate through the floors in the same pass.
type TierResult struct {
	Tier       string
	Claims     []ClaimResult
	Population []inventory.Finding
	Findings   []ownership.Finding
}

// EvaluateTier judges one Tier's pipeline claims against its
// self-telemetry reading. The set must derive from the artefact the
// collectors report running: judging head's claims against another
// commit's telemetry is exactly the cascade ADR-0038 §4a makes
// structurally impossible, so a mismatch is a caller bug, not a reading.
func EvaluateTier(set Set, tier string, ev TierEvidence, damper *inventory.Damper,
	cfg Config, now time.Time) (TierResult, error) {

	if ev.RunningSHA != "" && set.SHA != ev.RunningSHA {
		return TierResult{}, fmt.Errorf("claim set derives from %s but tier %q reports running %s. Claims are judged against the artefact the collector reports running, so derive at the stamped SHA",
			set.SHA, tier, ev.RunningSHA)
	}

	out := TierResult{Tier: tier}

	// Floors first (ADR-0035, via the InventoryProvider slice): the
	// population judgement escalates never_seen and under_populated
	// through the resolved floor, and its findings join the delivery
	// kind, because a population shortfall is a delivery problem, never an
	// expectation red in disguise.
	out.Population = ev.Population.Findings(cfg.Population, now)
	out.Findings = append(out.Findings, inventory.DeliveryFindings(out.Population)...)

	claims := set.ForTier(tier)

	// Gates that make every claim unknown, in evidence order: an unknown
	// running artefact, an empty population, a reading nobody has.
	if cause, gated := tierGate(ev); gated {
		for _, claim := range claims {
			clearShortfall(damper, claim.Key(), now)
			out.Claims = append(out.Claims, ClaimResult{Claim: claim, Status: StatusUnknown, Detail: cause})
		}
		return out, nil
	}

	if settling(ev.AppliedAt, cfg.Settle.forKind(SelfTelemetry), now) {
		detail := fmt.Sprintf("inside the %s settle window after APPLIED at %s, so the claim is not judged yet", cfg.Settle.forKind(SelfTelemetry), set.SHA)
		for _, claim := range claims {
			clearShortfall(damper, claim.Key(), now)
			out.Claims = append(out.Claims, ClaimResult{Claim: claim, Status: StatusPending, Detail: detail})
		}
		return out, nil
	}

	identified, unidentified := observedComponents(ev.Self)
	for _, claim := range claims {
		res := ClaimResult{Claim: claim}
		matched := identified[componentKey{claim.ComponentKind, claim.Component}] ||
			(claim.Shape == ShapeUnidentified && unidentified[claim.ComponentKind])

		switch {
		case matched:
			clearShortfall(damper, claim.Key(), now)
			res.Status = StatusGreen
			res.Detail = "the component emits its own telemetry, so the config worked"
		default:
			since := damper.Observe(claim.Key(), 0, claimFloor, now)
			res.Since = since
			if now.Sub(since) < cfg.grace() {
				res.Status = StatusPending
				res.Detail = fmt.Sprintf("silent, inside the %s grace window, so the claim is not red yet", cfg.grace())
			} else {
				res.Status = StatusRed
				res.Detail = fmt.Sprintf("%s %s is instantiated in the artefact at %s and has emitted no self-telemetry past the settle window, so the config did not work",
					claim.ComponentKind, claim.Component, set.SHA)
				out.Findings = append(out.Findings, ownership.Finding{
					Kind:    ownership.Expectation,
					Subject: ownership.Subject{Kind: ownership.KindTier, ID: claim.Tier},
					Grade:   ownership.Violation,
					Detail:  res.Detail,
				})
			}
		}
		out.Claims = append(out.Claims, res)
	}
	return out, nil
}

// tierGate returns the cause that makes every pipeline claim unknown,
// when one holds. An unapplied or unreported artefact and an empty
// population are delivery's reds and drift's reds, never expectation's
// (ADR-0038 §4a: delivery failure is structurally unable to cascade into
// expectation-red).
func tierGate(ev TierEvidence) (string, bool) {
	if ev.RunningSHA == "" {
		return "the collectors report no commit stamp, so no artefact is known to be in force and there is nothing to judge the claims against", true
	}
	if ev.Population.Seen == 0 {
		return "no collector is matched into this Tier. See the Tier's population finding", true
	}
	known := false
	var causes []string
	for _, kind := range telemetry.SelfSignals() {
		sig, ok := ev.Self.Signals[kind]
		if ok && sig.Known {
			known = true
		} else if ok && sig.Cause != "" {
			causes = append(causes, sig.Cause)
		}
	}
	if !known {
		cause := "no self-telemetry reading is available"
		if len(causes) > 0 {
			cause += ": " + strings.Join(causes, "; ")
		}
		return cause + ", so the claims read as unknown", true
	}
	return "", false
}

// componentKey joins a claim to a normalised reading.
type componentKey struct {
	kind selftelemetry.Kind
	id   string
}

// observedComponents normalises every component-identity attribute
// combination the reading carries, through the one platform-owned
// normaliser (ADR-0039 §3), into the identified set and the
// unidentified-kind set the claims match against. Synthetic graph nodes
// and collector-level readings are tolerated and match nothing: expected
// shapes, never failures (R-4 §5.4).
func observedComponents(self telemetry.SelfObserved) (map[componentKey]bool, map[selftelemetry.Kind]bool) {
	identified := map[componentKey]bool{}
	unidentified := map[selftelemetry.Kind]bool{}
	for _, kind := range telemetry.SelfSignals() {
		sig, ok := self.Signals[kind]
		if !ok || !sig.Known {
			continue
		}
		for _, comp := range sig.Components {
			r := selftelemetry.Normalise(comp.Attributes)
			switch r.Class {
			case selftelemetry.ClassComponent:
				identified[componentKey{r.Kind, r.ID}] = true
			case selftelemetry.ClassUnidentified:
				unidentified[r.Kind] = true
			}
		}
	}
	return identified, unidentified
}

// settling reports whether now is inside the settle window after
// AppliedAt. A zero AppliedAt means the transition instant is unknown:
// treated as settled, because holding claims pending forever would make
// not-knowing a permanent neutral.
func settling(appliedAt time.Time, window time.Duration, now time.Time) bool {
	return !appliedAt.IsZero() && now.Sub(appliedAt) < window
}

// clearShortfall resets a claim's dampening clock: no shortfall is in
// force, so a future one serves its full grace window (ADR-0035 §3).
func clearShortfall(damper *inventory.Damper, key string, now time.Time) {
	damper.Observe(key, claimFloor.Min, claimFloor, now)
}
