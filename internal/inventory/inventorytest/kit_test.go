package inventorytest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/inventory"
)

var t0 = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

// fake is a controllable InventoryProvider: a conforming one by default,
// with knobs each test twists to prove the kit catches the breakage.
type fake struct {
	cadence time.Duration
	counts  map[string]int // fingerprint → instances

	answerEmpty   bool // invent a count for the empty selector
	answerAll     bool // invent a count for any unknown selector
	dropAsOf      bool // omit the timestamp
	dropCause     bool // omit the cause on unknown counts
	payloadOnMiss bool // carry Instances alongside Known false
	offBy         int  // derail every count by this much
}

func (f *fake) Name() string { return "fake" }

func (f *fake) Declaration() inventory.Declaration {
	return inventory.Declaration{RefreshCadence: f.cadence}
}

func (f *fake) Expected(_ context.Context, selector map[string]string) inventory.Count {
	c := inventory.Count{}
	if !f.dropAsOf {
		c.AsOf = t0
	}
	n, seeded := f.counts[estate.Fingerprint(selector)]
	known := seeded
	if len(selector) == 0 {
		known = f.answerEmpty
	} else if !seeded {
		known = f.answerAll
	}
	if known {
		c.Known = true
		c.Instances = n + f.offBy
		return c
	}
	if !f.dropCause {
		c.Cause = "the harness arranged no answer for this selector"
	}
	if f.payloadOnMiss {
		c.Instances = 7
	}
	return c
}

func conforming() *fake {
	return &fake{
		cadence: time.Minute,
		counts: map[string]int{
			estate.Fingerprint(map[string]string{"telecraft.tier": "edge"}):    40,
			estate.Fingerprint(map[string]string{"telecraft.tier": "nothing"}): 0,
		},
	}
}

func kitFor(p inventory.Provider) Kit {
	return Kit{
		Provider: p,
		Seeded: []Seed{
			{Selector: map[string]string{"telecraft.tier": "edge"}, Instances: 40},
			{Selector: map[string]string{"telecraft.tier": "nothing"}, Instances: 0},
		},
		Unanswerable: map[string]string{"telecraft.inventorytest.absent": "unanswerable"},
	}
}

// A conforming implementation passes with no violations: the kit is the
// contract, and the contract is satisfiable.
func TestConformingProviderPasses(t *testing.T) {
	if got := Violations(context.Background(), kitFor(conforming())); len(got) != 0 {
		t.Fatalf("a conforming provider was found in violation:\n  %s", strings.Join(got, "\n  "))
	}
}

// expectViolation runs the kit and demands at least one violation naming
// the given fragment, proof the kit catches that exact breakage.
func expectViolation(t *testing.T, k Kit, fragment string) {
	t.Helper()
	got := Violations(context.Background(), k)
	for _, v := range got {
		if strings.Contains(v, fragment) {
			return
		}
	}
	t.Fatalf("no violation mentions %q; got:\n  %s", fragment, strings.Join(got, "\n  "))
}

func TestKitCatchesNoProvider(t *testing.T) {
	expectViolation(t, Kit{}, "no provider")
}

func TestKitCatchesNoSeeds(t *testing.T) {
	expectViolation(t, Kit{Provider: conforming()}, "no seeded selectors")
}

func TestKitCatchesMissingCadence(t *testing.T) {
	p := conforming()
	p.cadence = 0
	expectViolation(t, kitFor(p), "refresh cadence")
}

func TestKitCatchesWrongCount(t *testing.T) {
	p := conforming()
	p.offBy = 3
	expectViolation(t, kitFor(p), "want 40")
}

func TestKitCatchesUnknownForASeededSelector(t *testing.T) {
	p := conforming()
	delete(p.counts, estate.Fingerprint(map[string]string{"telecraft.tier": "nothing"}))
	expectViolation(t, kitFor(p), "never a blind spot")
}

func TestKitCatchesMissingAsOf(t *testing.T) {
	p := conforming()
	p.dropAsOf = true
	expectViolation(t, kitFor(p), "no as_of")
}

func TestKitCatchesInventedCountForEmptySelector(t *testing.T) {
	p := conforming()
	p.answerEmpty = true
	expectViolation(t, kitFor(p), "selector-less")
}

func TestKitCatchesInventedCountForUnanswerable(t *testing.T) {
	p := conforming()
	p.answerAll = true
	expectViolation(t, kitFor(p), "invented count")
}

func TestKitCatchesSilentGap(t *testing.T) {
	p := conforming()
	p.dropCause = true
	expectViolation(t, kitFor(p), "silent gap")
}

func TestKitCatchesPayloadOnUnknown(t *testing.T) {
	p := conforming()
	p.payloadOnMiss = true
	expectViolation(t, kitFor(p), "while Known is false")
}
