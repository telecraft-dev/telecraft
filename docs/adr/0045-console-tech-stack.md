# ADR-0045: Console tech stack: boring on purpose, sovereignty where it counts

- Status: accepted
- Date: 2026-08-17 (session G7)

## Context

ADR-0006 left the console's stack to G7, chosen fresh (the prior Preact
console was judged not good enough; its code is mined for nothing). The
governing constraints: air-gap first-class (ADR-0019: every asset
self-hosted), the vendor-word lint's spirit (ADR-0001: no vendor design
system), URL-addressability as a load-bearing feature (ADR-0042 §3.5), a
bespoke deterministic canvas engine (ADR-0044), branding applied
separately as a later pass, and an open-source contributor pool to care
about. DOM scale is modest by construction: every surface renders at
authored-object cardinality, never collector count (ADR-0041).

## Decision

1. **TypeScript + React + Vite.** Boring on purpose: the largest
   contributor pool, and React keeps ADR-0006's Backstage door genuinely
   open (a future read-only plugin could share components). Leaner
   frameworks were declined: the scale argument for them does not exist
   here, and every future contributor would pay the familiarity tax.
2. **Canvas: xyflow (React Flow) as the interaction substrate; the
   ADR-0044 engine sits above it as a pure TypeScript library** (model
   in, geometry out: layout, Manhattan routing, band/row constraints).
   xyflow supplies pan/zoom, node lifecycle and constrainable drag.
   **Custom SVG is the named escape hatch**: if xyflow's interaction
   model ever fights the row constraints, the pure-engine split confines
   the rewrite to the rendering shell. This is the console's one
   substantial dependency inside a differentiating surface; the split is
   what makes it acceptable.
3. **Data and URL state: TanStack Query + TanStack Router.** Query
   matches the contract's fetch shape: bulk face payloads for a shelf,
   drawers on demand (ADR-0041). Router's typed search params turn
   ADR-0042's everything-in-the-URL rule into a compiler-checked
   contract instead of a convention.
4. **Styling: headless primitives (Radix) + design tokens (CSS
   variables); no opinionated or vendor design system.** The console
   ships structurally complete and visually token-driven, so the
   branding pass swaps tokens, never markup.
5. **Vitest + Playwright.** The engine tests headless as pure functions;
   Playwright covers the surfaces. **Zero runtime CDN dependencies,
   enforced in CI**: fonts, icons, everything bundled; the air-gap
   check lives beside the vendor-word lint, not in a doc.
6. **The console lives in the product repo** (`console/`), consuming
   only the documented platform API, ADR-0006's constraint enforced by
   location: if the console needs it, the API grows it, documented.

## Consequences

- The documented API remains a Phase 2 deliverable with a real consumer
  from day one.
- An xyflow major-version fight is a contained rendering-shell decision,
  not a product rewrite.
- Contributor onboarding cost is conventional-React cheap; nothing in
  the stack requires project-specific framework knowledge beyond the
  engine library, which is deliberately plain functions.

## Sources

- Session G7; ADR-0001, 0006, 0019, 0041, 0042, 0044.
