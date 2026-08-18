// Package inventory defines the InventoryProvider seam (ADR-0035): one
// deliberately narrow question — given this Tier's selector, how many
// instances should match. The answer is a count plus an as-of timestamp,
// Known false when the substrate cannot say (ADR-0008 discipline). The
// seam is separate from EstateProvider by design: the estate seam reads
// the population that exists, keyed on the collector; this seam reads what
// should exist, from the substrate (a container orchestrator's API, a
// CMDB, a cloud inventory) — different source, different auth, different
// deployment shape.
//
// Expectations built on the answer are floors, never equalities (ADR-0035
// §2): the only finding is a shortfall, surplus is never a finding, and
// the platform never invents a count — a provider that cannot answer says
// so with Known false and a cause, and an absent provider simply leaves
// the derived source absent.
//
// Freshness is the platform's arithmetic, never the provider's claim
// (ADR-0036 §3, applied here as ADR-0035 requires the same contract
// discipline): the implementation declares its refresh cadence, and
// ForEvaluation demotes a count past the staleness horizon to Known false
// before it can feed a floor. Stale data may inform a human, never a
// verdict.
//
// The contract ships as a conformance test kit, not prose (ADR-0036 §4):
// internal/inventory/inventorytest runs against any implementation.
// Implementations live under internal/provider/ and are qualified there
// (ADR-0001); nothing vendor-shaped crosses this interface in either
// direction.
package inventory

import (
	"context"
	"fmt"
	"time"
)

// Declaration is an implementation's static contract declaration: stated
// once, before any reading, and never varying per selector or per call.
// The seam carries one reading kind — the expected count — so the
// declaration's load-bearing field is the cadence.
type Declaration struct {
	// RefreshCadence is how often the implementation's answer refreshes,
	// declared so the platform can compute staleness uniformly (ADR-0036
	// §3). Mandatory and positive: without a cadence no freshness
	// arithmetic exists, and ForEvaluation demotes every count rather
	// than let an unverifiable one feed a floor.
	RefreshCadence time.Duration
}

// Provider is the InventoryProvider seam (ADR-0035). Implementations live
// under internal/provider/ (ADR-0001).
type Provider interface {
	// Name identifies the implementation for logs and stamps — a
	// qualified name as runtime data, never a type.
	Name() string

	// Declaration is the static contract declaration.
	Declaration() Declaration

	// Expected answers the seam's one question: how many instances should
	// match this Tier's selector right now. A selector matching nothing is
	// a count of zero — a real reading, not a blind spot. An empty
	// selector, an unreachable substrate, or an unanswerable ask comes
	// back Known false with a cause — degradation is data in the reading,
	// never an error and never a crash (ADR-0008).
	Expected(ctx context.Context, selector map[string]string) Count
}

// Count is one answer: the substrate's expected population for a
// selector, taken at one instant.
type Count struct {
	// Known keeps "the substrate cannot say" distinct from "the substrate
	// says zero" (ADR-0008). When false, Cause says why and Instances
	// means nothing.
	Known bool
	Cause string

	// AsOf is the instant the count was taken. Set whenever the count is
	// carried, Known or not: "we could not count, as of when" is still a
	// statement with a timestamp (ADR-0036 §2).
	AsOf time.Time

	// Instances is how many instances should match, per the substrate.
	Instances int
}

// StaleTolerance is the multiplier over the declared refresh cadence that
// sets the staleness horizon (ADR-0036 §3): a count older than
// cadence × StaleTolerance is demoted to Known false at evaluation —
// three missed refreshes is decisively quiet, while one slow poll is not,
// the same posture as the estate seam.
const StaleTolerance = 3

// ForEvaluation returns the count as floor resolution must see it: past
// the staleness horizon at now it is demoted to Known false with its
// payload cleared, so a stale derived count can never float a
// fresh-looking floor. AsOf survives the demotion — "we stopped counting,
// as of when" stays a statement with a timestamp. A declaration without a
// cadence demotes unconditionally: freshness that cannot be established
// must not feed a floor, and the cause names the declaration's fault.
func (c Count) ForEvaluation(d Declaration, now time.Time) Count {
	if !c.Known {
		return c
	}
	if d.RefreshCadence <= 0 {
		return Count{Known: false, AsOf: c.AsOf,
			Cause: "the provider declares no refresh cadence, so freshness cannot be established — a count of unverifiable age never feeds a floor (ADR-0036 §3)"}
	}
	horizon := d.RefreshCadence * StaleTolerance
	age := now.Sub(c.AsOf)
	if age <= horizon {
		return c
	}
	return Count{Known: false, AsOf: c.AsOf,
		Cause: fmt.Sprintf("stale: counted %s ago, past the %s staleness horizon (declared cadence %s × tolerance %d) — stale data may inform a human, never a floor (ADR-0036 §3)",
			age.Round(time.Second), horizon, d.RefreshCadence, StaleTolerance)}
}
