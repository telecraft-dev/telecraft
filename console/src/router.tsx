import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router'
import { lazy } from 'react'
import { AuthGate } from './auth/AuthGate'
import { AppShell } from './chrome/AppShell'
import { Home } from './surfaces/home/Home'

// Each Workspace loads its own code (issue #125). The five Workspaces
// (ADR-0042 §1, amended by ADR-0056 §1) already meet at the router, so the
// router is where the bundle divides: a Workspace reached through a dynamic
// import is a chunk of its own, fetched on the navigation that first needs
// it. Home is the exception and is imported eagerly: it is the landing, so
// a dynamic import there would only add a round trip to the first paint.
//
// The reason to bother is the canvas engine's substrate. `@xyflow/react` is
// the console's one substantial dependency (ADR-0045 §2), and only
// Topology's flow canvas and Compose's node canvas draw with it. Split
// here, it leaves the entry chunk that Estate and Catalogue also pay for,
// and lands in a chunk those two Workspaces share.
//
// These are `React.lazy` rather than the router's own
// `lazyRouteComponent`, and the difference is not cosmetic.
// `lazyRouteComponent` hands the router a `preload()` the router calls on
// every navigation, which keeps each match in its asynchronous loading path
// even when the module has been in memory for minutes. In a console where
// every surface state is a search param, that path is walked on each
// keystroke of a filter and each tick of a checkbox, and the match renders
// its pending state in between: the Workspace blanks and remounts, and a
// controlled input reverts under the reader's hand. `React.lazy` resolves
// once and renders synchronously afterwards, so only the first navigation
// to a Workspace waits.
//
// The chunks are same-origin assets Vite emits into `dist/assets`, so
// splitting takes nothing outside the origin and the zero-CDN rule is
// untouched (ADR-0019, ADR-0045 §5). `tools/check-bundle-budget.mjs` holds
// the entry chunk to a ceiling, so the split cannot quietly undo itself.
const Estate = lazy(() =>
  import('./surfaces/estate/Estate').then((m) => ({ default: m.Estate })),
)
const Topology = lazy(() =>
  import('./surfaces/topology/Topology').then((m) => ({ default: m.Topology })),
)
const Compose = lazy(() =>
  import('./surfaces/compose/Compose').then((m) => ({ default: m.Compose })),
)
const Catalogue = lazy(() =>
  import('./surfaces/catalogue/Catalogue').then((m) => ({ default: m.Catalogue })),
)

// Every surface state is URL-addressable: workspace, selection, lens
// (ADR-0042 §3.5). The search params below are that rule as a
// compiler-checked contract (ADR-0045 §3). A console state that cannot be
// cited in a PR comment does not exist.

export interface RootSearch {
  /** The environment lens; explicit here beats the persisted preference. */
  lens?: string
  /** The selected object, as `kind:id` (see objectref.ts). */
  object?: string
  /** The running Tour, by id (ADR-0051 §3). */
  tour?: string
  /** The Step within it, one-based, because a URL is read by people. */
  step?: number
}

// Every Workspace sits behind the auth gate (REQ-017, ADR-0019): signed
// out, the same URL renders the sign-in surface and resumes afterwards.
function Root() {
  return (
    <AuthGate>
      <AppShell />
    </AuthGate>
  )
}

/** A numeric search param: a number, or the string a URL carries one as. */
function numberParam(value: unknown): number | undefined {
  if (typeof value === 'number') return Number.isFinite(value) ? value : undefined
  if (typeof value !== 'string' || value.trim() === '') return undefined
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : undefined
}

const rootRoute = createRootRoute({
  component: Root,
  validateSearch: (search: Record<string, unknown>): RootSearch => ({
    lens: typeof search.lens === 'string' ? search.lens : undefined,
    object: typeof search.object === 'string' ? search.object : undefined,
    // A Tour rides with the lens rather than with a Workspace's own state:
    // it belongs to the console, so it survives every switch. Anything
    // unreadable here is clamped by the runner, never thrown (ADR-0051 §4).
    tour: typeof search.tour === 'string' ? search.tour : undefined,
    step: numberParam(search.step),
  }),
})

/**
 * Home: the landing, and the fifth top-level entry (ADR-0056 §1). It holds
 * no state of its own, so it validates no search params beyond the root's:
 * there is no selection, no filter and no view-switcher to address, which
 * is how it satisfies ADR-0042 §3.5 rather than an exemption from it.
 */
const homeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: Home,
})

/** The Estate view-switchers: one model, complementary representations (ADR-0042 §1). */
export type EstateView = 'shelf' | 'rollup' | 'list'

export interface EstateSearch {
  /** Shelf scope: the user's team subtree by default, one click widens (ADR-0042 §2). */
  scope?: 'team' | 'estate'
  /** The Estate view: the shelf lands; roll-up and the flat list switch in place. */
  view?: EstateView
  /** Flat-list pre-filter: the Tier a collector count opened (ADR-0042 §3.4). */
  tier?: string
  /** Flat-list filters: explicit filters stay available; the lens never is one. */
  team?: string
  env?: string
  /** Flat-list filter: ungoverned collectors only, the onboard CTA's door (ADR-0031). */
  ungoverned?: boolean
  /** The claim flow's herd: selected ungoverned collector ids, comma-joined
   * (ADR-0042 §6). Console selection state, never a selector: the produced
   * selector generalises and enumerates nothing. */
  herd?: string
  /** The Add-a-Tier flow (ADR-0060 §1): `add` opens the panel; the rest are
   * its pre-fills from the Blueprints view door or the claim flow's draft
   * branch. `blueprint` rides as `id@version`; `selector` reuses the claim
   * handoff's `key=value,key=value` shape; `name` seeds the Tier name. */
  add?: boolean
  blueprint?: string
  name?: string
  selector?: string
}

const estateRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/estate',
  component: Estate,
  validateSearch: (search: Record<string, unknown>): EstateSearch => ({
    scope: search.scope === 'estate' ? 'estate' : search.scope === 'team' ? 'team' : undefined,
    view:
      search.view === 'rollup' || search.view === 'list' || search.view === 'shelf'
        ? search.view
        : undefined,
    tier: typeof search.tier === 'string' ? search.tier : undefined,
    team: typeof search.team === 'string' ? search.team : undefined,
    env: typeof search.env === 'string' ? search.env : undefined,
    ungoverned: search.ungoverned === true || search.ungoverned === 'true' ? true : undefined,
    herd: typeof search.herd === 'string' ? search.herd : undefined,
    add: search.add === true || search.add === 'true' ? true : undefined,
    blueprint: typeof search.blueprint === 'string' ? search.blueprint : undefined,
    name: typeof search.name === 'string' ? search.name : undefined,
    selector: typeof search.selector === 'string' ? search.selector : undefined,
  }),
})

/** The Topology view-switchers: the flow canvas, and the rollout ledger.
 * One model, complementary representations (ADR-0042 §1, ADR-0029). */
export type TopologyView = 'flow' | 'rollout'

export interface TopologySearch {
  /** The Topology view: the flow canvas lands; the rollout ledger switches
   * in place. A selected `object` of kind `rollout` implies the ledger. */
  view?: TopologyView
}

const topologyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/topology',
  component: Topology,
  validateSearch: (search: Record<string, unknown>): TopologySearch => ({
    view: search.view === 'rollout' || search.view === 'flow' ? search.view : undefined,
  }),
})

/** The Compose surfaces: three projections of one open Blueprint (ADR-0043 §1). */
export type ComposeSurface = 'composer' | 'requirements' | 'canvas'

export interface ComposeSearch {
  /** The signal lane a who-acts chip lands on (ADR-0042 §3.3). */
  lane?: string
  /** The surface switcher; switching never loses the draft (ADR-0043 §1). */
  surface?: ComposeSurface
  /** Whether the resident read-only YAML flyout is open (REQ-035). */
  yaml?: boolean
  /** The claim flow's draft-new-Tier handoff (ADR-0042 §6): the pre-filled
   * selector in its `key=value,key=value` shape, plus the new Tier's id,
   * owning team, and Environment. */
  claim?: string
  tier?: string
  team?: string
  env?: string
  /** The Blueprints browse view (ADR-0061 §3). `browse` opens it, not a
   * `view` key: other routes' `view` unions would collide in functional
   * search updates. Its filters are explicit and URL-addressable like the
   * flat list's; the lens never filters. The shared `env` above doubles as
   * the browse Environment filter, since the claim handoff and the browse
   * view are never on screen together. */
  browse?: boolean
  substrate?: string
  serviceClass?: string
  endorsed?: boolean
}

const composeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/compose',
  component: Compose,
  validateSearch: (search: Record<string, unknown>): ComposeSearch => ({
    lane: typeof search.lane === 'string' ? search.lane : undefined,
    surface:
      search.surface === 'composer' ||
      search.surface === 'requirements' ||
      search.surface === 'canvas'
        ? search.surface
        : undefined,
    yaml: search.yaml === true || search.yaml === 'true' ? true : undefined,
    claim: typeof search.claim === 'string' ? search.claim : undefined,
    browse: search.browse === true || search.browse === 'true' ? true : undefined,
    substrate: typeof search.substrate === 'string' ? search.substrate : undefined,
    serviceClass: typeof search.serviceClass === 'string' ? search.serviceClass : undefined,
    endorsed: search.endorsed === true || search.endorsed === 'true' ? true : undefined,
    tier: typeof search.tier === 'string' ? search.tier : undefined,
    team: typeof search.team === 'string' ? search.team : undefined,
    env: typeof search.env === 'string' ? search.env : undefined,
  }),
})

/**
 * The Catalogue & Governance view-switchers (ADR-0042 §1): browse the
 * retained catalogues, a team's effective palette, and the governance
 * editor whose edits exit as PRs.
 */
export type CatalogueView = 'browse' | 'palette' | 'governance' | 'activation'

export interface CatalogueSearch {
  view?: CatalogueView
  /** The catalogue version browsed; absent means the active one (ADR-0020 §9). */
  version?: string
  /** Browse filters: together they filter on the named signal at the named level. */
  stability?: string
  signal?: string
  /** The palette's team; absent means the signed-in user's team. */
  team?: string
  /** A class/type entry prefilling a Grant draft in the governance view. */
  request?: string
}

const catalogueRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/catalogue',
  component: Catalogue,
  validateSearch: (search: Record<string, unknown>): CatalogueSearch => ({
    view:
      search.view === 'palette' ||
      search.view === 'governance' ||
      search.view === 'browse' ||
      search.view === 'activation'
        ? search.view
        : undefined,
    version: typeof search.version === 'string' ? search.version : undefined,
    stability: typeof search.stability === 'string' ? search.stability : undefined,
    signal: typeof search.signal === 'string' ? search.signal : undefined,
    team: typeof search.team === 'string' ? search.team : undefined,
    request: typeof search.request === 'string' ? search.request : undefined,
  }),
})

const routeTree = rootRoute.addChildren([
  homeRoute,
  estateRoute,
  topologyRoute,
  composeRoute,
  catalogueRoute,
])

export const router = createRouter({ routeTree })

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
