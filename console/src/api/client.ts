import type {
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
