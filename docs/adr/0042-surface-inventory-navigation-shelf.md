# ADR-0042: Surface inventory, activity-first navigation, and the shelf

- Status: accepted
- Date: 2026-08-17 (session G7)

## Context

OQ-14 accumulated every prototype verdict: P1 delivered three composer
surfaces plus the flat filter-first estate list; P2 two roll-up views; P3
three topology surfaces plus layout laws; P4 the card-and-drawer plus the
finding that cards need a shelf before the grid scales. ADR-0041 fixed the
card contract with shelf-summary fields on the face precisely so a shelf
could group and sort without drawer fetches. ADR-0006 requires a small
purpose-built console on a documented API. OQ-3 assigned the onboard-CTA
authoring flow here. Eleven surfaces exist by verdict; this ADR decides how
they compose into one product.

## Decision

1. **Navigation is activity-first: four areas.** Every prototype verdict
   said "complementary representations of one model, not competing
   candidates": that is a view-switcher relationship, so surfaces group
   under the activity they serve, never as top-level menu entries:
   - **Estate** ("how are we doing?"): the shelf (landing view), the
     tree-table roll-up, the zoomable-card roll-up, the flat filter-first
     list (the InfoSec workflow; the only home of per-collector detail).
   - **Topology** ("how does telemetry flow?"): flow canvas, path board,
     tier ledger.
   - **Compose** ("author a Blueprint"): Composer, Requirement-first
     overview, Node canvas, YAML flyout (ADR-0043).
   - **Catalogue & Governance**: catalogue and Schema Registry browsing,
     Grant and Exemption request flows. Browse-and-request, thin; not a
     workspace.
   Object access is served by **global jump-to-object search**, not by
   object-first navigation. This inventory is the closed v1 list.
2. **The shelf's resting state**: scope defaults to the signed-in user's
   team subtree (ownership-derived authz, ADR-0016/0019), and one click
   widens to the estate; sections by team subtree, **environment rows
   within each section** (P3's aligned-rows law), production leading
   (ADR-0033); cards ordered worst-severity-first **from face summary
   fields alone** (ADR-0041 §2), tie-broken on finding counts; neutral and
   no-verdict cards sink to the row's tail but are never hidden: hiding
   reads as healthy, the same sin the denominator rule prevents
   (ADR-0017/0031). **All-healthy sections collapse to a summary line**;
   cards themselves never collapse: P4's equal-heights verdict holds at
   card grain, the section line contains no card.
3. **Linking model (five rules):**
   1. Within a workspace, view-switchers preserve selection, filters and
      lens: the traced Service on the path board is still traced on the
      flow canvas.
   2. The ADR-0041 card face + drawer is **one universal component,
      summoned in place** (side panel) wherever a Tier appears: shelf,
      canvas Tier card, ledger row, list row. Inspection never navigates.
   3. **Findings travel via their who-acts chip**: the routing target
      deep-links to the surface that can act (blueprint-shaped findings
      into Compose at the offending lane, grant-shaped into Governance,
      delivery-shaped to the Tier in Topology). Inspect stays; action
      travels.
   4. Collector counts are doors to the flat list, pre-filtered: P3's
      rule that per-collector detail lives in list surfaces only.
   5. **Every surface state is URL-addressable**: workspace, view,
      selection, filters, lens. A console state that cannot be cited in a
      PR comment does not exist.
4. **The environment lens is emphasis and evaluation context, never a
   hard filter.** One global chrome control, default `production`
   (ADR-0033), persisted per user. Multi-environment surfaces keep every
   row visible and let the lens choose emphasis; single-environment
   evaluation surfaces (the composer) treat it as the selected evaluation
   context. Explicit list filters remain available; the lens sets
   defaults, never ceilings. **An explicit lens in the URL beats the
   persisted preference**; bare URLs fall back to preference.
5. **The "why?" affordance is a provenance popover with an optional
   travel action.** Every derived value (strictness, floors, claim
   verdicts, staleness demotions) opens ADR-0041's structured provenance
   chain (claim → implying config lines → judged SHA), fetched with the
   drawer payload. Spatial derivations carry an action ("trace the C1
   Paths") that lights the canvas via rule 3.3.
6. **The claim flow** (the OQ-3 onboard CTA): every ungoverned
   representation converges on one flow. Herd-first: the flat list
   multi-selects and the flow operates on the selection; the suggested
   selector **generalises over shared identity attributes and never
   enumerates instance ids** (evidence supplied by the Unmatched
   artefact's self-telemetry, ADR-0030). After owning team + Environment
   (defaulted), two paths one question apart: **attach** to an existing
   Tier (candidates ranked by selector proximity) or **draft** a new Tier
   (opens Compose, selector pre-filled). **Exit is always a PR** via the
   forge adapter, user-attributed (ADR-0014), carrying the rendered
   impact preview: the console proposes, the PR decides. Quarantine
   routing stays a Compose concern (ADR-0031's two referents stay
   distinct).
7. **The console's only non-git state is a per-user presentation
   preference store**: lens, collapsed sections, within-row canvas
   arrangement (ADR-0044). Presentation only, never model truth, fully
   loseable: losing it changes what leads, never what is asserted.

## Consequences

- The shelf sorts and groups from face fields alone: ADR-0041's
  shelf-summary bet is exercised, not just available.
- Jump-to-object search is load-bearing, not a convenience: it is the
  answer to the object-first counter-argument.
- The claim flow's selector suggestion is the one place the console
  synthesises policy from observed state; it is bounded to draft-input
  reviewed under the existing impact-preview machinery.
- Two users may see different emphasis (lens, arrangement) at the same
  bare URL; a shared explicit URL always means what the sender saw.

## Sources

- Session G7; OQ-14, OQ-3; P1 to P4 verdicts; ADR-0006, 0014, 0016, 0017,
  0019, 0025, 0028, 0030, 0031, 0033, 0035, 0041.
