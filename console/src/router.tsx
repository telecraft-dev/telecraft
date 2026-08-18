import {
  createRootRoute,
  createRoute,
  createRouter,
  redirect,
} from '@tanstack/react-router'
import { AppShell } from './chrome/AppShell'
import { Catalogue } from './surfaces/catalogue/Catalogue'
import { Compose } from './surfaces/compose/Compose'
import { Shelf } from './surfaces/estate/Shelf'
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

const rootRoute = createRootRoute({
  component: AppShell,
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

export interface EstateSearch {
  /** Shelf scope: the user's team subtree by default, one click widens (ADR-0042 §2). */
  scope?: 'team' | 'estate'
}

const estateRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/estate',
  component: Shelf,
  validateSearch: (search: Record<string, unknown>): EstateSearch => ({
    scope: search.scope === 'estate' ? 'estate' : search.scope === 'team' ? 'team' : undefined,
  }),
})

const topologyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/topology',
  component: FlowCanvas,
})

const composeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/compose',
  component: Compose,
})

const catalogueRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/catalogue',
  component: Catalogue,
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
