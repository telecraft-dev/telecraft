# P3 — Topology canvas

- Date: 2026-08-14
- Status: verdict recorded
- Prototype: `.proto/p3-topology-canvas/` (throwaway, gitignored, fixture data)

## Question

Does never-draw-collectors (ADR-0007) hold at realistic estate scale, and do
multiple Paths per Service read clearly?

## What was built

Three variants over one fixture (6 Tiers including sibling production/staging
pairs per ADR-0025, 9 Hops, 4 destinations including quarantine, 38 services
including multi-Path, no-collector-at-all, and untrusted on-ramp shapes, plus
an ungoverned-collectors population), with an estate-scale toggle
(~120 / ~2,500 / ~22,000 collectors) that changes only the counts — never the
authored picture:

- **A · Flow canvas** — spatial node-edge graph; Tier cards carry matched
  counts, served/git split, and derived strictness; click a service to trace
  its Paths as coloured overlays with everything else dimmed.
- **B · Path board** — service-first rows; each Path a line of stop-chips
  through shared Tier columns; team/class/multi-Path filters.
- **C · Tier ledger** — the anti-canvas control: no drawn edges, every Hop a
  text row on a Tier card, environments as aligned full-width rows.

## Verdict

**The core bet holds, and all three surfaces ship.** At 22,000 collectors the
authored graph stayed at 6 Tiers and the counts sufficed — no urge to draw
collectors ("counts suffice"; drill-down belongs to the flat estate list
already in OQ-14, not the canvas). Multiple Paths per Service read clearly in
both A's coloured overlays and B's stacked rows; both are intuitive and both
are kept — as with P2, these are complementary representations of one model,
not competing candidates ("under the hood it's the same data just being
represented differently"). The canvas earns its place beyond eye candy: flow
comprehension is genuinely easier on the graph, and the no-collector shape
(direct emitters) was first understood as *legitimate and governed* there.
Quarantine and ungoverned read as two distinct concerns, neither as failure —
the OQ-3 treatment carries forward.

Layout rules that had to be learned in-session (recorded as G7 constraints):

1. **Untrusted and ungoverned sources get a dedicated band above the governed
   edge Tiers** — their hops must never route through or behind governed
   nodes, and grouping them answered the trust-on-Hop question cleanly
   without tainting the gateway.
2. **Environments render as aligned full-width rows** (or bands), never as
   per-column stacks whose alignment depends on how many nodes each column
   holds — this is also where the three-axis vocabulary (Tier / Service
   Class / Environment) stayed clearest.
3. **Tracing dims everything not on the selected service's Paths.**

One negative finding: **derived strictness did not explain itself.** "Judged
at production × C1" (ADR-0025 §4) was not followable from the card alone,
even with the derivation spelled out in parentheses. The rule is right; the
surface must show its work — G7 needs an affordance (e.g. "why C1?" revealing
the traversing C1 Paths), not just microcopy.

Confirmed model point, sharper than the ADR stated it: the collection
infrastructure's Environment is fully independent of the observed
application's environment — a production-class pipeline legitimately observes
a development system. Sibling Tiers read as deliberate structure, not
duplication, including the staging sibling trialling a newer Blueprint
version.

## What it changed

- G7's surface inventory grows by three topology surfaces (canvas, path
  board, tier ledger) over one shared model — alongside P2's two roll-up
  views, P1's composer surfaces, and the flat estate list.
- The three layout rules above and the strictness-derivation affordance are
  G7 design constraints.
- G6 observability cards can assume tier-level aggregation as the canvas
  grain; per-collector detail lives in list surfaces only.
