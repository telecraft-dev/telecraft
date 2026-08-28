import type {
  ActivationProposalRequest,
  ActivationsPayload,
  AuthProviderInfo,
  BlueprintDoc,
  CardDrawer,
  CatalogueComponent,
  CatalogueEntry,
  CatalogueVersionsPayload,
  ClaimContext,
  ClaimOutcome,
  ClaimPreview,
  ClaimPreviewRequest,
  ClaimRequest,
  CollectorRow,
  ComposeVerdict,
  EditionStanding,
  EndorsementDoc,
  EstatePayload,
  GovernancePayload,
  GovernanceProposalRequest,
  IndexedObject,
  Me,
  PlatformApi,
  Proposal,
  ProposalOutcome,
  ProposalRef,
  RolloutProgress,
  SetupGuidance,
  TierProposalRequest,
  TopologyPayload,
} from './types'
import { demoApi, demoMode } from './demo'

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
    // A server-worded error passes through as it is; the bare status line
    // gains a plain first sentence, because it lands in an error slot a
    // reader sees verbatim.
    const payload = (await res.json().catch(() => undefined)) as { error?: string } | undefined
    throw new Error(
      payload?.error ??
        `The server could not complete this request. ${path}: ${res.status} ${res.statusText}`,
    )
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

/**
 * The live client: the documented platform API over HTTP. Demo mode swaps
 * it for a build-time snapshot of the same documents (see api/demo.ts).
 * The console's surfaces consume the contract, never the transport.
 */
export const liveApi: PlatformApi = {
  me: () => get<Me>('/api/v1/me'),
  edition: () => get<EditionStanding>('/api/v1/edition'),
  authProviders: () => get<AuthProviderInfo[]>('/api/v1/auth/providers'),
  login: (provider: string, username: string, secret: string) =>
    post<Me>('/api/v1/auth/login', { provider, username, secret }),
  logout: () => post<void>('/api/v1/auth/logout'),
  objects: () => get<IndexedObject[]>('/api/v1/objects'),
  estate: () => get<EstatePayload>('/api/v1/estate'),
  drawer: (tier: string) => get<CardDrawer>(`/api/v1/drawer?tier=${encodeURIComponent(tier)}`),
  collectors: () => get<CollectorRow[]>('/api/v1/collectors'),
  topology: () => get<TopologyPayload>('/api/v1/topology'),
  /** Active Rollouts' cohort progress across both delivery paths (ADR-0029). */
  rollouts: () => get<RolloutProgress[]>('/api/v1/rollouts'),
  blueprints: () => get<BlueprintDoc[]>('/api/v1/blueprints'),
  catalogue: () => get<CatalogueComponent[]>('/api/v1/catalogue'),
  catalogueVersions: () => get<CatalogueVersionsPayload>('/api/v1/catalogue/versions'),

  activations: () => get<ActivationsPayload>('/api/v1/activations'),
  catalogueEntries: (version: string) =>
    get<CatalogueEntry[]>(`/api/v1/catalogue/entries?version=${encodeURIComponent(version)}`),
  governance: () => get<GovernancePayload>('/api/v1/governance'),
  /** The one evaluator, called continuously as the user edits (ADR-0022 §2). */
  validate: (draft: BlueprintDoc, environment: string) =>
    post<ComposeVerdict>('/api/v1/validate', { draft, environment }),
  /** The composer's PR exit through the forge adapter, fail closed (ADR-0028).
   * A claim context rides along on the draft-new-Tier path: the PR then
   * authors the Tier binding beside the Blueprint (ADR-0042 §6). */
  propose: (draft: BlueprintDoc, environment: string, claim?: ClaimContext) =>
    post<Proposal>('/api/v1/proposals', { draft, environment, ...(claim ? { claim } : {}) }),

  /** The claim flow's continuous impact evaluation (ADR-0042 §6). */
  claimPreview: (request: ClaimPreviewRequest) =>
    post<ClaimPreview>('/api/v1/claims/preview', request),

  /**
   * The claim flow's attach exit: a PR widening the chosen Tier's selector
   * (ADR-0042 §6). A 422 comes back as named problems for the panel to
   * show: the fail-closed refusal shape, never a thrown error.
   */
  claim: async (request: ClaimRequest): Promise<ClaimOutcome> => {
    const res = await fetch('/api/v1/claims', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(request),
    })
    if (res.status === 401) {
      throw new UnauthenticatedError('/api/v1/claims')
    }
    if (res.status === 422) {
      const body = (await res.json()) as { problems?: string[] }
      return { problems: body.problems ?? ['the claim was refused'] }
    }
    if (!res.ok) {
      throw new Error(
        `The server could not complete this request. /api/v1/claims: ${res.status} ${res.statusText}`,
      )
    }
    return { proposal: (await res.json()) as Proposal }
  },

  /**
   * A governance edit exits as a PR via the forge adapter (ADR-0042 §6). A
   * 422 is the render-gate shape of refusal: the problems come back as
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
      throw new Error(
        `The server could not complete this request. /api/v1/governance/proposals: ${res.status} ${res.statusText}`,
      )
    }
    return { proposal: (await res.json()) as ProposalRef }
  },

  /**
   * Activating a version is a change to the estate, so it leaves here the
   * way every other change does: as a pull request, attributed to the
   * operator, and the review is the audit (ADR-0020 §6, ADR-0028).
   */
  proposeActivation: async (request: ActivationProposalRequest): Promise<ProposalOutcome> => {
    const res = await fetch('/api/v1/activations/proposals', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(request),
    })
    if (res.status === 401) {
      throw new UnauthenticatedError('/api/v1/activations/proposals')
    }
    if (res.status === 422) {
      const body = (await res.json()) as { problems?: string[] }
      return { problems: body.problems ?? ['the activation was refused'] }
    }
    if (!res.ok) {
      throw new Error(
        `The server could not complete this request. /api/v1/activations/proposals: ${res.status} ${res.statusText}`,
      )
    }
    return { proposal: (await res.json()) as ProposalRef }
  },

  /** Every Endorsement held on this estate (ADR-0061 §2). */
  endorsements: () => get<EndorsementDoc[]>('/api/v1/endorsements'),

  /** The never_seen card's setup guidance, generated on view (ADR-0060 §4). */
  setup: (tier: string) =>
    get<SetupGuidance>(`/api/v1/setup?tier=${encodeURIComponent(tier)}`),

  /**
   * The Tier-first flow's PR exit through the forge adapter (ADR-0060 §2).
   * A 422 is the fail-closed shape of refusal: the problems come back as
   * data for the flow to show, never as a thrown error.
   */
  proposeTier: async (request: TierProposalRequest): Promise<ProposalOutcome> => {
    const res = await fetch('/api/v1/tiers/proposals', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(request),
    })
    if (res.status === 401) {
      throw new UnauthenticatedError('/api/v1/tiers/proposals')
    }
    if (res.status === 422) {
      const body = (await res.json()) as { problems?: string[] }
      return { problems: body.problems ?? ['the Tier proposal was refused'] }
    }
    if (!res.ok) {
      throw new Error(
        `The server could not complete this request. /api/v1/tiers/proposals: ${res.status} ${res.statusText}`,
      )
    }
    return { proposal: (await res.json()) as ProposalRef }
  },
}

/**
 * The client every surface consumes. On an instance it is the live API; in
 * the static demo it is the snapshot-backed client, which answers the same
 * shapes and ends every write path at an explanatory notice (issue #50).
 */
export const api: PlatformApi = demoMode ? demoApi : liveApi
