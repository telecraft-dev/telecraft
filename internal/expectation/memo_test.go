package expectation

import (
	"reflect"
	"testing"
)

// Recompute-from-scratch equals the memoised result, and the memo is
// provably loseable: dropping it (a fresh Memo) changes cost, never
// content (ADR-0038 §3, issue #34 AC).
func TestMemoisedEqualsRecomputedAndIsLoseable(t *testing.T) {
	src := fixtureSource(t)

	memo := NewMemo()
	first := memo.Derive(src)  // computes
	replay := memo.Derive(src) // replays

	scratch := Derive(src)
	if !reflect.DeepEqual(first, scratch) {
		t.Error("memoised result differs from a from-scratch derivation")
	}
	if !reflect.DeepEqual(replay, scratch) {
		t.Error("replayed result differs from a from-scratch derivation")
	}

	// Losing the memo: a fresh one re-derives the identical Set. Nothing
	// was persisted, so nothing could have been lost but time.
	lost := NewMemo()
	if !reflect.DeepEqual(lost.Derive(src), scratch) {
		t.Error("a fresh Memo derives a different Set: the memo must be loseable with no change in content")
	}
}

// The memo keys by SHA: another commit is another derivation, never a
// replay of the old one.
func TestMemoKeysBySHA(t *testing.T) {
	src := fixtureSource(t)
	memo := NewMemo()
	memo.Derive(src)

	other := src
	other.SHA = "0000000000000000000000000000000000000000"
	set := memo.Derive(other)
	if set.SHA != other.SHA {
		t.Errorf("memo replayed SHA %s for a Source at %s: the key is the commit", set.SHA, other.SHA)
	}
	for _, c := range set.Claims {
		if c.SHA != other.SHA {
			t.Fatalf("claim %s carries SHA %s, want %s", c.Key(), c.SHA, other.SHA)
		}
	}
}
