package inventory

import "time"

// FloorSource names where a Tier's population floor came from, ranked
// derived > declared > absent (ADR-0035 §2).
type FloorSource string

const (
	// FloorDerived is a live answer from an InventoryProvider: it floats
	// with the autoscaler by construction, so it outranks any static
	// declaration.
	FloorDerived FloorSource = "derived"

	// FloorDeclared is the Tier's authored min_expected — reviewable in
	// git, for substrates with no API ("at least 12 boxes in that rack").
	FloorDeclared FloorSource = "declared"

	// FloorAbsent is no floor at all: no provider, no declaration, no
	// teeth — never_seen keeps its neutrality and nobody is forced to
	// guess (ADR-0035 §2).
	FloorAbsent FloorSource = ""
)

// Floor is one Tier's resolved population floor. Expectations are floors,
// never equalities: the only finding a Floor can back is a shortfall, and
// surplus is never a finding (ADR-0035 §2).
type Floor struct {
	Source FloorSource

	// Min is the floor: at least this many instances should match. Zero
	// with FloorDerived is a real answer — the substrate says nothing
	// should exist — and carries no teeth, exactly like absence.
	Min int

	// AsOf is when a derived floor was counted; zero on declared and
	// absent floors, which have no instant.
	AsOf time.Time
}

// ResolveFloor ranks the two possible sources for one Tier (ADR-0035 §2):
// a Known derived count wins outright — including a derived zero, which
// is the substrate honestly expecting nothing; a positive declared
// min_expected stands in when the derived count is absent or unknown; and
// with neither, the floor is absent and the platform invents no count.
// Pass the derived count through ForEvaluation first: a stale count must
// never float a fresh-looking floor.
//
// Resolution never silences the comparison: when both sources exist they
// are compared by the judgement (Population.Findings), because a declared
// floor above live reality usually means a shrunk fleet someone should
// notice.
func ResolveFloor(derived Count, declared int) Floor {
	if derived.Known {
		return Floor{Source: FloorDerived, Min: derived.Instances, AsOf: derived.AsOf}
	}
	if declared > 0 {
		return Floor{Source: FloorDeclared, Min: declared}
	}
	return Floor{}
}
