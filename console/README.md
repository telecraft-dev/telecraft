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

The fixture backend signs in with `demo@example.com` / `demo-password`
(it prints this at start-up). The platform binary verifies PBKDF2 hashes
from the estate's `users.yaml` instead; `telecraft passwd` authors them.

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
  hatch (ADR-0045 §2). Edges derive from the model and are never
  hand-drawn; topology nodes drag within their Environment row or band
  only, with the within-row offset in the per-user presentation store;
  simulate is cosmetic dots over the routed geometry and persists
  nothing (ADR-0044 §3, §5).
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

Endpoints are `GET` unless marked, returning JSON. The TypeScript shapes
live in `src/api/types.ts`. Everything outside `/api/v1/auth/` wants a
session; a 401 renders the sign-in surface in place of the shell, and the
URL the user arrived on survives the round trip.

| Endpoint | Returns |
|---|---|
| `/api/v1/auth/providers` | How sign-in works on this instance (REQ-017, ADR-0019): each provider's name and flow, `password` or `redirect`. Answers signed out. |
| `/api/v1/auth/login` (POST) | Signs in with a password provider: `{provider, username, secret}` in, the session cookie and the `me` payload out. |
| `/api/v1/auth/{name}/start` | Begins a redirect provider's round trip (OIDC; SAML when it lands); `return_to` names the path to resume. |
| `/api/v1/auth/logout` (POST) | Ends the session. |
| `/api/v1/me` | The signed-in user: id, name, team (the shelf's resting scope), and `editableTeams` — the team subtree the ownership tree derives their authoring rights from (ADR-0019 §2). Surfaces offer authoring actions exactly on objects owned inside that set. |
| `/api/v1/objects` | The jump-to-object index: every authored object with kind, id, name, and owning team, plus the active catalogue's entries (kind `entry`, id `class/type`, no team — nobody owns them). |
| `/api/v1/estate` | The shelf's bulk payload: Environments (production leading), the team tree, one ADR-0041 card face per Tier, and the ungoverned band's counts — collectors matching no Tier selector, split by how they are read (served the Unmatched artefact, or foreign via the estate provider; ADR-0030/0031). Concern, never failure: they carry the onboard CTA and appear in no compliance denominator. |
| `/api/v1/drawer?tier=` | The on-demand drawer for one Tier (ADR-0041 §3): findings with kind, severity, dampening state, who-acts routing target, and mandatory remediation, plus "why?" derivations as structured provenance (claim, implying config lines, judged SHA, optional trace action). |
| `/api/v1/collectors` | Per-collector detail for the flat list, its only home (ADR-0042 §3.4): collector id, Tier, team, Environment, state, version, last seen, and the reported identifying attributes Tier selectors match on (ADR-0013). Rows without a Tier are the ungoverned population (ADR-0031), marked by how they are read (`served` or `foreign`); their attributes are the claim flow's raw material. |
| `/api/v1/topology` | Tiers, ungoverned sources, Hops (with trust and signals), and Services' Paths, at authored-object grain. Each Tier carries its selector-matched collector count, derived Service Class, and the served/git delivery split — collectors are matched into a Tier by selector and appear only as these numbers, never as nodes (ADR-0007). |
| `/api/v1/blueprints` | Blueprint schema v1 documents (ADR-0024): per-signal lanes of ordered Component references, local Components, the collector-wide extensions block, the bound Tier, version-stamped `satisfies` claims, and the Catalogue key each lane item instantiates, so lane items deep-link to their Catalogue entries. |
| `/api/v1/catalogue` | Governed Components at their pinned versions. |
| `/api/v1/catalogue/versions` | The installed catalogue versions (ADR-0020 §9: retained, never replaced), each with its entry count and source, and which one is active. |
| `/api/v1/catalogue/entries?version=` | One retained catalogue version's entries: `(class, type)` identity with the `deprecated_type` alias, display name, source (`upstream` or `adopter`), per-signal stability, and per-signal deprecation notices. An unknown version is a 404, never an empty list. |
| `/api/v1/governance` | The authored, git-resident Allow-list policy (ADR-0021 §5): Owners, Allow-lists and Grants in their authored shapes. The console derives each team's effective palette — with total provenance — from this plus the active catalogue (`src/governance/effective.ts`, the same derived-presentation pattern as the roll-up). |
| `POST /api/v1/governance/proposals` | A governance edit exiting as a PR via the forge adapter (ADR-0042 §6). The body carries the complete edited policy plus a title; the server validates fail-closed exactly as loading does (ADR-0021, REQ-011) and answers 422 with the problems named, or the opened proposal — opaque id, URL, branch — mirroring the forge seam (`internal/forge`). The console proposes, the PR decides; the platform binary wires this to `forge.Submit`. |
| `POST /api/v1/validate` | The one evaluator (ADR-0022 §1): the open draft plus its Environment in; findings, palette verdicts (show, grey with reason, hidden-with-count), requirement verdicts (claimed beside met, never blended), the save gate, and the rendered-artefact preview out. Stateless; the composer calls it on every interaction, judging with the same authored governance policy and active catalogue the endpoints above serve. |
| `POST /api/v1/proposals` | The composer exit (ADR-0043 §6): the draft becomes a change proposal through the forge adapter, render-in-PR and user-attributed (ADR-0028, ADR-0014). Enforcement is on: an allow-list violation answers 409 and no proposal opens, fail closed. An optional `claim` context (the claim flow's draft-new-Tier path, ADR-0042 §6) makes this the PR authoring the Tier binding beside the Blueprint; it is judged by the claim rulebook first and refused 422 with the problems named. |
| `POST /api/v1/claims/preview` | The claim flow's continuous impact evaluation (ADR-0042 §6): the constrained selector plus Environment in — with `mode` and `tier` once the one question (attach or draft) is answered — and the impact out: matched ungoverned collectors split by referent, governed populations the selector does not contradict (blast radius, reported, never hidden), attach candidates ranked by selector proximity with the widened selector each implies, and the rendered Tier binding the PR would carry. For attach the judged selector is the widened one: what merge would actually serve. |
| `POST /api/v1/claims` | The claim flow's attach exit: a PR widening the chosen Tier's selector via the forge adapter, user-attributed (ADR-0014) — the console proposes, the PR decides; the platform binary wires this to `forge.Submit` like the other proposal exits. Fail closed, 422 with the problems named; a selector key that names one instance (`service.instance.id` and kin) is refused however it arrives — generalise-never-enumerate is enforced server-side, not assumed of the UI. |

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
- `ungoverned`: the flat list narrowed to the ungoverned population — the
  onboard CTA's door (ADR-0031).
- `herd`: the claim flow's selection, ungoverned collector ids
  comma-joined (ADR-0042 §6). A non-empty herd summons the claim panel;
  view-switching preserves it. Console selection state only — the
  produced selector generalises over shared identity attributes and
  never contains these ids.
- `lane`: the signal lane a Blueprint-shaped who-acts chip lands on in
  Compose (ADR-0042 §3.3).
- `surface` (Compose): the surface switcher over the one open Blueprint
  (ADR-0043 §1) — `composer` (the default), `requirements`, or `canvas`.
  Switching never loses the draft.
- `yaml`: whether the resident read-only YAML flyout is open (REQ-035);
  it rides every Compose surface.
- `claim`, `tier`, `team`, `env` (Compose): the claim flow's
  draft-new-Tier handoff (ADR-0042 §6) — the pre-filled selector in its
  `key=value,key=value` shape plus the new Tier's id, owning team, and
  Environment. Compose opens a fresh draft Blueprint bound to the new
  Tier, and Save proposes the Tier binding beside it as one PR.
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
