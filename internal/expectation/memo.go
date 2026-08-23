package expectation

import "sync"

// Memo is the engine's only cache: in-memory memoisation of Derive,
// keyed by SHA, coarser than ADR-0038 §3's "(SHA, Tier) at most", which
// is the permitted direction. A SHA pins the authored trees (ADR-0013),
// so the key is honest: two Sources with the same SHA carry the same
// trees and derive the same Set.
//
// The memo is confirmed loseable: it is evaluator-internal, appears on
// no cache list (ADR-0032's closed list stands unamended), and losing it
// costs one recomputation, never an answer. Nothing is ever persisted;
// a committed expectations file is a drift surface against the artefact
// it restates, and derived-never-authored is easier to defend if the
// claim set never looks like a file.
type Memo struct {
	mu    sync.Mutex
	bySHA map[string]Set
}

// NewMemo builds an empty Memo. One Memo serves an evaluator; losing it
// (or simply building a fresh one) changes cost, never content.
func NewMemo() *Memo {
	return &Memo{bySHA: map[string]Set{}}
}

// Derive returns the Set for the Source, computing it on the first call
// per SHA and replaying it after. The replayed Set is the same value a
// fresh derivation produces, the memoisation invariant the tests pin.
func (m *Memo) Derive(src Source) Set {
	m.mu.Lock()
	defer m.mu.Unlock()
	if set, ok := m.bySHA[src.SHA]; ok {
		return set
	}
	set := Derive(src)
	m.bySHA[src.SHA] = set
	return set
}
