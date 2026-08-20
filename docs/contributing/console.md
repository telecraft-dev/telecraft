---
title: Console architecture
description: The four Workspaces, the card data contract, the pure canvas engine, the presentation store, demo mode, and the zero-CDN rule.
order: 6
---

# Console architecture

The console lives at `console/` in the product repository and consumes only
the documented platform API. That constraint is enforced by location: if the
console needs something, the API grows it, documented. `console/README.md`
holds the endpoint-by-endpoint contract.

## The stack

ADR-0045 chose boring on purpose, and named its one escape hatch.

| Concern | Choice | Why |
|---|---|---|
| Language and build | TypeScript, React, Vite | The largest contributor pool, and it keeps the door open to a future read-only plugin sharing components. |
| Canvas | xyflow as the interaction substrate, with the pure engine above it | xyflow supplies pan, zoom, node lifecycle and constrainable drag. Custom SVG is the named escape hatch if its interaction model ever fights the row constraints. |
| Data and URL state | TanStack Query, TanStack Router | Query matches the contract's fetch shape: bulk face payloads for a shelf, drawers on demand. Router's typed search params turn "every state is in the URL" into a compiler-checked contract rather than a convention. |
| Styling | Radix headless primitives over CSS-variable design tokens | No opinionated or vendor design system. The console ships structurally complete and token-driven, so a branding pass swaps tokens, never markup. |
| Tests | Vitest, Playwright | The engine tests headless as pure functions; Playwright covers the surfaces. |

DOM scale stays modest by construction: every surface renders at
authored-object cardinality, never collector count.

## The four Workspaces

Navigation is activity-first. Surfaces group under the activity they serve,
because every prototype verdict found them to be complementary representations
of one model rather than competing candidates. That is a view-switcher
relationship, not a menu.

| Workspace | Question it answers | Surfaces |
|---|---|---|
| **Estate** | How are we doing? | The shelf (the landing view), the tree-table roll-up, and the flat filter-first list, which is the only home of per-collector detail. |
| **Topology** | How does telemetry flow? | The flow canvas and the rollout ledger. |
| **Compose** | How do I author a Blueprint? | The Composer, the requirement-first overview, the node canvas, and the resident read-only YAML flyout. |
| **Catalogue and Governance** | What may we use, and how do I ask for more? | Browsing the retained catalogue versions, a team's effective palette with total provenance, and the governance editor whose Allow-list and Grant edits exit as pull requests. Browse-and-request, deliberately thin. |

Object access is served by global jump-to-object search, not by object-first
navigation.

The code follows the same shape:

- `src/surfaces/<workspace>/` holds each Workspace's surfaces.
- `src/chrome/` holds the shell: Workspace navigation, the environment lens,
  and jump-to-object search. `src/chrome/workspaces.ts` is the list of
  Workspaces, and `console/tools/assemble-site.mjs` mirrors it.
- `src/router.tsx` declares the routes and validates every search param.
- `src/api/` holds the client, the demo adapter, and the TypeScript shapes of
  the platform API.

Every surface state is URL-addressable. `console/README.md` lists every
search param and what it means. Two rules keep the links honest: within a
Workspace, view-switchers preserve selection, filters and lens; and a
finding's who-acts chip deep-links to the surface that can act on it.
Inspection stays where you are, and only action travels.

## The card data contract

ADR-0041 makes one payload the sole source for every card surface. The canvas
Tier cards and the shelf's observability cards read the same face payload:
one model, many representations, and nothing that draws a card may consume
anything else.

**The face** is cheap and bulk-fetchable for a whole shelf. It carries:

- **Three bands in fixed order: Delivery, Expectation, Conformance.** Each is
  an *enum state* plus an optional worst-finding label. The states include
  the honest neutrals, each distinct: `not_applicable`, `unknown`,
  `pending_settle`, and `stale_demoted`.
- **Per-signal matrix rows**: volume in and out with the reduction between
  them, freshness, and a shape summary, every reading carrying its own
  `known`, `cause` and `asOf`, so last-known-plus-age renders from the
  contract rather than from the console guessing.
- **The population line**: matched count, floor, floor source, and the
  population state with its neutral age.
- **Shelf summary fields**: owning team, Environment, per-band worst
  severity, and finding counts per kind, including waived counts. The shelf
  groups and orders from these alone, so it never fetches a drawer to sort.

**The drawer** is fetched per card on demand: the findings list with kind,
severity, dampening state, who-acts routing target and mandatory remediation
text, plus the "why?" derivations as structured provenance. Every derived
value on the face carries a claim, the config lines that implied it, and the
SHA it was judged against.

Two rules matter when you write a card surface.

**Hue appears nowhere in the contract.** States are the contract; glyphs and
band position map from states. A renderer cannot make colour load-bearing by
reading a field, because there is no field to read. Three distinct reds stay
distinguishable by band position and glyph.

**A finding without remediation is a complaint.** The remediation text is
mandatory in the payload, so a surface never has to invent one.

The contract is integer-versioned (`contractVersion`, currently 2), and both
sides are held to one artefact, `console/fixtures/card-contract.json`. The Go
engine writes it with `go test ./internal/card -update`, and
`console/tests/card-contract.test.ts` reads it. A field added or renamed on
one side without the other following is a failing test, and the version bump
is the reviewable event.

## The canvas engine

Two canvases ship: the composer's node canvas, which is authoring, and the
topology flow canvas, which is reading. ADR-0044 gives them one engine and
two vocabularies.

The engine is a **pure library**: model in, geometry out, unit-testable
without a browser. It lives at `src/engine/`, with `types.ts` holding the
model vocabulary and `layout.ts` holding layout and routing. Both graphs
compile to the same `EngineModel` of bands, nodes and edges, and the
rendering shell only draws what the engine returns.

Layout is derived and deterministic, and the semantic constraints are
invariants rather than conventions. A node's vertical position is a function
of its band and nothing else. The ungoverned band sits above every Environment
row whatever order the model lists them in. No layout that violates an
invariant is expressible, which is what "geometry that cannot lie" means.

Three interaction rules follow from that, and they are distinct in kind:

- **Edges are never hand-drawn**, on either canvas. They derive from signal
  membership and ordering exactly as the renderer sees them. A hand-drawn edge
  would let the picture disagree with the artefact, which is the one thing
  this product exists to prevent.
- **Topology nodes drag within their Environment row or band only**, to
  rearrange. A node can never leave its band, because the picture must not lie
  about the estate. The within-row offset persists in the presentation store.
- **Composer-canvas nodes never drag.** Their geometry carries meaning:
  processors sit at the level of the signals that use them, and skipped
  components pass straight through rather than arcing around. Drag-authoring
  is a model edit, and derived layout then places the node.

Selecting a Service traces its Paths as overlays and dims everything not on
them. Clicking a Tier summons the universal card panel in place. Simulate is
cosmetic in v1: per-journey dots born at a receiver, traversing the chain,
signal groups staggered, persisting nothing.

Because the engine owns geometry, the interaction substrate beneath it is
replaceable. That is what makes the single substantial dependency inside a
differentiating surface acceptable.

## The presentation store

`src/presentation/store.ts` is the console's only non-git state. It is
per-user, presentation only, and fully loseable: losing it changes what leads,
never what is asserted.

The schema is a closed key list, and that is the whole of it:

```ts
interface Presentation {
  lens?: string
  collapsedSections: Record<string, boolean>
  arrangement: Record<string, Record<string, number>>
}
```

`lens` is the environment lens preference, which an explicit lens in a URL
beats. `collapsedSections` holds per-section collapse overrides.
`arrangement` holds within-row canvas offsets, keyed canvas id then node id.

Anything resembling domain data added here is a design regression that needs
an ADR-0042 amendment, not a pull request. `console/tests/presentation.test.ts`
holds the shape.

## Demo mode

A static host has no server to call, so the public demo is the console bundle
plus a build-time snapshot of the platform API.

```sh
go run ./cmd/telecraft snapshot ...   # writes the API's documents to a JSON file
npm run build:demo                    # vite build --mode demo, selecting .env.demo
```

`.env.demo` sets `VITE_DEMO=1`; every other build leaves the flag unset and
the live client in place. `src/api/demo.ts` then answers the whole contract
from the snapshot.

Three things are worth knowing before you change anything in this path:

- **Nothing in the snapshot is authored by hand.** `internal/console` computes
  it with the same loaders and evaluators a server uses, and a rendered tree
  that no longer matches its sources refuses the build outright.
- **The two continuous evaluators still run.** The composer's validate call
  and the claim flow's preview run in the browser over the same estate
  documents the server would judge with, so Compose and the claim panel stay
  live rather than canned.
- **Write paths render in full and stop at a notice** naming the pull-request
  exit they stand in for. Read-only falls out by construction: there is
  nothing to POST to.

`console/tools/assemble-site.mjs` assembles what a host publishes: the built
console, the snapshot, an entry document per Workspace URL in both spellings
(`<route>.html` and `<route>/index.html`), and `404.html` for the deeper
parameterised routes no static host can enumerate. Every Workspace URL
resolves to a real document and answers 200, because a URL that renders the
right page behind a 404 works for a human and is broken for link previews,
sharing and uptime checks. `console/tests/site.test.ts` holds that list
together with `src/chrome/workspaces.ts`, so a Workspace added to the chrome
and not to the assembler is a failing test.

## The zero-CDN rule

Every asset is bundled and self-hosted, fonts included, which are the system
stack. Air-gap deployment is a first-class requirement, not a nice-to-have,
so the rule is checked rather than documented. Two layers enforce it in CI:

1. `tools/check-zero-cdn.mjs` scans every text file in `dist/` for absolute
   and protocol-relative URLs. HTML, CSS and SVG tolerate none that could be
   fetched. JavaScript tolerates only the allowlisted never-fetched string
   literals the script documents, such as links inside error messages. XML
   namespace identifiers are allowed everywhere, because `xmlns` is a name
   rather than an address.
2. The Playwright suite intercepts every network request at runtime and fails
   if the running console touches any host beyond its own origin.

The demo job runs the same check over the demo bundle. The snapshot beside it
is estate data, endpoints and module paths the console prints and never
fetches, so it is placed after the check rather than exempted from it.
