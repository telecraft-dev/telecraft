// Package estatetest is the shipped EstateProvider conformance kit
// (ADR-0036 §4): the contract as a fixture suite every implementation must
// pass, never prose. Run drives a provider the harness has seeded with
// known collectors and fails the test with one actionable line per
// violation; Violations is the same judgement as data, which is how the
// kit's own tests prove a deliberately broken provider is caught.
//
// The kit checks what ADR-0036 lists: capability honesty — every reading
// kind explicitly declared, incapable readings absent-with-declaration and
// never populated, capable-but-unknown readings loud with a cause;
// the minimum populated set — identity attributes on every collector and
// an as_of timestamp on every reading carried; structural preservation —
// pipeline component order and the recursive health tree exactly as
// seeded; the unknown-collector discipline — Known false, never an error;
// and staleness demotion — a reading past the declared horizon never
// survives into evaluation.
//
// ADR-0008's "verify the seam against a third implementation" stays true
// through this kit rather than re-litigation: a new implementation passes
// Run, or it does not conform.
package estatetest

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/estate"
)

// Kit is one conformance run: an implementation wired to a live fixture
// estate, plus what the harness arranged for it to see.
type Kit struct {
	// Provider is the implementation under test, already reading a
	// fixture estate that contains the Seeded collectors.
	Provider estate.Provider

	// Seeded lists the collectors the harness arranged, with the exact
	// readings the provider must reproduce. Must be non-empty: a kit run
	// over an empty estate would pass vacuously.
	Seeded []Seed

	// Absent is an identity guaranteed to match no collector; the
	// unknown-collector checks ask for it. Empty means the kit invents
	// one.
	Absent map[string]string
}

// Seed is one collector the harness arranged, and the readings the
// provider must reproduce for it. Nil or zero expectation fields are not
// checked — a harness states what it controls.
type Seed struct {
	// Identity is the identifying attributes the collector reports; the
	// provider must return exactly one collector carrying all of them.
	Identity map[string]string

	// Pipelines, when non-nil, is the exact Effective pipeline list the
	// collector runs — compared deep and in order (ADR-0004).
	Pipelines []estate.Pipeline

	// Health, when non-nil, is the exact component-health tree —
	// compared recursively, so a flattened roll-up cannot pass
	// (ADR-0008).
	Health *estate.ComponentHealth

	// Delivery, when non-empty, is the delivery state the collector must
	// report (ADR-0004 vocabulary).
	Delivery estate.DeliveryState
}

// Run executes the kit and fails the test with one line per violation.
func Run(t *testing.T, k Kit) {
	t.Helper()
	for _, v := range Violations(context.Background(), k) {
		t.Error(v)
	}
}

// Violations executes the kit and returns every contract violation found,
// each one actionable: what failed, on which collector, and which rule.
// Empty means the implementation conforms.
func Violations(ctx context.Context, k Kit) []string {
	var out []string
	fail := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }

	if k.Provider == nil {
		return []string{"the kit was handed no provider"}
	}
	if len(k.Seeded) == 0 {
		return []string{"the kit was handed no seeded collectors — a run over an empty estate passes vacuously and proves nothing"}
	}

	decl := k.Provider.Declaration()
	for _, kind := range estate.Kinds() {
		if _, declared := decl.Readings[kind]; !declared {
			fail("the declaration says nothing about reading %q — incapable is a declaration, never an omission (ADR-0036 §1)", kind)
		}
	}
	if decl.RefreshCadence <= 0 {
		fail("the declaration carries no refresh cadence — freshness is the platform's arithmetic and the cadence is its mandatory input (ADR-0036 §3)")
	}

	est := k.Provider.Estate(ctx)
	if est.AsOf.IsZero() {
		fail("the estate reading carries no as_of — even an empty estate is a statement with a timestamp (ADR-0036 §2)")
	}
	if !reflect.DeepEqual(est.Declaration, decl) {
		fail("the estate reading echoes a declaration different from the provider's — the declaration is static, one truth (ADR-0036 §1)")
	}

	for _, c := range est.Collectors {
		checkCollector(fail, decl, c)
	}

	for _, seed := range k.Seeded {
		checkSeed(fail, decl, est, seed)
	}

	checkUnknown(fail, decl, est, k.Absent)
	checkStaleness(fail, decl, est, k.Seeded)
	return out
}

// checkCollector holds the minimum populated set (ADR-0036 §2) and
// capability honesty (§1) on one returned collector.
func checkCollector(fail func(string, ...any), decl estate.Declaration, c estate.Collector) {
	name := estate.Fingerprint(c.Identity)
	if len(c.Identity) == 0 {
		fail("a collector was returned with no identity attributes — a reading nothing can match belongs to nobody, and absent identity is non-conforming full stop (ADR-0036 §2)")
		name = "(no identity)"
	}
	for _, v := range readings(c) {
		switch {
		case !decl.Capable(v.kind):
			if v.known || v.populated || v.cause != "" {
				fail("collector %s populates reading %q, which the declaration says it can never populate — declare the capability or stop reporting it (ADR-0036 §1)", name, v.kind)
			}
		case v.known && v.asOf.IsZero():
			fail("collector %s reading %q is populated without as_of — absent timestamps are non-conforming, full stop (ADR-0036 §2)", name, v.kind)
		case !v.known && v.cause == "":
			fail("collector %s reading %q is a silent gap: declared capable, not delivered, no cause — capable-but-silent is a provider fault and must be loud (ADR-0036 §1)", name, v.kind)
		case !v.known && v.asOf.IsZero():
			fail("collector %s reading %q is unknown without as_of — 'we cannot see' is still a statement with a timestamp (ADR-0036 §2)", name, v.kind)
		case !v.known && v.populated:
			fail("collector %s reading %q carries a payload while Known is false — an unknown reading's payload means nothing and must be empty", name, v.kind)
		}
	}
}

// checkSeed holds the provider to what the harness arranged: the collector
// exists exactly once, and its readings reproduce the seed byte-for-byte.
func checkSeed(fail func(string, ...any), decl estate.Declaration, est estate.Estate, seed Seed) {
	name := estate.Fingerprint(seed.Identity)
	var found []estate.Collector
	for _, c := range est.Collectors {
		if containsAll(c.Identity, seed.Identity) {
			found = append(found, c)
		}
	}
	if len(found) != 1 {
		fail("seeded collector %s appears %d times in the estate reading, want exactly once", name, len(found))
		return
	}
	c := found[0]

	if seed.Pipelines != nil {
		switch {
		case !decl.Capable(estate.EffectiveKind):
			fail("the kit was seeded with pipelines but the declaration says Effective can never be populated — the harness and the declaration disagree")
		case !c.Effective.Known:
			fail("seeded collector %s: Effective is unknown (%s) though the harness arranged a running config", name, c.Effective.Cause)
		case !reflect.DeepEqual(c.Effective.Pipelines, seed.Pipelines):
			fail("seeded collector %s: Effective pipelines differ from what the collector runs — order and wiring must survive verbatim, never flattened or resorted (ADR-0004)\n  got:  %+v\n  want: %+v", name, c.Effective.Pipelines, seed.Pipelines)
		}
	}
	if seed.Health != nil {
		switch {
		case !decl.Capable(estate.HealthKind):
			fail("the kit was seeded with a health tree but the declaration says health can never be populated — the harness and the declaration disagree")
		case !c.Health.Known:
			fail("seeded collector %s: health is unknown (%s) though the harness arranged a health report", name, c.Health.Cause)
		case !reflect.DeepEqual(c.Health.Component, *seed.Health):
			fail("seeded collector %s: the health tree differs from what the collector reported — the recursive tree must survive verbatim, never the flattened roll-up (ADR-0008)\n  got:  %+v\n  want: %+v", name, c.Health.Component, *seed.Health)
		}
	}
	if seed.Delivery != "" {
		switch {
		case !decl.Capable(estate.DeliveryStatusKind):
			fail("the kit was seeded with a delivery state but the declaration says delivery status can never be populated — the harness and the declaration disagree")
		case !c.DeliveryStatus.Known:
			fail("seeded collector %s: delivery status is unknown (%s) though the harness arranged a report", name, c.DeliveryStatus.Cause)
		case c.DeliveryStatus.State != seed.Delivery:
			fail("seeded collector %s: delivery state = %q, want %q — the OpAMP vocabulary crosses verbatim (ADR-0004)", name, c.DeliveryStatus.State, seed.Delivery)
		}
	}
}

// checkUnknown holds the unknown-collector discipline (ADR-0008): asking
// for a collector nobody reported yields Known false with a cause on every
// capable reading, zero on every incapable one — and never an error, which
// the seam's signature already makes unutterable.
func checkUnknown(fail func(string, ...any), decl estate.Declaration, est estate.Estate, absent map[string]string) {
	if len(absent) == 0 {
		absent = map[string]string{"telecraft.estatetest.absent": "matches-nothing"}
	}
	c := est.Lookup(absent)
	for _, v := range readings(c) {
		switch {
		case !decl.Capable(v.kind):
			if v.known || v.populated || v.cause != "" {
				fail("unknown collector: incapable reading %q is not zero — incapable stays absent-with-declaration even for a collector nobody can see (ADR-0036 §1)", v.kind)
			}
		case v.known:
			fail("unknown collector: reading %q came back Known — not knowing is a normal state and must be reported honestly (ADR-0008)", v.kind)
		case v.cause == "":
			fail("unknown collector: reading %q carries no cause — the caller must learn why nothing is known", v.kind)
		case v.asOf.IsZero():
			fail("unknown collector: reading %q carries no as_of — 'we cannot see' is still a statement with a timestamp (ADR-0036 §2)", v.kind)
		}
	}
}

// checkStaleness holds the demotion rule (ADR-0036 §3) against a real
// reading: inside the horizon it survives evaluation, past the horizon it
// demotes to Known false with its payload gone. The arithmetic is the
// platform's, but running it here proves the provider's as_of and cadence
// make it computable.
func checkStaleness(fail func(string, ...any), decl estate.Declaration, est estate.Estate, seeds []Seed) {
	horizon := decl.RefreshCadence * estate.StaleTolerance
	if horizon <= 0 {
		return // the missing cadence is already a violation
	}
	for _, seed := range seeds {
		c := est.Lookup(seed.Identity)
		for _, v := range readings(c) {
			if !v.known || v.asOf.IsZero() {
				continue
			}
			name := estate.Fingerprint(c.Identity)
			if fresh := readingOf(c.ForEvaluation(decl, v.asOf.Add(horizon)), v.kind); !fresh.known {
				fail("collector %s reading %q was demoted while still inside the staleness horizon — demotion is for silence, not for freshness (ADR-0036 §3)", name, v.kind)
			}
			stale := readingOf(c.ForEvaluation(decl, v.asOf.Add(horizon+time.Second)), v.kind)
			switch {
			case stale.known:
				fail("collector %s reading %q survived evaluation past the staleness horizon — a stale reading never feeds a fresh-looking verdict (ADR-0036 §3)", name, v.kind)
			case stale.populated:
				fail("collector %s reading %q was demoted but still carries its payload — nothing downstream may quietly use it", name, v.kind)
			case stale.cause == "":
				fail("collector %s reading %q was demoted without a cause", name, v.kind)
			}
			return // one real reading proves the arithmetic
		}
	}
	fail("no seeded collector carries a Known reading with as_of, so staleness demotion could not be exercised — seed at least one populated reading")
}

// reading is one reading flattened for rule-checking, whatever its kind.
type reading struct {
	kind      estate.ReadingKind
	known     bool
	cause     string
	asOf      time.Time
	populated bool
}

// readings flattens a collector's three readings for uniform checks.
func readings(c estate.Collector) []reading {
	return []reading{
		{estate.EffectiveKind, c.Effective.Known, c.Effective.Cause, c.Effective.AsOf,
			len(c.Effective.Pipelines) > 0},
		{estate.HealthKind, c.Health.Known, c.Health.Cause, c.Health.AsOf,
			!reflect.DeepEqual(c.Health.Component, estate.ComponentHealth{})},
		{estate.DeliveryStatusKind, c.DeliveryStatus.Known, c.DeliveryStatus.Cause, c.DeliveryStatus.AsOf,
			c.DeliveryStatus.State != "" || len(c.DeliveryStatus.ConfigHash) > 0 || c.DeliveryStatus.Error != ""},
	}
}

// readingOf picks one kind's flattened reading.
func readingOf(c estate.Collector, kind estate.ReadingKind) reading {
	for _, v := range readings(c) {
		if v.kind == kind {
			return v
		}
	}
	return reading{}
}

// containsAll reports whether every asked pair equals the reported one.
func containsAll(reported, asked map[string]string) bool {
	for k, v := range asked {
		if reported[k] != v {
			return false
		}
	}
	return true
}
