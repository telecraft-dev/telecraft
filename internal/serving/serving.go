// Package serving is the stateless OpAMP serving path (REQ-040, ADR-0013):
// a collector connects and reports identifying attributes; the server
// matches them against the selectors held in git and serves the rendered
// artefact at that path, remembering nothing. The artefact carries its own
// identity — the renderer stamped the commit SHA into it, so "which commit
// is this running" is read from the collector, never remembered about it.
// Removing the server loses delivery, never the record.
//
// The serving path may hold exactly three things, all rebuildable, none
// durable (ADR-0032 §1): the repo Snapshot — the fetched estate at
// last-known head plus the selector index compiled from it, refreshed by
// poll so the fetch interval is the bounded staleness; the per-connection
// layer-1 digest of each connected collector's last-reported effective
// config (ADR-0005), in process memory only, dying with the connection or
// the process; and nothing else. Artefact choice is a pure function of
// (head, reported attributes), which is why N replicas behind a load
// balancer are N independent read-only clones needing no coordination
// (ADR-0032 §2), and why restart is a non-event by construction.
//
// The server never serves an empty config map (REQ-042, ADR-0010 rule 6) —
// the Supervisor would report APPLIED-and-healthy while running nothing. A
// collector matching no selector receives the Unmatched artefact
// (ADR-0030), rendered non-empty for exactly this purpose.
package serving

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/telecraft-dev/telecraft/internal/renderer"
)

// Snapshot is the first of ADR-0032's two caches: the estate repo at
// last-known head, compiled to the selector index the serving decision
// reads. A cache *of* git, never a fork of it — loss is a re-fetch.
type Snapshot struct {
	// Commit is the head SHA the snapshot was taken at; empty when the
	// source is a plain directory outside any git history. Identity still
	// travels in the artefacts themselves (ADR-0013) — this field only
	// names what was fetched.
	Commit string

	// entries is the compiled selector index in Tier-id order — the stable
	// order is what makes an equal-specificity tie deterministic.
	entries []entry

	// unmatched is the Unmatched artefact (ADR-0030), served to a
	// collector matching no selector.
	unmatched []byte
}

// entry is one Tier's line in the selector index: the selector authored on
// the Tier and the rendered artefact bytes served on a match.
type entry struct {
	tier     string
	selector map[string]string
	artefact []byte
}

// LoadSnapshot compiles the estate checkout at root into a Snapshot,
// recording commit as the head it was taken at. It fails closed on
// anything that would make serving lie: an invalid topology, a
// selector-carrying Tier whose rendered artefact is missing or empty, or a
// missing Unmatched artefact — a refused snapshot leaves the server on the
// previous head, which is bounded staleness, never mis-delivery.
func LoadSnapshot(root, commit string) (*Snapshot, error) {
	topo, err := renderer.LoadTopology(root)
	if err != nil {
		return nil, err
	}

	snap := &Snapshot{Commit: commit}
	for _, tier := range topo.SortedTiers() {
		if len(tier.Selector) == 0 {
			// No selector, no served collectors: the Tier is reachable
			// only by the git-delivered path (REQ-041).
			continue
		}
		rel := "rendered/" + tier.Team + "/" + tier.Name + ".yaml"
		artefact, err := readArtefact(root, rel)
		if err != nil {
			return nil, fmt.Errorf("tier %q: %w", tier.ID(), err)
		}
		snap.entries = append(snap.entries, entry{
			tier:     tier.ID(),
			selector: tier.Selector,
			artefact: artefact,
		})
	}

	snap.unmatched, err = readArtefact(root, renderer.UnmatchedArtefactPath)
	if err != nil {
		return nil, fmt.Errorf("no Unmatched artefact — the renderer emits it unconditionally, so this estate needs a re-render (ADR-0030): %w", err)
	}
	return snap, nil
}

// readArtefact reads one rendered artefact, refusing emptiness at the
// source: an empty file here would otherwise become an empty config map on
// the wire (ADR-0010 rule 6).
func readArtefact(root, rel string) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("%s is empty — an empty artefact can never be served (ADR-0010 rule 6)", rel)
	}
	return raw, nil
}

// Match is one serving decision: the artefact to serve and where it came
// from. Exactly one of Tier or Unmatched is meaningful, and Artefact is
// non-empty in both cases — the Unmatched artefact exists so that "no
// match" never becomes "no config" (ADR-0030).
type Match struct {
	// Tier is the matched Tier's team-qualified id; empty when unmatched.
	Tier string

	// Artefact is the rendered config to serve, byte-for-byte as the
	// renderer committed it — the commit stamp rides inside (ADR-0013).
	Artefact []byte

	// Unmatched marks a collector matching no selector: it receives the
	// Unmatched artefact and becomes maximally visible, never silent.
	Unmatched bool
}

// Match resolves reported identifying attributes to the artefact this head
// serves: a pure function of (head, attributes), no state consulted or
// written (ADR-0032). A selector matches when every authored pair equals
// the reported attribute; the most specific satisfied selector (most
// pairs) wins, and an equal-specificity tie resolves to the first Tier in
// id order — deterministic, so replicas cannot disagree.
func (s *Snapshot) Match(attrs map[string]string) Match {
	best := -1
	var won *entry
	for i := range s.entries {
		e := &s.entries[i]
		if !satisfies(e.selector, attrs) {
			continue
		}
		if len(e.selector) > best {
			best = len(e.selector)
			won = e
		}
	}
	if won == nil {
		return Match{Artefact: s.unmatched, Unmatched: true}
	}
	return Match{Tier: won.tier, Artefact: won.artefact}
}

// satisfies reports whether every selector pair equals the reported
// attribute — equality over all pairs, no wildcards.
func satisfies(selector, attrs map[string]string) bool {
	for k, v := range selector {
		if attrs[k] != v {
			return false
		}
	}
	return true
}
