package conformance

// The registry facet of library_drift: the last clause of ADR-0034 §2.
// A schema-conformance requirement pins a Schema Registry version by
// default, so an activation cannot silently move a Service's score; the
// price of the pin is that the estate's bar can move on without the Service
// noticing. This file notices: where the scope passes the pinned version
// and fails the active one, the verdict is library_drift, one more facet on
// the finding kind ADR-0026 defined, no new kind and no new outcome.
//
// The facet lives here rather than in internal/drift, where the other two
// are judged, because the diagnosis is drawn from Observed readings per
// row. internal/drift judges authored config at head and holds no telemetry
// evidence; this package gathers evidence per Schema Registry version
// already, so the second reading is a second judgement over what is in
// hand, not new machinery. internal/drift keeps the facet vocabulary
// complete and points back here.
//
// The drift arm judges the scored verdict alone. The improvement and
// information findings that ride alongside it are advice, and advice that
// has moved is not a bar that has moved: the drift diagnosis exists to say
// a Service stopped clearing a floor it is not yet held to, and only the
// violation-grade verdict is a floor.

import (
	"fmt"

	"github.com/telecraft-dev/telecraft/internal/requirements"
)

// FacetRegistry is the facet a library_drift finding carries when the
// referenced object is the Schema Registry: a pinned reference behind the
// active version, with the scope passing the pin and failing the active
// version (ADR-0034 §2, ADR-0026 §7). It is the one facet this package
// writes; the requirement and component facets are judged from the authored
// trees by internal/drift.
const FacetRegistry = "registry"

// schemaDrift is the drift arm of the landed schema judgement: it takes the
// findings the pinned version produced and, where the scope passes its pin
// while provably failing the active version, rewrites the scored verdict as
// library_drift with the registry facet.
//
// It runs only where every one of these holds, and each guard is a decision
// rather than a shortcut:
//
//   - The reference pins. A tracking reference is judged against the active
//     version already (ADR-0026 §1): it has no pin to fall behind, so
//     nothing here touches it.
//   - The evidence names an active version, and it is not the pin itself.
//     Told no designation, the arm judges nothing: which version is active
//     is an activation decision (ADR-0020 §6), never this package's guess.
//   - The pinned verdict is compliant. A scope failing its own pin is the
//     ordinary failure, reported through the existing path unchanged, and a
//     scope whose pinned verdict is unknown proved nothing to drift from
//     (ADR-0026 §6: drift means passed the pinned version, failed the
//     current one, and nothing else).
//   - The active verdict is a provable failure: misconfigured (arrived in
//     the wrong shape) or not_delivered (a group the active scope reaches
//     carried nothing). An unknown active verdict raises nothing, because
//     calling a reading gap drift would manufacture a red out of a blind
//     spot (ADR-0008).
func schemaDrift(req requirements.Requirement, pinned []Finding, ev Evidence) []Finding {
	a := *req.Schema
	if a.Tracking() {
		return pinned
	}
	active, activeRef, ok := ev.Schema.ActiveRegistry()
	if !ok || activeRef == a.RegistryVersion {
		return pinned
	}

	verdict := scoredIndex(pinned)
	if verdict < 0 || pinned[verdict].Outcome != Compliant {
		return pinned
	}

	current, judged := scoredFinding(judgeSchemaAgainst(req, active, ev))
	if !judged || (current.Outcome != Misconfigured && current.Outcome != NotDelivered) {
		return pinned
	}

	pinned[verdict] = driftFinding(pinned[verdict], a.RegistryVersion, activeRef, current)
	return pinned
}

// driftFinding writes the library_drift verdict from the two judgements: the
// pin it passes, the active version it fails, and what the active version
// found wanting. The gap detail and the registry-derived fix are the active
// judgement's own (ADR-0034 §7): the group, the attribute, its declared type
// and level, and upstream's migration note where the registry carries one.
func driftFinding(was Finding, pin, active string, current Finding) Finding {
	f := Finding{
		Requirement: was.Requirement,
		Grade:       was.Grade,
		Outcome:     LibraryDrift,
		Facet:       FacetRegistry,
		Missing:     current.Missing,
	}
	f.Detail = append(f.Detail,
		fmt.Sprintf("passes Schema Registry version %s, which this requirement pins, and fails %s, the active version", pin, active))
	f.Detail = append(f.Detail, current.Detail...)
	f.Remediation = driftRemediation(pin, active, current)
	return f
}

// driftRemediation is the fix, in the order the reader can act on it: close
// the gap the active version names, then move the pin. The first half is the
// active judgement's registry-derived remediation; the fallback covers the
// one failing shape that writes none, a group the active scope reaches that
// carried nothing.
func driftRemediation(pin, active string, current Finding) string {
	fix := current.Remediation
	if fix == "" {
		fix = fmt.Sprintf("Nothing arrived for part of what Schema Registry version %s covers in this scope; the detail names it. Get the service emitting it.", active)
	}
	return fmt.Sprintf("%s Then move this requirement's pin from Schema Registry version %s to %s in a PR.", fix, pin, active)
}

// scoredIndex locates the finding that is the requirement's verdict: the
// one scored finding among the level findings that ride alongside it.
func scoredIndex(findings []Finding) int {
	for i, f := range findings {
		if f.Scored() {
			return i
		}
	}
	return -1
}

// scoredFinding is scoredIndex for a judgement this arm only reads.
func scoredFinding(findings []Finding) (Finding, bool) {
	i := scoredIndex(findings)
	if i < 0 {
		return Finding{}, false
	}
	return findings[i], true
}
