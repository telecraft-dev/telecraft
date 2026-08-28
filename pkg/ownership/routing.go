package ownership

import "fmt"

// FindingKind is one of the ADR-0017 roll-up columns. Kinds are never
// blended with each other; a Rollup scores each on its own.
type FindingKind string

const (
	ServiceConformance FindingKind = "service_conformance"
	Delivery           FindingKind = "delivery"
	ComponentHealth    FindingKind = "component_health"

	// Expectation carries claim failures from the Expectation engine
	// (ADR-0038 §5): unbacked data claims Service-attached and
	// advisory-grade, pipeline claims Tier-attached and
	// violation-capable after dampening. Its own roll-up column, never
	// blended. Exemptions apply unmodified.
	Expectation FindingKind = "expectation"
)

func (k FindingKind) Valid() bool {
	switch k {
	case ServiceConformance, Delivery, ComponentHealth, Expectation:
		return true
	}
	return false
}

// Grade is the weight one finding carries. Breach is graded, never a block:
// advisory is worth surfacing, violation is the floor unmet (ADR-0035
// wording adopted here).
//
// The four values are the one grade vocabulary the platform has. Producers
// name their own concepts in it rather than minting synonyms: ADR-0034 §3's
// "improvement" is advisory, and its "information" is neutral. A second
// spelling of the same three weights would give the console two ladders to
// reconcile, which is exactly the divergence this type exists to prevent.
type Grade string

const (
	Pass Grade = "pass"

	// Neutral is a finding that is worth reporting and settles nothing: it
	// is not a pass, and it is excluded from every denominator (ADR-0035
	// §6). Reporting an opt-in attribute nobody adopted is the shape of
	// it: counting it as a pass would inflate a ratio nobody earned, and
	// counting it as a failure would demand something nobody asked for.
	Neutral Grade = "neutral"

	Advisory  Grade = "advisory"
	Violation Grade = "violation"
)

func (g Grade) Valid() bool {
	switch g {
	case Pass, Neutral, Advisory, Violation:
		return true
	}
	return false
}

// severity orders grades for the worst-outcome badge. Neutral ranks with
// pass: it never darkens a badge, because it says nothing is wrong.
var severity = map[Grade]int{Pass: 0, Neutral: 0, Advisory: 1, Violation: 2}

// Subject identifies the object a finding is about.
type Subject struct {
	Kind ObjectKind
	ID   string

	// Tier is set only on a collector subject: the Tier the collector
	// matched into by selector, from which it inherits its owner. A
	// collector is never owned directly (ADR-0016).
	Tier string
}

// Finding is one unit of compliance data, routed by its Subject. Findings
// are data: this package neither produces nor judges them, it decides who
// is accountable and how they aggregate.
type Finding struct {
	Kind    FindingKind
	Subject Subject
	Grade   Grade

	// Detail is the diagnosis. A waiver never erases it.
	Detail string

	// Waived marks an Exemption applied to this finding: it waives the
	// count, never the diagnosis, and stays visible in every roll-up.
	Waived bool
}

// OwnerOf resolves the Owner a finding about s routes to: the owner of the
// object the finding is about, not the owner of anything it renders into
// (ADR-0016). A collector subject routes through the Tier it matched into.
// Every failure to resolve is an error: a finding that routes nowhere is
// the silent failure this package exists to refuse.
func (e Estate) OwnerOf(s Subject) (Owner, error) {
	if s.Kind == KindCollector {
		if s.Tier == "" {
			return Owner{}, fmt.Errorf("collector %q names no tier. A collector inherits its owner from the Tier it matched into", s.ID)
		}
		tier, ok := e.Objects[Ref{Kind: KindTier, ID: s.Tier}]
		if !ok {
			return Owner{}, fmt.Errorf("collector %q matched tier %q, which is not an authored Tier in this estate", s.ID, s.Tier)
		}
		return e.Tree.Owners[tier.Owner], nil
	}

	if s.Tier != "" {
		return Owner{}, fmt.Errorf("%s %q carries a tier, but only a collector subject routes through one", s.Kind, s.ID)
	}
	obj, ok := e.Objects[Ref{Kind: s.Kind, ID: s.ID}]
	if !ok {
		return Owner{}, fmt.Errorf("no authored %s %q in this estate, so the finding routes nowhere", s.Kind, s.ID)
	}
	return e.Tree.Owners[obj.Owner], nil
}
