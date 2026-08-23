# P2: Estate & team roll-up view

- Date: 2026-08-12
- Status: verdict recorded
- Prototype: `.proto/p2-team-rollup/` (throwaway, gitignored, fixture data)

## Question

Does hierarchical team roll-up (ratio-plus-worst per finding kind, ADR-0017)
read clearly, and where do "ungoverned" and "provider-can't-report" states
sit without looking like failures?

## What was built

Two variants over shared fixtures (Acme Engineering tree, ~15 services,
waivers, cross-team component routing, an ElasticFleet permanently-UNSET
delivery column):

- **A · Tree-table**: expandable team → owner → finding rows; three fixed
  columns (conformance / delivery / component), each ratio bar + worst badge
  + waived chip.
- **B · Cards**: zoomable team cards with breadcrumb and a persistent
  current-node summary strip.

Structural choices that survived contact: neutral states (`unknown`,
`ungoverned`, `UNSET`) excluded from ratio *and* worst badge, rendered as
dashed grey-blue chips with "not counted, not a failure" microcopy; a node
with only neutral readings shows "no verdict" rather than a score; waived
rows keep their diagnosis badge beside owner+expiry; cross-team routing
rendered on both sides (routed-here badge / routed-away note).

## Verdict

**Both variants ship.** The table view and the card view are complementary
product surfaces, not competing candidates; G7 designs the IA around both
(table for scanning/drill-down, cards for zooming/presentation). Recorded as
a G7 constraint.

The neutral-state treatment drew no adverse reaction and its mechanics
(excluded from denominator and worst badge; "no verdict" for all-neutral
nodes) carry forward as the default; re-probe deliberately in G7 with the
real design (the P2 reaction questions in `.proto/p2-team-rollup/QUESTIONS.md`
were not individually walked).

## What it changed

- G7 surface inventory starts from two roll-up views, shared model.
- The "excluded from denominator" rule for neutral states graduates from
  prototype choice to design default (feeds the G5 EstateProvider contract
  and G7).
