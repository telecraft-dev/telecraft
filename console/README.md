# Telecraft console

The purpose-built console (ADR-0006), scaffolded on the ADR-0045 stack:
TypeScript, React, and Vite; TanStack Router and Query; Radix primitives
over design tokens; xyflow as the canvas interaction substrate with the
pure engine library above it (ADR-0044). The shell is ADR-0042's: four
activity-first Workspaces, global jump-to-object search, and every surface
state URL-addressable.

## Running it

```sh
npm install
npm run backend    # the fixture backend on http://127.0.0.1:4700
npm run dev        # the console on http://localhost:5173, proxying /api
```

Checks, as CI runs them:

```sh
npm run typecheck
npm test               # Vitest: engine, presentation store, shelf ordering
npm run build          # tsc --noEmit + vite build into dist/
npm run check:zero-cdn # no external host in any built artefact
npm run e2e            # Playwright against dist/ and the fixture backend
```

## Layout

- `src/engine/` — the canvas engine: a pure library, model in, geometry
  out (ADR-0044). Band and row constraints are invariants; xyflow only
  draws what the engine returns, and custom SVG stays the named escape
  hatch (ADR-0045 §2).
- `src/surfaces/` — the Workspace surfaces: the Estate shelf, the
  Topology flow canvas, Compose, and Catalogue & Governance.
- `src/chrome/` — the shell: Workspace navigation, the environment lens,
  and jump-to-object search.
- `src/presentation/` — the per-user presentation store, the console's
  only non-git state (ADR-0042 §7).
- `tools/` — the fixture backend and the zero-CDN check.
- `fixtures/` — the fixture estate, mirroring
  `internal/renderer/testdata/estate`.

## The platform API

The console consumes only the documented platform API (ADR-0045 §6): if
the console needs it, the API grows it, documented here. The fixture
backend (`tools/fixture-backend.mjs`) implements this contract over the
fixture estate; the platform binary replaces it when the API endpoint
lands there.

All endpoints are `GET`, returning JSON. The TypeScript shapes live in
`src/api/types.ts`.

| Endpoint | Returns |
|---|---|
| `/api/v1/me` | The signed-in user: id, name, and team (the shelf's resting scope). |
| `/api/v1/objects` | The jump-to-object index: every authored object with kind, id, name, and owning team. |
| `/api/v1/estate` | The shelf's bulk payload: Environments (production leading), the team tree, and one ADR-0041 card face per Tier. |
| `/api/v1/topology` | Tiers, ungoverned sources, Hops (with trust and signals), and Services' Paths, at authored-object grain. |
| `/api/v1/blueprints` | Blueprint summaries with per-signal component lanes in renderer order. |
| `/api/v1/catalogue` | Governed Components at their pinned versions. |

Card faces follow the ADR-0041 contract, integer-versioned
(`contractVersion: 1`): three bands as enum states plus worst-finding
labels, shelf summary fields, and the population line. Hue appears
nowhere in the contract.

## Zero-CDN rule

Every asset is bundled and self-hosted; fonts are the system stack
(ADR-0019, ADR-0045 §5). Two enforcement layers run in CI beside the
vendor-word lint:

1. `tools/check-zero-cdn.mjs` scans every text file in `dist/` and fails
   on any external URL. HTML, CSS, and SVG tolerate none; JavaScript
   tolerates only the allowlisted never-fetched string literals the
   script documents.
2. The Playwright suite intercepts every network request and fails if
   the running console touches any host beyond its own origin.

## URL state

Search params are validated by the router (ADR-0045 §3):

- `lens` — the environment lens; an explicit value beats the persisted
  per-user preference (ADR-0042 §4).
- `object` — the selected object as `kind:id`, for example
  `tier:data-flow/gateway` (see `src/objectref.ts`).
- `scope` — shelf scope: the signed-in user's team subtree (`team`,
  the default) or the whole estate (`estate`).
