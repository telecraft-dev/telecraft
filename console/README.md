# Telecraft console

The purpose-built console (ADR-0006), scaffolded on the ADR-0045 stack:
TypeScript, React, and Vite; TanStack Router and Query; Radix primitives
over design tokens; xyflow as the canvas interaction substrate with the
pure engine library above it (ADR-0044). The shell is ADR-0042's:
activity-first Workspaces, global jump-to-object search, and every surface
state URL-addressable. Home leads them and is the one named for a place,
because the activity it serves is choosing which activity (ADR-0056).

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
npm test                    # Vitest: engine, presentation store, shelf ordering, Home
npm run check:palette       # the design tokens against their accessibility floors
npm run build               # tsc --noEmit + vite build into dist/
npm run check:zero-cdn      # no external host in any built artefact
npm run check:bundle-budget # the entry chunk within its gzipped ceiling
npm run e2e                 # Playwright against dist/ and the fixture backend
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
- `src/surfaces/`: the Workspace surfaces (Home, the Estate shelf, the
  Topology flow canvas, Compose, and Catalogue & Governance).
- `src/home/`: Home's derivation, a pure module like `src/estate/`. It
  judges nothing: the estate standing is `estate/rollup.ts`'s own root row
  and the worst Tiers are in `estate/order.ts`'s own order, so the landing
  cannot disagree with the surface it points at (ADR-0056 §2). Nothing it
  returns is blended, and every bounded list reports what it dropped.
- `src/chrome/`: the shell (Workspace navigation, the environment lens,
  and jump-to-object search).
- `src/presentation/`: the per-user presentation store, the console's
  only non-git state (ADR-0042 §7, ADR-0051 §6).
- `src/tours/`: guided Tours (ADR-0051): the authored Steps, the one
  runner that draws them, and the chrome control that offers the welcome.
- `tools/`: the fixture backend and the zero-CDN check.
- `fixtures/`: the fixture estate, mirroring
  `internal/renderer/testdata/estate`, plus `card-contract.json`, the
  shared card-contract artefact the engine writes and the console's
  contract test reads.

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
| `/api/v1/me` | The signed-in user: id, name, team (the shelf's resting scope), and `editableTeams`, the team subtree the ownership tree derives their authoring rights from (ADR-0019 §2). Surfaces offer authoring actions exactly on objects owned inside that set. |
| `/api/v1/objects` | The jump-to-object index: every authored object with kind, id, name, and owning team, plus the active catalogue's entries (kind `entry`, id `class/type`, no team, because nobody owns them). |
| `/api/v1/estate` | The shelf's bulk payload: Environments (production leading), the team tree, one ADR-0041 card face per Tier, and the ungoverned band's counts: collectors matching no Tier selector, split by how they are read (served the Unmatched artefact, or foreign via the estate provider; ADR-0030/0031). Concern, never failure: they carry the onboard CTA and appear in no compliance denominator. |
| `/api/v1/drawer?tier=` | The on-demand drawer for one Tier (ADR-0041 §3): findings with kind, severity, dampening state, who-acts routing target, and mandatory remediation, plus "why?" derivations as structured provenance (claim, implying config lines, judged SHA, optional trace action). |
| `/api/v1/collectors` | Per-collector detail for the flat list, its only home (ADR-0042 §3.4): collector id, Tier, team, Environment, state, version, last seen, and the reported identifying attributes Tier selectors match on (ADR-0013). Rows without a Tier are the ungoverned population (ADR-0031), marked by how they are read (`served` or `foreign`); their attributes are the claim flow's raw material. |
| `/api/v1/topology` | Tiers, ungoverned sources, Hops (with trust and signals), and Services' Paths, at authored-object grain. Each Tier carries its selector-matched collector count, derived Service Class, and the served/git delivery split. Collectors are matched into a Tier by selector and appear only as these numbers, never as nodes (ADR-0007). |
| `/api/v1/rollouts` | Every active Rollout's cohort progress (ADR-0029), computed per request: membership is a pure function of the authored Rollout and reported identifying attributes, never stored (§4). Each Rollout carries its authored facts (owner, target Tier, `from`/`to` bindings, active stage), the evaluation's verdict (`hold`, `blocked`, `advance`, `abort`) with its reason and evidence, per-cohort progress (cumulative membership per stage, split by delivery path, with running status against the two rendered artefacts: served by acknowledged config hash, foreign by the `telecraft.tier` stamp readings, ADR-0039 §5), every halted member with its condition and reason (§6, the set is extensible), and "why?" provenance for the authored facts. Foreign members are advisory: lag, never failure (§7). Pending stages carry the membership preview: information for the reviewer, never the authoritative decision. |
| `/api/v1/blueprints` | Blueprint schema v1 documents (ADR-0024): per-signal lanes of ordered Component references, local Components, the collector-wide extensions block, the bound Tier, version-stamped `satisfies` claims, and the Catalogue key each lane item instantiates, so lane items deep-link to their Catalogue entries. |
| `/api/v1/catalogue` | Governed Components at their pinned versions. |
| `/api/v1/catalogue/versions` | The installed catalogue versions (ADR-0020 §9: retained, never replaced), each with its entry count and source, and which one is active. |
| `/api/v1/catalogue/entries?version=` | One retained catalogue version's entries: `(class, type)` identity with the `deprecated_type` alias, display name, source (`upstream` or `adopter`), per-signal stability, and per-signal deprecation notices. An unknown version is a 404, never an empty list. |
| `/api/v1/governance` | The authored, git-resident Allow-list policy (ADR-0021 §5): Owners, Allow-lists and Grants in their authored shapes. The console derives each team's effective palette, with total provenance, from this plus the active catalogue (`src/governance/effective.ts`, the same derived-presentation pattern as the roll-up). |
| `POST /api/v1/governance/proposals` | A governance edit exiting as a PR via the forge adapter (ADR-0042 §6). The body carries the complete edited policy plus a title; the server validates fail-closed exactly as loading does (ADR-0021, REQ-011) and answers 422 with the problems named, or the opened proposal (opaque id, URL, branch) mirroring the forge seam (`internal/forge`). The console proposes, the PR decides; the platform binary wires this to `forge.Submit`. |
| `POST /api/v1/validate` | The one evaluator (ADR-0022 §1): the open draft plus its Environment in; findings, palette verdicts (show, grey with reason, hidden-with-count), requirement verdicts (claimed beside met, never blended), the save gate, and the rendered-artefact preview out. Stateless; the composer calls it on every interaction, judging with the same authored governance policy and active catalogue the endpoints above serve. |
| `POST /api/v1/proposals` | The composer exit (ADR-0043 §6): the draft becomes a change proposal through the forge adapter, render-in-PR and user-attributed (ADR-0028, ADR-0014). Enforcement is on: an allow-list violation answers 409 and no proposal opens, fail closed. An optional `claim` context (the claim flow's draft-new-Tier path, ADR-0042 §6) makes this the PR authoring the Tier binding beside the Blueprint; it is judged by the claim rulebook first and refused 422 with the problems named. |
| `POST /api/v1/claims/preview` | The claim flow's continuous impact evaluation (ADR-0042 §6): the constrained selector plus Environment in (with `mode` and `tier` once the one question, attach or draft, is answered) and the impact out: matched ungoverned collectors split by referent, governed populations the selector does not contradict (blast radius, reported, never hidden), attach candidates ranked by selector proximity with the widened selector each implies, and the rendered Tier binding the PR would carry. For attach the judged selector is the widened one: what merge would actually serve. |
| `POST /api/v1/claims` | The claim flow's attach exit: a PR widening the chosen Tier's selector via the forge adapter, user-attributed (ADR-0014). The console proposes, the PR decides; the platform binary wires this to `forge.Submit` like the other proposal exits. Fail closed, 422 with the problems named; a selector key that names one instance (`service.instance.id` and kin) is refused however it arrives: generalise-never-enumerate is enforced server-side, not assumed of the UI. |

Card faces follow the ADR-0041 contract, integer-versioned
(`contractVersion: 2`): three bands as enum states plus worst-finding
labels, the per-signal matrix rows, shelf summary fields, and the
population line. Hue appears nowhere in the contract: band position and
glyph are what distinguish the three reds, and a renderer cannot make
colour load-bearing by reading a field. The face also carries optional
waived counts per kind (`waivedCounts`), an additive shelf summary field
the roll-up reads: an Exemption waives the count, never the diagnosis,
and waived counts ride every roll-up level (ADR-0017, ADR-0037).

Version 2 added the metering rows P4's verdict put under the bands
(ADR-0040): per signal lane, volume as items in and out with the
reduction between them, freshness, and a shape summary, each reading
carrying its own `known`, `cause` and `asOf`, so last-known-plus-age
renders from the contract rather than from the console guessing. It also
added the population line's ADR-0035 `state` with its neutral age, and
the Tier's `churn` reading. In-minus-out is *reduction*: a filter
dropping ninety per cent is doing its job, and the only readings the
meter reds off are `refused`, `sendFailed` and `enqueueFailed`.

Both sides of the contract are held to one artefact,
`fixtures/card-contract.json`. The engine writes it (with
`go test ./internal/card -update`) and `tests/card-contract.test.ts`
reads it, so a field added or renamed on either side without the other
following is a failing test, and the version bump is the reviewable
event.

## Demo mode

A static host has no server to call, so the public demo at
`demo.telecraft.dev` is the console bundle plus a build-time **snapshot**
of the platform API (issue #50):

```sh
go run ./cmd/telecraft snapshot \
  -estate ../estate-demo -catalogue ../estate-demo/catalogues/catalogue-v0.158.0.json \
  -library ../estate-demo/requirements -exemptions ../estate-demo/exemptions \
  -rows ../estate-demo/demo/rows.yaml -readings ../estate-demo/demo/readings.yaml \
  -commit "$(git -C ../estate-demo rev-parse HEAD)" -team engineering \
  -out console/dist/demo-snapshot.json
npm run build:demo     # VITE_DEMO=1: the console reads the snapshot, not /api
```

`telecraft snapshot` (`internal/console`) loads the estate with the same
loaders the platform uses and writes the documents the endpoints above
serve, computed by the packages that own each judgement: `internal/renderer`
for artefacts and floors, `internal/drift` for library_drift,
`internal/conformance` for the verdict cross, `internal/expectation` for
claims, `internal/inventory` for populations, `internal/serving` for
selector matching, `internal/allowlist` for the governance policy. Nothing
in the snapshot is authored by hand; a rendered tree that no longer matches
its sources refuses the build outright (ADR-0028 §2).

The two things a repository cannot hold (the collector estate and the
arrivals, which reach a real instance through the EstateProvider and
TelemetryProvider seams) are declared by the estate in a readings file and
played back through those same seams. They are inputs like the authored
YAML; every verdict over them is the product's.

In the console, `src/api/demo.ts` answers the whole contract from the
snapshot. Read paths project it; the two continuous evaluators (the
composer's `validate` and the claim flow's preview) run in the browser
over the same estate documents the server would judge with, so Compose and
the claim panel stay live rather than canned. The write paths render in
full and terminate at an explanatory notice that names the pull-request
exit they stand in for: Compose's Save, the claim flow's attach and draft
exits, and the governance editor's proposal. Read-only falls out by
construction: there is nothing to POST to.

Deep links need the host's single-page fallback: the deploy copies
`index.html` to `404.html`, which is how a static host serves a URL the
bundle owns (ADR-0042 §3.5).

## Guided Tours

A **Tour** is an authored sequence of **Steps** teaching the console over
whatever estate the reader is signed in to (ADR-0051). Tours are data, not
surfaces: `src/tours/welcome.ts` is the whole of the welcome Tour, and
`src/tours/TourRunner.tsx` is the only thing that draws one.

To write another, add a file beside it and register it in
`src/tours/registry.ts`:

```ts
export const claiming: Tour = {
  id: 'claiming',
  title: 'Bringing ungoverned collectors in',
  summary: 'What the claim flow does, and what it proposes.',
  steps: [
    {
      id: 'herd',
      title: 'Start from the collectors',
      body: 'Sixty words at most. Longer than that is documentation, and the documentation lives in docs/.',
      anchor: 'ungoverned-band',       // a `data-tour` attribute on a surface
      to: '/estate',                    // the Workspace it is read in
      search: { view: 'list', ungoverned: true },
    },
  ],
}
```

Four rules hold, and each is checked rather than remembered:

- **A Tour narrates; it never drives.** A Step navigates and points. It
  never clicks a control, authors anything, or shows invented data.
  Anchored Steps take no pointer events, so the product underneath stays
  usable while one is open.
- **Anchors are `data-tour`, never `data-testid`.** A test id is a test's
  grip and may be renamed with the test; an anchor is content a reader is
  shown. `e2e/tour.spec.ts` walks every Step of every registered Tour and
  fails on an anchor that resolves nowhere.
- **A missing anchor degrades to a centred Step**, and an unknown Tour id
  renders nothing at all. Teaching is never load-bearing.
- **The position is in the URL** (`?tour=welcome&step=3`), like every other
  console state. What a reader has already been *offered* is presentation
  state, in the store beside the lens.

The welcome Tour opens itself once per reader, and only on a bare landing
URL: a shared link carries the sender's context, and a Tour never lands on
top of it. Its first Step is the one place a Tour reads differently on the
public demo.

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
- `view` (Topology): the Topology view-switcher, one of `flow` (the flow
  canvas, the default) or `rollout` (the rollout ledger, ADR-0029).
  Switching preserves selection and lens (ADR-0042 §3.1); a selected
  `object` of kind `rollout` implies the ledger, so a Rollout deep link
  needs no explicit view.
- `ungoverned`: the flat list narrowed to the ungoverned population, the
  onboard CTA's door (ADR-0031).
- `herd`: the claim flow's selection, ungoverned collector ids
  comma-joined (ADR-0042 §6). A non-empty herd summons the claim panel;
  view-switching preserves it. Console selection state only: the
  produced selector generalises over shared identity attributes and
  never contains these ids.
- `lane`: the signal lane a Blueprint-shaped who-acts chip lands on in
  Compose (ADR-0042 §3.3).
- `surface` (Compose): the surface switcher over the one open Blueprint
  (ADR-0043 §1): `composer` (the default), `requirements`, or `canvas`.
  Switching never loses the draft.
- `yaml`: whether the resident read-only YAML flyout is open (REQ-035);
  it rides every Compose surface.
- `claim`, `tier`, `team`, `env` (Compose): the claim flow's
  draft-new-Tier handoff (ADR-0042 §6): the pre-filled selector in its
  `key=value,key=value` shape plus the new Tier's id, owning team, and
  Environment. Compose opens a fresh draft Blueprint bound to the new
  Tier, and Save proposes the Tier binding beside it as one PR.
- `view` (Catalogue & Governance): `browse` (the default), `palette`, or
  `governance`, view-switchers over one model, like the Estate's.
- `version`: the catalogue version browsed; absent means the active one.
- `stability`, `signal`: the browse filters. Together they filter on the
  named signal at the named level.
- `team` (palette view): the team whose effective palette shows; absent
  means the signed-in user's team.
- `request`: a `class/type` entry prefilling a Grant draft in the
  governance view, the browse-and-request door from a non-allowed
  palette row (ADR-0042 §1).
- `tour`, `step`: the running guided Tour and the Step within it, one-based
  (ADR-0051 §3). Both ride at the root beside `lens`, so a Tour survives a
  Workspace switch and a Step is citable in a pull-request comment. An
  unknown Tour is no Tour; a Step past either end lands on the nearest real
  one.
