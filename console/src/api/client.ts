import type {
  AuthProviderInfo,
  BlueprintSummary,
  CardDrawer,
  CatalogueComponent,
  CatalogueEntry,
  CatalogueVersionsPayload,
  CollectorRow,
  EstatePayload,
  GovernancePayload,
  GovernanceProposalRequest,
  IndexedObject,
  Me,
  ProposalOutcome,
  ProposalRef,
  TopologyPayload,
} from './types'

/** A 401: no session, or one the estate no longer knows. The auth gate
 * turns this into the sign-in surface; nothing else retries it. */
export class UnauthenticatedError extends Error {
  constructor(path: string) {
    super(`${path}: sign in to use this API`)
    this.name = 'UnauthenticatedError'
  }
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (res.status === 401) {
    throw new UnauthenticatedError(path)
  }
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`)
  }
  return (await res.json()) as T
}

async function post<T>(path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body ?? {}),
  })
  if (res.status === 401) {
    throw new UnauthenticatedError(path)
  }
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`)
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

export const api = {
  me: () => get<Me>('/api/v1/me'),
  authProviders: () => get<AuthProviderInfo[]>('/api/v1/auth/providers'),
  login: (provider: string, username: string, secret: string) =>
    post<Me>('/api/v1/auth/login', { provider, username, secret }),
  logout: () => post<void>('/api/v1/auth/logout'),
  objects: () => get<IndexedObject[]>('/api/v1/objects'),
  estate: () => get<EstatePayload>('/api/v1/estate'),
  drawer: (tier: string) => get<CardDrawer>(`/api/v1/drawer?tier=${encodeURIComponent(tier)}`),
  collectors: () => get<CollectorRow[]>('/api/v1/collectors'),
  topology: () => get<TopologyPayload>('/api/v1/topology'),
  blueprints: () => get<BlueprintSummary[]>('/api/v1/blueprints'),
  catalogue: () => get<CatalogueComponent[]>('/api/v1/catalogue'),
  catalogueVersions: () => get<CatalogueVersionsPayload>('/api/v1/catalogue/versions'),
  catalogueEntries: (version: string) =>
    get<CatalogueEntry[]>(`/api/v1/catalogue/entries?version=${encodeURIComponent(version)}`),
  governance: () => get<GovernancePayload>('/api/v1/governance'),

  /**
   * A governance edit exits as a PR via the forge adapter (ADR-0042 §6). A
   * 422 is the render-gate shape of refusal — the problems come back as
   * data for the editor to show, never as a thrown error.
   */
  proposeGovernance: async (request: GovernanceProposalRequest): Promise<ProposalOutcome> => {
    const res = await fetch('/api/v1/governance/proposals', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(request),
    })
    if (res.status === 401) {
      throw new UnauthenticatedError('/api/v1/governance/proposals')
    }
    if (res.status === 422) {
      const body = (await res.json()) as { problems?: string[] }
      return { problems: body.problems ?? ['the proposal was refused'] }
    }
    if (!res.ok) {
      throw new Error(`/api/v1/governance/proposals: ${res.status} ${res.statusText}`)
    }
    return { proposal: (await res.json()) as ProposalRef }
  },
}
