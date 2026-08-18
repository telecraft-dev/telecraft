import type {
  BlueprintSummary,
  CardDrawer,
  CatalogueComponent,
  CollectorRow,
  EstatePayload,
  IndexedObject,
  Me,
  TopologyPayload,
} from './types'

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`)
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
  blueprints: () => get<BlueprintSummary[]>('/api/v1/blueprints'),
  catalogue: () => get<CatalogueComponent[]>('/api/v1/catalogue'),
}
