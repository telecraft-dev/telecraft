// Package inventorytest is the shipped InventoryProvider conformance kit:
// the contract as a fixture suite every implementation must pass, never
// prose. It is the same kit pattern ADR-0036 §4 established for EstateProvider,
// applied to this seam as ADR-0035 requires. Run drives a provider the
// harness has pointed at a known substrate and fails the test with one
// actionable line per violation; Violations is the same judgement as
// data, which is how the kit's own tests prove a deliberately broken
// provider is caught.
//
// The kit checks: a positive declared refresh cadence (freshness is the
// platform's arithmetic and the cadence is its mandatory input); every
// seeded selector answered Known with the arranged count and an as_of;
// a selector matching nothing answered as a count of zero, a real
// reading, never Known false and never a guess; an empty selector and an
// unanswerable ask answered Known false with a cause and an as_of, never
// an invented count and never an error; and staleness demotion (a count
// past the declared horizon never survives into floor resolution).
package inventorytest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/inventory"
)

// Kit is one conformance run: an implementation wired to a substrate the
// harness controls, plus what the harness arranged for it to count.
type Kit struct {
	// Provider is the implementation under test, already reading a
	// substrate that answers the Seeded selectors.
	Provider inventory.Provider

	// Seeded lists selectors the harness arranged, with the exact count
	// the provider must derive for each. Must be non-empty: a kit run
	// that counts nothing passes vacuously. Seed at least one selector
	// with a positive count, so staleness demotion has a real reading to
	// exercise, and one with Instances zero if the harness can arrange a
	// selector matching nothing.
	Seeded []Seed

	// Unanswerable, when non-empty, is a selector the harness guarantees
	// the provider cannot answer (an unmappable attribute, a substrate
	// hole). The ask must come back Known false with a cause, never an
	// invented count. Optional: an implementation that can answer every
	// well-formed selector has nothing to seed here.
	Unanswerable map[string]string
}

// Seed is one selector the harness arranged, and the count the provider
// must derive for it.
type Seed struct {
	// Selector is the Tier-selector-shaped ask: equality over identity
	// attributes.
	Selector map[string]string

	// Instances is the exact expected population the substrate holds for
	// the selector. Zero is a real seed: a selector matching nothing is a
	// count of zero, not a blind spot.
	Instances int
}

// Run executes the kit and fails the test with one line per violation.
func Run(t *testing.T, k Kit) {
	t.Helper()
	for _, v := range Violations(context.Background(), k) {
		t.Error(v)
	}
}

// Violations executes the kit and returns every contract violation found,
// each one actionable: what failed, on which selector, and which rule.
// Empty means the implementation conforms.
func Violations(ctx context.Context, k Kit) []string {
	var out []string
	fail := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }

	if k.Provider == nil {
		return []string{"the kit was handed no provider"}
	}
	if len(k.Seeded) == 0 {
		return []string{"the kit was handed no seeded selectors: a run that counts nothing proves nothing"}
	}

	decl := k.Provider.Declaration()
	if decl.RefreshCadence <= 0 {
		fail("the declaration carries no refresh cadence: Telecraft needs it to tell a fresh count from a stale one")
	}

	for _, seed := range k.Seeded {
		name := estate.Fingerprint(seed.Selector)
		c := k.Provider.Expected(ctx, seed.Selector)
		checkCarried(fail, "selector "+name, c)
		switch {
		case !c.Known:
			fail("selector %s: the count came back Known false (%s) though the harness arranged the substrate to answer. A selector matching little or nothing is a count, never a blind spot", name, c.Cause)
		case c.Instances != seed.Instances:
			fail("selector %s: Instances = %d, want %d: report the substrate's count exactly", name, c.Instances, seed.Instances)
		}
	}

	// An empty selector matches nothing by construction (ADR-0007: a
	// collector is matched by satisfying every authored pair, and the
	// most specific selector wins; a pairless Tier is served the
	// Unmatched artefact, ADR-0030). Counting "everything" would answer a
	// question nobody asked, so the honest reading is Known false.
	empty := k.Provider.Expected(ctx, nil)
	checkCarried(fail, "the empty selector", empty)
	if empty.Known {
		fail("the empty selector came back Known with Instances = %d: a selector-less ask has no population to count, so answer Known false", empty.Instances)
	}

	if len(k.Unanswerable) > 0 {
		name := estate.Fingerprint(k.Unanswerable)
		c := k.Provider.Expected(ctx, k.Unanswerable)
		checkCarried(fail, "unanswerable selector "+name, c)
		if c.Known {
			fail("unanswerable selector %s: the count came back Known with Instances = %d. The harness guaranteed no answer exists, so this is an invented count", name, c.Instances)
		}
	}

	checkStaleness(fail, decl, k)
	return out
}

// checkCarried holds the rules every carried count obeys, Known or not:
// a mandatory as_of, a cause on every unknown, and no payload without
// knowledge.
func checkCarried(fail func(string, ...any), name string, c inventory.Count) {
	if c.AsOf.IsZero() {
		fail("%s: the count carries no as_of: even a count that failed needs a timestamp", name)
	}
	if c.Instances < 0 {
		fail("%s: Instances = %d: a population is never negative", name, c.Instances)
	}
	if !c.Known {
		if c.Cause == "" {
			fail("%s: the count is a silent gap: Known false with no cause. Say why nothing is known", name)
		}
		if c.Instances != 0 {
			fail("%s: the count carries Instances = %d while Known is false: an unknown count's payload must be zero", name, c.Instances)
		}
	}
}

// checkStaleness holds the demotion rule (ADR-0036 §3) against a real
// count: inside the horizon it survives evaluation, past the horizon it
// demotes to Known false with its payload gone. The arithmetic is the
// platform's, but running it here proves the provider's as_of and
// cadence make it computable.
func checkStaleness(fail func(string, ...any), decl inventory.Declaration, k Kit) {
	horizon := decl.RefreshCadence * inventory.StaleTolerance
	if horizon <= 0 {
		return // the missing cadence is already a violation
	}
	for _, seed := range k.Seeded {
		c := k.Provider.Expected(context.Background(), seed.Selector)
		if !c.Known || c.AsOf.IsZero() {
			continue
		}
		name := estate.Fingerprint(seed.Selector)
		if fresh := c.ForEvaluation(decl, c.AsOf.Add(horizon)); !fresh.Known {
			fail("selector %s: the count was demoted while still inside the staleness horizon: only a count past the horizon demotes", name)
		}
		stale := c.ForEvaluation(decl, c.AsOf.Add(horizon+time.Second))
		switch {
		case stale.Known:
			fail("selector %s: the count survived evaluation past the staleness horizon: a stale count cannot set a floor", name)
		case stale.Instances != 0:
			fail("selector %s: the count was demoted but still carries Instances = %d: clear it so nothing downstream can use it", name, stale.Instances)
		case stale.Cause == "":
			fail("selector %s: the count was demoted without a cause", name)
		}
		return // one real count proves the arithmetic
	}
	fail("no seeded selector yields a Known count with as_of, so staleness demotion could not be exercised: seed at least one answerable selector")
}
