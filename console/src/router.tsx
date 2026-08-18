import {
  createRootRoute,
  createRoute,
  createRouter,
  redirect,
} from '@tanstack/react-router'
import { AuthGate } from './auth/AuthGate'
import { AppShell } from './chrome/AppShell'
import { Catalogue } from './surfaces/catalogue/Catalogue'
import { Compose } from './surfaces/compose/Compose'
import { Estate } from './surfaces/estate/Estate'
import { FlowCanvas } from './surfaces/topology/FlowCanvas'

// Every surface state is URL-addressable — workspace, selection, lens
// (ADR-0042 §3.5): the search params below are that rule as a
// compiler-checked contract (ADR-0045 §3). A console state that cannot be
// cited in a PR comment does not exist.

export interface RootSearch {
  /** The environment lens; explicit here beats the persisted preference. */
  lens?: string
  /** The selected object, as `kind:id` (see objectref.ts). */
  object?: string
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

const rootRoute = createRootRoute({
  component: Root,
  validateSearch: (search: Record<string, unknown>): RootSearch => ({
    lens: typeof search.lens === 'string' ? search.lens : undefined,
    object: typeof search.object === 'string' ? search.object : undefined,
  }),
})

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: () => {
    throw redirect({ to: '/estate' })
  },
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
  /** Flat-list filters — explicit filters stay available; the lens never is one. */
  team?: string
  env?: string
  /** Flat-list filter: ungoverned collectors only — the onboard CTA's door (ADR-0031). */
  ungoverned?: boolean
  /** The claim flow's herd: selected ungoverned collector ids, comma-joined
   * (ADR-0042 §6). Console selection state, never a selector — the produced
   * selector generalises and enumerates nothing. */
  herd?: string
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
  }),
})

const topologyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/topology',
  component: FlowCanvas,
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
export type CatalogueView = 'browse' | 'palette' | 'governance'

export interface CatalogueSearch {
  view?: CatalogueView
  /** The catalogue version browsed; absent means the active one (ADR-0020 §9). */
  version?: string
  /** Browse filters — together they filter on the named signal at the named level. */
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
      search.view === 'palette' || search.view === 'governance' || search.view === 'browse'
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
  indexRoute,
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
