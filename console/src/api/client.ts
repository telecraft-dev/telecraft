import type {
  BlueprintDoc,
  CardDrawer,
  CatalogueComponent,
  CollectorRow,
  ComposeVerdict,
  EstatePayload,
  IndexedObject,
  Me,
  Proposal,
  TopologyPayload,
} from './types'

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`)
  }
  return (await res.json()) as T
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const payload = (await res.json().catch(() => undefined)) as { error?: string } | undefined
    throw new Error(payload?.error ?? `${path}: ${res.status} ${res.statusText}`)
  }
  return (await res.json()) as T
}

export const api = {
  me: () => get<Me>('/api/v1/me'),
  objects: () => get<IndexedObject[]>('/api/v1/objects'),
  estate: () => get<EstatePayload>('/api/v1/estate'),
  drawer: (tier: string) => get<CardDrawer>(`/api/v1/drawer?tier=${encodeURIComponent(tier)}`),
  collectors: () => get<CollectorRow[]>('/api/v1/collectors'),
  topology: () => get<TopologyPayload>('/api/v1/topology'),
  blueprints: () => get<BlueprintDoc[]>('/api/v1/blueprints'),
  catalogue: () => get<CatalogueComponent[]>('/api/v1/catalogue'),
  /** The one evaluator, called continuously as the user edits (ADR-0022 §2). */
  validate: (draft: BlueprintDoc, environment: string) =>
    post<ComposeVerdict>('/api/v1/validate', { draft, environment }),
  /** The PR exit through the forge adapter — fail closed (ADR-0028). */
  propose: (draft: BlueprintDoc, environment: string) =>
    post<Proposal>('/api/v1/proposals', { draft, environment }),
}
