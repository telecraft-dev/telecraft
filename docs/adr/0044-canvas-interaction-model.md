# ADR-0044: One canvas engine, two vocabularies; geometry that cannot lie

- Status: accepted
- Date: 2026-08-17 (session G7)

## Context

Two canvases ship: the composer's node canvas (components inside one
Blueprint, authoring, P1-D) and the topology flow canvas (Tiers across
the estate, reading, P3-A). Their verdicts demand the same machinery
(orthogonal 90° Manhattan routing with per-signal bend offsets, band/row-
constrained layout, trace-and-dim, node cards), and both recorded layout
rules that carry meaning: signal-level stagger and straight-past-the-node
on P1-D; environment rows and the ungoverned band on P3-A.

## Decision

1. **One canvas engine, two vocabularies.** A single engine owns layout,
   routing and interaction primitives; the composer graph (components,
   per-signal edges) and the topology graph (Tiers, Hops, Paths) are two
   schemas rendered by it. The engine is a **pure library: model in,
   geometry out**, unit-testable without a browser (ADR-0045 §2).
2. **Layout is derived and deterministic; semantic constraints are
   invariants.** Environments render as aligned full-width rows;
   untrusted/ungoverned sources sit in a dedicated band above the
   governed edge Tiers and their hops never route through or behind
   governed nodes (P3 laws); on the composer canvas, vertical position
   *is* the semantics: processors sit at the level of the signals that
   use them, and skipped components are passed straight, never arced
   (P1 law). No layout that violates an invariant is expressible.
3. **Drag rules (three, distinct in kind):**
   - **Drag-authoring is a model edit**, available everywhere the palette
     is (ADR-0043 §4). Derived layout then places the node.
   - **Topology nodes drag within their environment row or band only**,
     to rearrange; position persists in the per-user presentation store
     (ADR-0042 §7). A node can never be dragged out of its row or band:
     the picture must not lie about the estate.
   - **Composer-canvas nodes never drag**: their geometry carries the
     meaning the P1 verdict recorded, so it stays fully derived.
   - **Edges are never hand-drawn** on either canvas. Edges derive from
     signal membership and ordering exactly as the renderer sees them:
     a hand-drawn edge would let the picture disagree with the artefact,
     the one thing this product exists to prevent.
4. **Reading interactions**: pan and zoom; selecting a Service traces its
   Paths as overlays and dims everything not on them (P3); clicking a
   Tier summons the universal card panel in place (ADR-0042 §3.2); hover
   highlights a Hop; every derived chip (strictness, floor) carries the
   "why?" affordance (ADR-0042 §5). Nothing more.
5. **Simulate stays cosmetic in v1**: per-journey dots born at a
   receiver, traversing the full chain, signal groups staggered (the P1
   verdict; all-at-once pulses read wrong). Real simulate (synthetic
   telemetry through the rendered config with per-node before/after) is
   an explicit v1 non-goal; the seam is the rendered artefact plus the
   card contract's per-signal readings, revisited post-v1 (the OQ-18
   pattern: refused now, named so it can be found).

## Consequences

- The engine's determinism makes canvas rendering testable as pure
  functions and makes the *semantics* of any two users' screenshots
  identical; within-row arrangement may differ (ADR-0042 §7 accepts
  this).
- Refusing hand-drawn edges and composer drag means the canvases need no
  reconciliation path between picture and artefact: there is nothing to
  reconcile.
- The interaction substrate beneath the engine is replaceable
  (ADR-0045's escape hatch) precisely because the engine owns geometry.

## Sources

- Session G7; P1, P3 verdicts; ADR-0007, 0031, 0041, 0042, 0043, 0045.
