import type {
  AuthProviderInfo,
  BlueprintSummary,
  CatalogueComponent,
  EstatePayload,
  IndexedObject,
  Me,
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
  topology: () => get<TopologyPayload>('/api/v1/topology'),
  blueprints: () => get<BlueprintSummary[]>('/api/v1/blueprints'),
  catalogue: () => get<CatalogueComponent[]>('/api/v1/catalogue'),
}
