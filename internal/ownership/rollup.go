package ownership

import "fmt"

// Score is the roll-up for one finding kind at one node of the tree
// (ADR-0017 §3): a passing-over-counted ratio, a worst-outcome badge, and
// the waived count always alongside. The ratio is kept as an integer pair,
// deliberately: nothing here, or anywhere in this package, collapses
// kinds into each other or into a single blended number.
type Score struct {
	Passing int
	Counted int
	Worst   Grade
	Waived  int
}

// RoutedFinding pairs a finding with the Owner it routed to.
type RoutedFinding struct {
	Finding
	Owner Owner
}

// Rollup is one team's view: the set of findings routed to owners in its
// subtree (ADR-0017 §2), scored per kind. That set includes kinds beyond
// service verdicts: a parent team's view is bigger than the sum of its
// services, and that is the point. Waived findings appear in Findings with
// their diagnosis intact; they are absent only from the counted ratio.
type Rollup struct {
	Team     TeamID
	Scores   map[FindingKind]Score
	Findings []RoutedFinding
}

// Rollup computes one team's roll-up over a set of findings. It is
// computable at every level: the same call at a leaf, a mid-level team or
// the root scores that node's subtree. A finding that fails to route or
// carries an invalid kind or grade is an error, never silently dropped
// from a denominator.
func (e Estate) Rollup(team TeamID, findings []Finding) (Rollup, error) {
	subtree, err := e.Tree.Subtree(team)
	if err != nil {
		return Rollup{}, err
	}
	inSubtree := map[TeamID]bool{}
	for _, id := range subtree {
		inSubtree[id] = true
	}

	out := Rollup{Team: team, Scores: map[FindingKind]Score{}}
	for _, f := range findings {
		if !f.Kind.Valid() {
			return Rollup{}, fmt.Errorf("finding about %s %q has unknown kind %q. Use one of service_conformance, delivery, component_health, or expectation", f.Subject.Kind, f.Subject.ID, f.Kind)
		}
		if !f.Grade.Valid() {
			return Rollup{}, fmt.Errorf("finding about %s %q has unknown grade %q. Use one of pass, neutral, advisory, or violation", f.Subject.Kind, f.Subject.ID, f.Grade)
		}
		owner, err := e.OwnerOf(f.Subject)
		if err != nil {
			return Rollup{}, err
		}
		if !inSubtree[owner.Team] {
			continue
		}

		out.Findings = append(out.Findings, RoutedFinding{Finding: f, Owner: owner})
		s, seen := out.Scores[f.Kind]
		if !seen {
			// A kind whose every finding is waived keeps a pass badge: the
			// waived count alongside is what stops an exemption-heavy 100%
			// from hiding (ADR-0017).
			s.Worst = Pass
		}
		switch {
		case f.Waived:
			s.Waived++
		case f.Grade == Neutral:
			// Neutral is excluded from every denominator (ADR-0035 §6): it
			// stays in Findings, where a human reads it, and out of the
			// ratio, where it would either inflate a score nobody earned
			// or deflate one nobody lost.
		default:
			s.Counted++
			if f.Grade == Pass {
				s.Passing++
			}
			if severity[f.Grade] > severity[s.Worst] {
				s.Worst = f.Grade
			}
		}
		out.Scores[f.Kind] = s
	}
	return out, nil
}
