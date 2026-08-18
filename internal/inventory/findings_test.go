package inventory

import (
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/ownership"
)

func one(t *testing.T, findings []Finding, class Class) Finding {
	t.Helper()
	var out []Finding
	for _, f := range findings {
		if f.Class == class {
			out = append(out, f)
		}
	}
	if len(out) != 1 {
		t.Fatalf("%d %s findings, want exactly one: %+v", len(out), class, findings)
	}
	return out[0]
}

// No inventory source and no declaration: no invented count, no teeth —
// the only finding is the neutral never_seen (ADR-0035 §2, §4).
func TestNoSourceNoTeeth(t *testing.T) {
	p := Population{Tier: "data-flow/edge", ShortfallSince: t0.Add(-time.Hour)}
	got := p.Findings(Config{}, t0)

	f := one(t, got, NeverSeen)
	if f.Grade != Neutral {
		t.Fatalf("never_seen grade = %q with no floor — neutrality is untouched without one (ADR-0035 §4)", f.Grade)
	}
	if len(got) != 1 {
		t.Fatalf("findings = %+v — with no floor the platform never invents a count or a shortfall", got)
	}
	if routed := DeliveryFindings(got); len(routed) != 0 {
		t.Fatalf("a neutral never_seen entered the roll-up: %+v — un-toothed neutral states stay excluded (P2's rule, ADR-0035 §6)", routed)
	}
}

// Floor > 0 and zero matches persisting past the window: never_seen
// escalates from neutral to violation-grade (ADR-0035 §4).
func TestNeverSeenEscalatesUnderAFloor(t *testing.T) {
	p := Population{
		Tier:           "data-flow/edge",
		Declared:       40,
		ShortfallSince: t0.Add(-10 * time.Minute),
	}
	f := one(t, p.Findings(Config{Grace: 5 * time.Minute}, t0), NeverSeen)
	if f.Grade != Violation {
		t.Fatalf("grade = %q, want violation — floor > 0 and zero matches persisted past the window (ADR-0035 §4)", f.Grade)
	}
	if !strings.Contains(f.Detail, "≥40") || !strings.Contains(f.Detail, "seen 0") {
		t.Fatalf("detail %q does not read as \"expected ≥40, seen 0\"", f.Detail)
	}
}

// Inside the grace window the same shortfall stays neutral: scale events
// have an honest transient (ADR-0035 §3).
func TestNeverSeenDampenedInsideGrace(t *testing.T) {
	p := Population{
		Tier:           "data-flow/edge",
		Declared:       40,
		ShortfallSince: t0.Add(-time.Minute),
	}
	f := one(t, p.Findings(Config{Grace: 5 * time.Minute}, t0), NeverSeen)
	if f.Grade != Neutral {
		t.Fatalf("grade = %q inside the grace window — a shortfall must persist before any finding raises (ADR-0035 §3)", f.Grade)
	}
}

// Collectors present but below the floor, persisting: under_populated,
// the sibling class — never a degree of never_seen (ADR-0035 §5).
func TestUnderPopulatedBelowFloor(t *testing.T) {
	p := Population{
		Tier:           "data-flow/edge",
		Derived:        Count{Known: true, AsOf: t0, Instances: 40},
		Seen:           12,
		EverSeen:       true,
		ShortfallSince: t0.Add(-10 * time.Minute),
	}
	got := p.Findings(Config{Grace: 5 * time.Minute}, t0)
	f := one(t, got, UnderPopulated)
	if f.Grade != Violation {
		t.Fatalf("grade = %q, want violation — the floor is unmet (ADR-0035 §5)", f.Grade)
	}
	if !strings.Contains(f.Detail, "≥40") || !strings.Contains(f.Detail, "seen 12") {
		t.Fatalf("detail %q does not read as \"expected ≥40, seen 12\"", f.Detail)
	}
	for _, x := range got {
		if x.Class == NeverSeen {
			t.Fatal("a Tier with collectors present raised never_seen — the classes are siblings, never conflated (ADR-0035 §5)")
		}
	}
}

// A Tier that once had collectors and dropped to zero is under-populated,
// not never_seen: it has readings and history, and "0 of 40 running" must
// not read as "nothing ever existed".
func TestDroppedToZeroIsUnderPopulated(t *testing.T) {
	p := Population{
		Tier:           "data-flow/edge",
		Declared:       40,
		Seen:           0,
		EverSeen:       true,
		ShortfallSince: t0.Add(-10 * time.Minute),
	}
	one(t, p.Findings(Config{Grace: 5 * time.Minute}, t0), UnderPopulated)
}

// Surplus is never a finding (ADR-0035 §2).
func TestSurplusIsNeverAFinding(t *testing.T) {
	p := Population{
		Tier:     "data-flow/edge",
		Derived:  Count{Known: true, AsOf: t0, Instances: 40},
		Declared: 40,
		Seen:     120,
		EverSeen: true,
	}
	if got := p.Findings(Config{}, t0); len(got) != 0 {
		t.Fatalf("findings = %+v — expectations are floors, never equalities", got)
	}
}

// An unpersisted shortfall raises nothing: the judgement never fires from
// a single instant (ADR-0035 §3).
func TestUnderPopulatedDampenedInsideGrace(t *testing.T) {
	p := Population{
		Tier:           "data-flow/edge",
		Derived:        Count{Known: true, AsOf: t0, Instances: 40},
		Seen:           12,
		EverSeen:       true,
		ShortfallSince: t0.Add(-time.Minute),
	}
	if got := p.Findings(Config{Grace: 5 * time.Minute}, t0); len(got) != 0 {
		t.Fatalf("findings = %+v inside the grace window — scale events have an honest transient", got)
	}
}

// When both sources exist they are compared, not silently resolved: a
// declared floor above the live derived count is a visible advisory
// (ADR-0035 §2), while the floor itself resolves derived.
func TestDeclaredAboveDerivedIsAVisibleConflict(t *testing.T) {
	p := Population{
		Tier:     "data-flow/edge",
		Derived:  Count{Known: true, AsOf: t0, Instances: 12},
		Declared: 40,
		Seen:     12,
		EverSeen: true,
	}
	got := p.Findings(Config{}, t0)
	f := one(t, got, FloorConflict)
	if f.Grade != Advisory {
		t.Fatalf("floor_conflict grade = %q, want advisory", f.Grade)
	}
	if f.Floor.Source != FloorDerived || f.Floor.Min != 12 {
		t.Fatalf("resolved floor = %+v — derived outranks declared (ADR-0035 §2)", f.Floor)
	}
	// seen 12 meets the derived floor of 12: no shortfall finding rides
	// along with the conflict.
	if len(got) != 1 {
		t.Fatalf("findings = %+v, want the conflict alone", got)
	}
}

// An aged neutral never_seen is flagged as the stale-config signal
// (ADR-0035 §7) — still neutral, still excluded from the roll-up.
func TestAgedNeverSeenIsAStaleConfigSignal(t *testing.T) {
	p := Population{Tier: "data-flow/edge", FirstWatched: t0.Add(-91 * 24 * time.Hour)}
	f := one(t, p.Findings(Config{}, t0), NeverSeen)
	if !f.StaleConfig {
		t.Fatal("a 91-day never_seen is not flagged — the aged neutral case is the stale-config signal (ADR-0035 §7)")
	}
	if f.Grade != Neutral {
		t.Fatalf("grade = %q — the stale-config signal is a presentation affordance, never a new finding class", f.Grade)
	}
	if !strings.Contains(f.Detail, "91 days") {
		t.Fatalf("detail %q does not surface the age", f.Detail)
	}
	if routed := DeliveryFindings([]Finding{f}); len(routed) != 0 {
		t.Fatalf("the stale-config signal entered the roll-up: %+v", routed)
	}
}

// A young never_seen with an age base is not yet stale config.
func TestYoungNeverSeenIsNotStaleConfig(t *testing.T) {
	p := Population{Tier: "data-flow/edge", FirstWatched: t0.Add(-24 * time.Hour)}
	if f := one(t, p.Findings(Config{}, t0), NeverSeen); f.StaleConfig {
		t.Fatal("a day-old never_seen was flagged as stale config")
	}
}

// The Damper starts the clock on the first sub-floor observation, holds
// the onset across observations, and clears on recovery so a fresh
// shortfall serves its full grace window (ADR-0035 §3).
func TestDamperTracksShortfallOnset(t *testing.T) {
	d := NewDamper()
	floor := Floor{Source: FloorDerived, Min: 40}

	if since := d.Observe("data-flow/edge", 40, floor, t0); !since.IsZero() {
		t.Fatalf("a Tier meeting its floor has onset %v, want none", since)
	}
	if since := d.Observe("data-flow/edge", 12, floor, t0.Add(time.Minute)); !since.Equal(t0.Add(time.Minute)) {
		t.Fatalf("first sub-floor observation: onset = %v, want %v", since, t0.Add(time.Minute))
	}
	if since := d.Observe("data-flow/edge", 13, floor, t0.Add(2*time.Minute)); !since.Equal(t0.Add(time.Minute)) {
		t.Fatalf("continuing shortfall: onset = %v, want the original %v", since, t0.Add(time.Minute))
	}
	if since := d.Observe("data-flow/edge", 40, floor, t0.Add(3*time.Minute)); !since.IsZero() {
		t.Fatalf("recovered Tier still holds onset %v", since)
	}
	if since := d.Observe("data-flow/edge", 12, floor, t0.Add(4*time.Minute)); !since.Equal(t0.Add(4 * time.Minute)) {
		t.Fatalf("a fresh shortfall after recovery: onset = %v, want %v — the full grace window applies again", since, t0.Add(4*time.Minute))
	}
	if since := d.Observe("data-flow/edge", 0, Floor{}, t0.Add(5*time.Minute)); !since.IsZero() {
		t.Fatal("a floor-less Tier accrued a shortfall — no floor, no teeth (ADR-0035 §2)")
	}
}

// Escalated findings enter the ADR-0017 roll-up as Tier-attached
// delivery-kind findings (ADR-0035 §6), and they route: the Rollup
// machinery accepts them as authored-Tier subjects.
func TestDeliveryFindingsJoinTheRollup(t *testing.T) {
	findings := []Finding{
		{Class: NeverSeen, Tier: "data-flow/edge", Grade: Violation, Detail: "expected ≥40, seen 0"},
		{Class: FloorConflict, Tier: "data-flow/edge", Grade: Advisory, Detail: "declared 40 above derived 12"},
		{Class: NeverSeen, Tier: "data-flow/gateway", Grade: Neutral},
	}
	routed := DeliveryFindings(findings)
	if len(routed) != 2 {
		t.Fatalf("%d roll-up findings, want 2 (the neutral one excluded): %+v", len(routed), routed)
	}
	for _, f := range routed {
		if f.Kind != ownership.Delivery {
			t.Fatalf("kind = %q — a population shortfall is a delivery problem (ADR-0035 §6)", f.Kind)
		}
		if f.Subject.Kind != ownership.KindTier || f.Subject.ID != "data-flow/edge" {
			t.Fatalf("subject = %+v — population findings are Tier-attached", f.Subject)
		}
	}
	if routed[0].Grade != ownership.Violation || routed[1].Grade != ownership.Advisory {
		t.Fatalf("grades = %q, %q — want violation then advisory", routed[0].Grade, routed[1].Grade)
	}
}
