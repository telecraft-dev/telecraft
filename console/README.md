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

- `src/engine/`: the canvas engine, a pure library (model in, geometry
  out; ADR-0044). Band and row constraints are invariants; xyflow only
  draws what the engine returns, and custom SVG stays the named escape
  hatch (ADR-0045 §2).
- `src/surfaces/`: the Workspace surfaces (the Estate shelf, the
  Topology flow canvas, Compose, and Catalogue & Governance).
- `src/chrome/`: the shell (Workspace navigation, the environment lens,
  and jump-to-object search).
- `src/presentation/`: the per-user presentation store, the console's
  only non-git state (ADR-0042 §7).
- `tools/`: the fixture backend and the zero-CDN check.
- `fixtures/`: the fixture estate, mirroring
  `internal/renderer/testdata/estate`.

## The platform API

The console consumes only the documented platform API (ADR-0045 §6): if
the console needs it, the API grows it, documented here. The fixture
backend (`tools/fixture-backend.mjs`) implements this contract over the
fixture estate; the platform binary replaces it when the API endpoint
lands there.

All endpoints are `GET` except the one proposal exit, returning JSON. The
TypeScript shapes live in `src/api/types.ts`.

| Endpoint | Returns |
|---|---|
| `/api/v1/me` | The signed-in user: id, name, and team (the shelf's resting scope). |
| `/api/v1/objects` | The jump-to-object index: every authored object with kind, id, name, and owning team, plus the active catalogue's entries (kind `entry`, id `class/type`, no team — nobody owns them). |
| `/api/v1/estate` | The shelf's bulk payload: Environments (production leading), the team tree, and one ADR-0041 card face per Tier. |
| `/api/v1/drawer?tier=` | The on-demand drawer for one Tier (ADR-0041 §3): findings with kind, severity, dampening state, who-acts routing target, and mandatory remediation, plus "why?" derivations as structured provenance (claim, implying config lines, judged SHA, optional trace action). |
| `/api/v1/collectors` | Per-collector detail for the flat list, its only home (ADR-0042 §3.4): collector id, Tier, team, Environment, state, version, last seen. |
| `/api/v1/topology` | Tiers, ungoverned sources, Hops (with trust and signals), and Services' Paths, at authored-object grain. |
| `/api/v1/blueprints` | Blueprint summaries with per-signal component lanes in renderer order, plus the Catalogue key each lane item instantiates, so composer palette items deep-link to their Catalogue entries. |
| `/api/v1/catalogue` | Governed Components at their pinned versions. |
| `/api/v1/catalogue/versions` | The installed catalogue versions (ADR-0020 §9: retained, never replaced), each with its entry count and source, and which one is active. |
| `/api/v1/catalogue/entries?version=` | One retained catalogue version's entries: `(class, type)` identity with the `deprecated_type` alias, display name, source (`upstream` or `adopter`), per-signal stability, and per-signal deprecation notices. An unknown version is a 404, never an empty list. |
| `/api/v1/governance` | The authored, git-resident Allow-list policy (ADR-0021 §5): Owners, Allow-lists and Grants in their authored shapes. The console derives each team's effective palette — with total provenance — from this plus the active catalogue (`src/governance/effective.ts`, the same derived-presentation pattern as the roll-up). |
| `POST /api/v1/governance/proposals` | The one write: a governance edit exiting as a PR via the forge adapter (ADR-0042 §6). The body carries the complete edited policy plus a title; the server validates fail-closed exactly as loading does (ADR-0021, REQ-011) and answers 422 with the problems named, or the opened proposal — opaque id, URL, branch — mirroring the forge seam (`internal/forge`). The console proposes, the PR decides; the platform binary wires this to `forge.Submit`. |

Card faces follow the ADR-0041 contract, integer-versioned
(`contractVersion: 1`): three bands as enum states plus worst-finding
labels, shelf summary fields, and the population line. Hue appears
nowhere in the contract. The face also carries optional waived counts
per kind (`waivedCounts`), an additive shelf summary field the roll-up
reads: an Exemption waives the count, never the diagnosis, and waived
counts ride every roll-up level (ADR-0017, ADR-0037).

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

- `lens`: the environment lens; an explicit value beats the persisted
  per-user preference (ADR-0042 §4).
- `object`: the selected object as `kind:id`, for example
  `tier:data-flow/gateway` (see `src/objectref.ts`).
- `scope`: the shelf scope, either the signed-in user's team subtree
  (`team`, the default) or the whole estate (`estate`).
- `view`: the Estate view-switcher, one of `shelf` (the default),
  `rollup` (the tree-table roll-up), or `list` (the flat filter-first
  list). Switching preserves selection, filters, and lens (ADR-0042
  §3.1).
- `tier`, `team`, `env`: the flat list's explicit filters. A collector
  count anywhere is a door that lands here with `tier` pre-filled
  (ADR-0042 §3.4); the lens is never one of these filters.
- `lane`: the signal lane a Blueprint-shaped who-acts chip lands on in
  Compose (ADR-0042 §3.3).
- `view` (Catalogue & Governance): `browse` (the default), `palette`, or
  `governance` — view-switchers over one model, like the Estate's.
- `version`: the catalogue version browsed; absent means the active one.
- `stability`, `signal`: the browse filters — together they filter on the
  named signal at the named level.
- `team` (palette view): the team whose effective palette shows; absent
  means the signed-in user's team.
- `request`: a `class/type` entry prefilling a Grant draft in the
  governance view — the browse-and-request door from a non-allowed
  palette row (ADR-0042 §1).
