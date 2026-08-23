// Package drift detects library_drift: config in git that passes the
// version it claims or pins while failing the current one: "the goalposts
// moved and you haven't caught up" (ADR-0026 §6, REQ-025). It is one
// finding kind with two facets carried as data (ADR-0026 §7): Requirement (the bar moved, whether a versioned Requirement
// or a raised Service Class floor) and Component (a pinned reference behind
// the owning team's head).
// Every finding is repo-owned: the subject is authored config, the
// accountable party is the consuming object's owner, and the remedy is a
// version-diff review and a PR, never re-instrumentation (ADR-0004).
//
// library_drift is deliberately distinct from the two divergences it must
// never be conflated with. Delivery divergence (Intended × Declared, a
// collector running something other than what git holds) is a delivery
// status (ADR-0004), judged per collector, not here. And a rendered/ tree
// inconsistent with the authored sources is the CI recompute's mismatch
// (ADR-0028 §2), a mechanical invariant, not a diagnosis. This package
// judges exactly one thing: authored config at head against the current
// bar, where a version stamp or committed artefact records that the subject
// met an earlier one.
//
// Resolving the claimed or pinned version is structural, by decision. The
// working tree holds head content only (ADR-0003); what a Requirement or
// Component looked like at an older version lives in git history. Rather
// than reach into history, the detection reads the claim records the model
// already carries:
//
//   - A pinned Component reference behind the owning team's head is drift
//     by definition: "you reference version N; the world is at M" is the
//     whole diagnosis (ADR-0026 §7); no content is needed to make it.
//   - A satisfies claim is stamped by the composer at save against the
//     version it was judged on (ADR-0026 §4). When the Intended config
//     fails the requirement's current version and the stamp sits behind
//     it, the stamp is trusted as the passed-at-claim-time record and the
//     diagnosis is drift. The History seam is where content-level replay
//     plugs in: an implementation backed by the estate's git history (the
//     server's local clone cache, ADR-0032) resolves the requirement as it
//     stood at the claimed version, and a subject that fails that too is
//     demoted to the ordinary failure it is (ADR-0026 §6), the
//     evaluator's business, silent here. No git-backed implementation
//     ships yet; the seam exists so growing one never reshapes the
//     detection.
//   - A committed rendered artefact is the claim record for floors: main
//     is always consistent (ADR-0028 §2), so an artefact under rendered/
//     was reviewed and merged under the floor policy then in force.
//     Breaching the current policy reads as the floor having been raised
//     over standing config, exactly the case REQ-025 names. A Tier with
//     no committed artefact has claimed nothing and raises nothing here;
//     its breach surfaces at render (ADR-0023 §5).
package drift

import (
	"fmt"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/conformance"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/internal/requirements"
)

// Facet is the referenced-object kind a library_drift finding is about
// (ADR-0026 §7): one finding kind, one mental model, sliced by facet.
type Facet string

const (
	// FacetRequirement marks the bar having moved: a Requirement version
	// raised past a satisfies claim, or a Service Class floor raised over a
	// committed artefact (REQ-025).
	FacetRequirement Facet = "requirement"

	// FacetComponent marks a pinned shared-Component reference behind the
	// owning team's head: "component update available" (ADR-0026 §2).
	FacetComponent Facet = "component"
)

// Finding is one library_drift diagnosis. Every finding routes to the team
// that owns the consuming object (ADR-0016): drift is the consumer's to
// resolve, because only the consumer can review the version diff and adopt
// it; an owning team cannot force propagation (ADR-0026 §3).
type Finding struct {
	Facet Facet

	// Team and Owner are the routing target: the consuming Blueprint's or
	// Tier's owning team and accountable owner.
	Team  string
	Owner string

	// Tier and Environment are set when the drifted subject is a committed
	// rendered artefact (the floor case); empty on Blueprint-scoped drift.
	Tier        string
	Environment string

	Blueprint string

	// Lane is the signal lane where drift is per-signal (floors), or the
	// affected lanes joined for a reference pinned in several.
	Lane string

	Message     string
	Remediation string
}

// Nudge is the housekeeping case (ADR-0026 §6): a stale claim on a subject
// that passes both the claimed and the current version. Not an outcome,
// never counted: visible so the stamp gets bumped, nothing more.
type Nudge struct {
	Blueprint string
	Team      string
	Owner     string
	Message   string
}

// History resolves what a Requirement looked like at a claimed version:
// content that lives in git history, which the working tree does not hold
// (ADR-0003). A nil History trusts the composer's stamp (ADR-0026 §4) as
// the passed-at-claim-time record; see the package comment for the
// decision.
type History interface {
	// RequirementAt returns the requirement as it stood at the given
	// version, and whether history can resolve it. An unresolvable version
	// falls back to trusting the stamp; the seam degrades toward
	// reporting drift, never toward silence.
	RequirementAt(id string, version int) (requirements.Requirement, bool)
}

// Inputs is everything one detection reads: the authored trees, the current
// bar (library, floors, catalogue) and the committed artefact set. Like the
// render, detection is a pure function of its inputs.
type Inputs struct {
	Estate    blueprint.Estate
	Topology  renderer.Topology
	Catalogue *catalogue.Catalogue
	Floors    renderer.FloorPolicy
	Library   requirements.Library
	Rendered  Rendered

	// History is optional; nil trusts the claim stamps.
	History History
}

// Report is one detection's result: the drift findings, worst cases first
// in stable order, and the housekeeping nudges beside them.
type Report struct {
	Findings []Finding
	Nudges   []Nudge
}

// Detect runs the three detections (pinned references behind head,
// satisfies claims behind a moved Requirement, committed artefacts under a
// raised floor) over one estate. It fails closed on inputs it cannot
// judge: a detection that guessed would report an estate cleaner or dirtier
// than anyone knows it to be.
func Detect(in Inputs) (Report, error) {
	if in.Catalogue == nil {
		return Report{}, fmt.Errorf("no catalogue: floor checks need the active Catalogue")
	}
	if err := in.Floors.Validate(); err != nil {
		return Report{}, err
	}

	var rep Report
	for _, bp := range in.Estate.SortedBlueprints() {
		rep.Findings = append(rep.Findings, pinDrift(in.Estate, bp)...)
		findings, nudges := claimDrift(in, bp)
		rep.Findings = append(rep.Findings, findings...)
		rep.Nudges = append(rep.Nudges, nudges...)
	}

	floorFindings, err := floorDrift(in)
	if err != nil {
		return Report{}, err
	}
	rep.Findings = append(rep.Findings, floorFindings...)
	return rep, nil
}

// pinDrift finds shared-Component references pinned behind the owning
// team's head: the Component facet, structural by definition (ADR-0026
// §7). One finding per (Blueprint, pinned reference); the fix is one pin
// bump, however many lanes reference it.
func pinDrift(est blueprint.Estate, bp blueprint.Blueprint) []Finding {
	type behind struct {
		ref   blueprint.Reference
		head  int
		lanes []string
	}
	seen := map[string]*behind{}
	var order []string

	for _, s := range laneNames() {
		for _, e := range laneEntries(bp, s) {
			ref := e.Reference()
			// Tracking references follow head by opt-in and cannot drift;
			// locals have no head apart from the Blueprint's own file.
			if ref.Local() || ref.Track || ref.Pin == 0 {
				continue
			}
			c, ok := est.Component(ref.ID())
			if !ok || ref.Pin >= c.Version {
				// Missing and ahead-of-head are load-time reference
				// findings (internal/blueprint); at-head is current.
				continue
			}
			key := ref.String()
			b, dup := seen[key]
			if !dup {
				b = &behind{ref: ref, head: c.Version}
				seen[key] = b
				order = append(order, key)
			}
			b.lanes = append(b.lanes, s)
		}
	}

	var out []Finding
	for _, key := range order {
		b := seen[key]
		out = append(out, Finding{
			Facet:     FacetComponent,
			Team:      bp.Team,
			Owner:     bp.Owner,
			Blueprint: bp.ID(),
			Lane:      strings.Join(b.lanes, ", "),
			Message: fmt.Sprintf("pins %s, but the owning team's head is version %d. A component update is available",
				b.ref, b.head),
			Remediation: fmt.Sprintf("review the %s v%d→v%d config diff and bump the pin in a PR",
				b.ref.ID(), b.ref.Pin, b.head),
		})
	}
	return out
}

// claimDrift judges one Blueprint's version-stamped satisfies claims
// against the requirements at head: the Requirement facet. The Intended
// reading (the lanes compiled exactly as they would render) meets the
// same config-assertion checker the conformance cross uses (ADR-0004).
func claimDrift(in Inputs, bp blueprint.Blueprint) ([]Finding, []Nudge) {
	var findings []Finding
	var nudges []Nudge
	var intended *conformance.Effective

	for _, claim := range bp.Satisfies {
		req, ok := in.Library.Requirements[claim.Requirement]
		if !ok {
			// A claim naming no requirement in the library cannot drift:
			// there is no current version to fail. Dangling claims are an
			// authoring concern, not a drift diagnosis.
			continue
		}
		if claim.Version >= req.Version {
			// At head, failing is the ordinary failure ("you never
			// complied"), which is the evaluator's diagnosis, never drift
			// (ADR-0026 §6). Ahead of head is a stamp on a version that
			// does not exist; nothing can drift against it.
			continue
		}
		if req.Config == nil {
			// A signal-only requirement asserts nothing about config, so
			// intent alone cannot be judged against it, so drift on such a
			// claim is undetectable from the repo (ADR-0004: the Intended
			// reading judges config assertions).
			continue
		}

		if intended == nil {
			eff := intendedEffective(in.Estate, bp)
			intended = &eff
		}
		passesCurrent, detail := intended.SatisfiesConfig(req.Config)
		if passesCurrent {
			nudges = append(nudges, Nudge{
				Blueprint: bp.ID(),
				Team:      bp.Team,
				Owner:     bp.Owner,
				Message: fmt.Sprintf("claims %s, but the requirement is at version %d and the config already passes it. Re-stamp the claim",
					claim, req.Version),
			})
			continue
		}
		if !passedClaimed(in.History, intended, claim) {
			// Fails the claimed version too: the ordinary failure outcome,
			// the evaluator's business. Reporting it as drift would tell
			// the owner the goalposts moved when they never cleared them.
			continue
		}

		findings = append(findings, Finding{
			Facet:     FacetRequirement,
			Team:      bp.Team,
			Owner:     bp.Owner,
			Blueprint: bp.ID(),
			Message: fmt.Sprintf("claims %s, but the requirement is now at version %d and the config in git fails it (%s). The Requirement moved and the config has not caught up",
				claim, req.Version, strings.Join(detail, "; ")),
			Remediation: fmt.Sprintf("review what changed between %s v%d and v%d, close the gap, and re-stamp the claim in the same PR. %s",
				claim.Requirement, claim.Version, req.Version, req.Remediation),
		})
	}
	return findings, nudges
}

// passedClaimed reports whether the subject passed the requirement at the
// claimed version. With history, the claimed version is replayed through
// the same checker; without it (or where history cannot resolve the
// version) the composer's stamp is trusted as the record (ADR-0026 §4).
func passedClaimed(h History, intended *conformance.Effective, claim blueprint.Claim) bool {
	if h == nil {
		return true
	}
	old, ok := h.RequirementAt(claim.Requirement, claim.Version)
	if !ok || old.Config == nil {
		return true
	}
	passed, _ := intended.SatisfiesConfig(old.Config)
	return passed
}

// floorDrift judges every Tier with a committed rendered artefact against
// the current floor policy: the raised-floor case of REQ-025, Requirement
// facet. The breach enumeration is the render's own (renderer.FloorPolicy
// .Breaches), so the two surfaces cannot disagree about what a breach is.
func floorDrift(in Inputs) ([]Finding, error) {
	var out []Finding
	for _, tier := range in.Topology.SortedTiers() {
		art, committed := in.Rendered[tier.ID()]
		if !committed {
			continue
		}
		bp, ok := in.Estate.Blueprint(tier.Binding().ID())
		if !ok {
			// A binding resolving to nothing refuses the render; the
			// committed artefact beside it is the recompute mismatch's
			// business (ADR-0028 §2), not a drift diagnosis.
			continue
		}
		breaches, err := in.Floors.Breaches(in.Topology, in.Catalogue, in.Estate, tier, bp)
		if err != nil {
			return nil, fmt.Errorf("tier %q: cannot derive judgement strictness: %w", tier.ID(), err)
		}
		for _, b := range breaches {
			rendered := "the committed artefact"
			if art.Commit != "" {
				rendered = fmt.Sprintf("the artefact committed at %s", art.Commit)
			}
			out = append(out, Finding{
				Facet:       FacetRequirement,
				Team:        tier.Team,
				Owner:       tier.Owner,
				Tier:        tier.ID(),
				Environment: tier.Environment,
				Blueprint:   bp.ID(),
				Lane:        string(b.Signal),
				Message: fmt.Sprintf("%s routes %s through %s (%s/%s), which is %s for %s. The current floor for Service Class %s in %s is %s, imposed by %s. The artefact met the floor in force when it merged, and the floor has since been raised",
					rendered, b.Signal, b.Component.ID(), b.Component.Class, b.Component.Type, b.Level, b.Signal, b.Class, tier.Environment, b.Floor, strings.Join(b.Imposers, ", ")),
				Remediation: fmt.Sprintf("route %s through a component at %s or better and re-render in the same PR, or take an owner-reviewed Exemption",
					b.Signal, b.Floor),
			})
		}
	}
	return out, nil
}

// intendedEffective wraps the renderer's Intended projection in the shape
// the config-assertion checker judges. Known is true by construction: the
// authored tree is never a blind spot: an empty Blueprint is a config that
// wires nothing, not an unavailable reading (ADR-0008).
func intendedEffective(est blueprint.Estate, bp blueprint.Blueprint) conformance.Effective {
	eff := conformance.Effective{Known: true}
	for _, p := range renderer.Intended(est, bp) {
		eff.Pipelines = append(eff.Pipelines, conformance.Pipeline{
			Name:       p.Name,
			Receivers:  p.Receivers,
			Processors: p.Processors,
			Exporters:  p.Exporters,
		})
	}
	return eff
}

// laneNames lists the lanes pin drift walks, signals then extensions, in
// stable order.
func laneNames() []string {
	names := make([]string, 0, len(blueprint.Signals)+1)
	for _, s := range blueprint.Signals {
		names = append(names, string(s))
	}
	return append(names, blueprint.ExtensionsLane)
}

// laneEntries returns one named lane's entries, extensions included.
func laneEntries(bp blueprint.Blueprint, name string) []blueprint.Entry {
	if name == blueprint.ExtensionsLane {
		return bp.Extensions
	}
	return bp.Lane(blueprint.Signal(name))
}
