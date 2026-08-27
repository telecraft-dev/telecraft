import { previewClaim, ungovernedSummary } from '../../tools/claims'
import { validate } from '../../tools/evaluator'
import { setupGuidance } from '../../tools/tiers'
import type {
  ActivationProposalRequest,
  ActivationsPayload,
  AuthProviderInfo,
  BlueprintDoc,
  CardDrawer,
  CatalogueComponent,
  CatalogueEntry,
  CatalogueVersionsPayload,
  ClaimOutcome,
  ClaimPreview,
  ClaimPreviewRequest,
  CollectorRow,
  ComposeVerdict,
  EndorsementDoc,
  Environment,
  EstatePayload,
  EstateSettings,
  Finding,
  GovernancePayload,
  IndexedObject,
  Me,
  PlatformApi,
  Provenance,
  Proposal,
  ProposalOutcome,
  RolloutProgress,
  SetupGuidance,
  TeamNode,
  TierProposalRequest,
  TopologyPayload,
} from './types'
import { CARD_CONTRACT_VERSION } from './types'
import type {
  BlueprintDoc as Draft,
  ClaimContext,
  ClaimRequest,
  GovernanceProposalRequest,
} from './types'

/**
 * Demo mode: the console reading a build-time snapshot instead of calling a
 * server (issue #50). The snapshot holds the same documents the platform
 * API serves, produced by `telecraft snapshot` running the real evaluators
 * over the demo estate, so every surface shows real output of real code,
 * and the only thing missing is the server.
 *
 * Read-only falls out by construction: there is nothing to POST to. The
 * write paths still render in full (the composer still validates on every
 * keystroke, the claim flow still previews impact, the governance editor
 * still refuses a malformed policy) and terminate at the notice below
 * instead of opening a pull request.
 */
export const demoMode = import.meta.env.VITE_DEMO === '1'

/** Where the snapshot lives, relative to the page. */
const SNAPSHOT = 'demo-snapshot.json'

/**
 * The explanation a write path ends at. It says what would have happened,
 * so the demo teaches the exit rather than hiding it: every change leaves
 * this console as a pull request against the estate repository, and the PR
 * decides (ADR-0028, ADR-0042 §6).
 */
export class DemoWriteError extends Error {
  constructor(what: string) {
    super(
      `This is a read-only demo, so ${what} stops here. On a real instance the ` +
        `console never writes to the estate. Instead it opens a pull request, ` +
        `rendered and attributed to you, and the review decides. Everything up to ` +
        `this point (the evaluation, the refusals, and the rendered preview) is ` +
        `the same code that runs on a real instance.`,
    )
    this.name = 'DemoWriteError'
  }
}

/** The notice a fail-closed endpoint returns in its problems shape. */
const refusal = (what: string) => [new DemoWriteError(what).message]

/** The snapshot's shape, as `telecraft snapshot` writes it. */
interface Snapshot {
  meta: {
    generatedAt: string
    commit: string
    repository?: string
    evaluatedAt: string
  }
  activations: ActivationsPayload
  estate: {
    me: { id: string; name: string; email: string; team: string; operator: boolean }
    environments: Environment[]
    teams: TeamNode
    cards: EstatePayload['cards']
    drawers: Record<string, CardDrawer>
    collectors: CollectorRow[]
    selectors: Record<string, Record<string, string>>
    topology: {
      sources: TopologyPayload['sources']
      delivery: Record<string, { served: number; git: number }>
      hops: TopologyPayload['hops']
      paths: TopologyPayload['paths']
    }
    services: { id: string; name: string; team: string; serviceClass?: string }[]
    rollouts?: RolloutProgress[]
    blueprints: BlueprintDoc[]
    catalogue: CatalogueComponent[]
    owners: GovernancePayload['owners']
    allowLists: GovernancePayload['allowLists']
    grants: GovernancePayload['grants']
    settings?: EstateSettings
    endorsements?: EndorsementDoc[]
    floors: Record<string, Record<string, string>>
    requirements: unknown[]
  }
  catalogues: {
    active: string
    versions: {
      version: string
      source: { repository: string; ref: string; commit?: string }
      components: CatalogueEntry[]
    }[]
  }
}

let loading: Promise<Snapshot> | undefined

/**
 * Loads the snapshot once and shares it. A failure is stated, never
 * retried into a spinner that never ends: a demo whose data is missing
 * should say so.
 */
function snapshot(): Promise<Snapshot> {
  if (!loading) {
    loading = fetch(SNAPSHOT).then((res) => {
      if (!res.ok) {
        throw new Error(
          `${SNAPSHOT}: ${res.status} ${res.statusText}. The demo snapshot did not load, ` +
            `so there is nothing to show. The estate's own pipeline produces it, and ` +
            `a failed build leaves the previous one in place.`,
        )
      }
      return res.json() as Promise<Snapshot>
    })
  }
  return loading
}

/** The metadata the demo banner shows: which estate, at which commit, when. */
export function demoMeta() {
  return snapshot().then((s) => s.meta)
}

/** The signed-in user's edit horizon: their team subtree (ADR-0019 §2). */
function subtree(teams: TeamNode, teamId: string): string[] {
  const find = (node: TeamNode): TeamNode | undefined => {
    if (node.id === teamId) return node
    for (const child of node.teams ?? []) {
      const hit = find(child)
      if (hit) return hit
    }
    return undefined
  }
  const rooted = find(teams)
  if (!rooted) return []
  const out: string[] = []
  const walk = (node: TeamNode) => {
    out.push(node.id)
    for (const child of node.teams ?? []) walk(child)
  }
  walk(rooted)
  return out
}

/**
 * The declared estate settings (ADR-0060 §4). A snapshot taken before the
 * estate declared them carries none, which is undeclared endpoints rather
 * than a failure: the guidance shows the gap instead of inventing values.
 */
function settingsOf(s: Snapshot): EstateSettings {
  return s.estate.settings ?? { opampEndpoint: '', selfTelemetryEndpoint: '' }
}

/** The active catalogue's entries: what authoring is judged against. */
function activeEntries(s: Snapshot): CatalogueEntry[] {
  return s.catalogues.versions.find((v) => v.version === s.catalogues.active)?.components ?? []
}

/**
 * The jump-to-object index: every authored object, plus the active
 * catalogue's entries, which are browsable and deep-linkable though owned
 * by nobody (ADR-0042 §1).
 */
function objects(s: Snapshot): IndexedObject[] {
  const out: IndexedObject[] = []
  const walkTeams = (team: TeamNode) => {
    out.push({ kind: 'team', id: team.id, name: team.name, team: team.id })
    for (const child of team.teams ?? []) walkTeams(child)
  }
  walkTeams(s.estate.teams)
  for (const card of s.estate.cards) {
    out.push({
      kind: 'tier',
      id: card.tier,
      name: card.name,
      team: card.team,
      environment: card.environment,
    })
  }
  for (const service of s.estate.services) {
    out.push({ kind: 'service', id: service.id, name: service.name, team: service.team })
  }
  for (const blueprint of s.estate.blueprints) {
    out.push({ kind: 'blueprint', id: blueprint.id, name: blueprint.name, team: blueprint.team })
  }
  for (const component of s.estate.catalogue) {
    out.push({ kind: 'component', id: component.id, name: component.name, team: component.team })
  }
  for (const entry of activeEntries(s)) {
    out.push({ kind: 'entry', id: `${entry.class}/${entry.type}`, name: entry.displayName ?? entry.type })
  }
  return out
}

/** The drawer for a Tier with nothing to report: empty, and honestly so. */
function emptyDrawer(tier: string): CardDrawer {
  const findings: Finding[] = []
  const provenance: Provenance[] = []
  return { contractVersion: CARD_CONTRACT_VERSION, tier, findings, provenance }
}

/**
 * The demo client. Every read answers from the snapshot; the two
 * continuous evaluators (the composer's validate and the claim flow's
 * preview) run in the browser against the same estate documents the
 * server would judge with, so those surfaces stay live rather than canned.
 */
export const demoApi: PlatformApi = {
  me: async (): Promise<Me> => {
    const s = await snapshot()
    return { ...s.estate.me, editableTeams: subtree(s.estate.teams, s.estate.me.team) }
  },

  authProviders: async (): Promise<AuthProviderInfo[]> => [],

  // The write paths keep the contract's signatures and ignore the
  // arguments: a stub that quietly dropped a parameter would let the demo
  // client drift from the API it stands in for.
  login: async (_provider: string, _username: string, _secret: string): Promise<Me> => {
    throw new DemoWriteError('signing in')
  },

  logout: async (): Promise<void> => {
    throw new DemoWriteError('signing out')
  },

  objects: async (): Promise<IndexedObject[]> => objects(await snapshot()),

  estate: async (): Promise<EstatePayload> => {
    const s = await snapshot()
    return {
      environments: s.estate.environments,
      teams: s.estate.teams,
      cards: s.estate.cards,
      ungoverned: ungovernedSummary(s.estate),
      settings: settingsOf(s),
    }
  },

  drawer: async (tier: string): Promise<CardDrawer> => {
    const s = await snapshot()
    const found = s.estate.drawers[tier]
    if (found) return found
    // A Tier with nothing to report answers empty, honestly.
    return emptyDrawer(tier)
  },

  collectors: async (): Promise<CollectorRow[]> => (await snapshot()).estate.collectors,

  topology: async (): Promise<TopologyPayload> => {
    const s = await snapshot()
    return {
      environments: s.estate.environments,
      tiers: s.estate.cards.map((card) => ({
        id: card.tier,
        name: card.name,
        team: card.team,
        environment: card.environment,
        ...(card.serviceClass ? { serviceClass: card.serviceClass } : {}),
        matched: card.population.matched,
        delivery: s.estate.topology.delivery[card.tier] ?? {
          served: card.population.matched,
          git: 0,
        },
      })),
      sources: s.estate.topology.sources,
      hops: s.estate.topology.hops,
      paths: s.estate.topology.paths,
    }
  },

  blueprints: async (): Promise<BlueprintDoc[]> => (await snapshot()).estate.blueprints,

  catalogue: async (): Promise<CatalogueComponent[]> => (await snapshot()).estate.catalogue,

  catalogueVersions: async (): Promise<CatalogueVersionsPayload> => {
    const s = await snapshot()
    return {
      active: s.catalogues.active,
      versions: s.catalogues.versions.map((v) => ({
        version: v.version,
        active: v.version === s.catalogues.active,
        components: v.components.length,
        source: v.source,
      })),
    }
  },

  activations: async (): Promise<ActivationsPayload> => (await snapshot()).activations,

  catalogueEntries: async (version: string): Promise<CatalogueEntry[]> => {
    const s = await snapshot()
    const found = s.catalogues.versions.find((v) => v.version === version)
    if (!found) {
      // Say "cannot know", never fabricate: an unknown version is an
      // error, not an empty list.
      throw new Error(`no catalogue version ${version} is installed on this instance`)
    }
    return found.components
  },

  /**
   * The rollout ledger (ADR-0029). A snapshot taken before the estate had
   * a Rollout carries none, which is an empty ledger rather than a
   * failure. The surface says so itself.
   */
  rollouts: async (): Promise<RolloutProgress[]> => (await snapshot()).estate.rollouts ?? [],

  governance: async (): Promise<GovernancePayload> => {
    const s = await snapshot()
    return {
      owners: s.estate.owners,
      allowLists: s.estate.allowLists,
      grants: s.estate.grants,
    }
  },

  /** The one evaluator, run in the browser over the same estate documents. */
  validate: async (draft: BlueprintDoc, environment: string): Promise<ComposeVerdict> => {
    const s = await snapshot()
    return validate(s.estate, activeEntries(s), draft, environment)
  },

  /** The claim flow's continuous impact evaluation, likewise. */
  claimPreview: async (request: ClaimPreviewRequest): Promise<ClaimPreview> => {
    const s = await snapshot()
    return previewClaim(s.estate, request)
  },

  propose: async (
    _draft: Draft,
    _environment: string,
    _claim?: ClaimContext,
  ): Promise<Proposal> => {
    throw new DemoWriteError('proposing this Blueprint')
  },

  claim: async (_request: ClaimRequest): Promise<ClaimOutcome> => ({
    problems: refusal('claiming these collectors'),
  }),

  proposeGovernance: async (_request: GovernanceProposalRequest): Promise<ProposalOutcome> => ({
    problems: refusal('proposing this policy change'),
  }),

  proposeActivation: async (_request: ActivationProposalRequest): Promise<ProposalOutcome> => ({
    problems: refusal('activating this version'),
  }),

  /**
   * The Endorsement ledger (ADR-0061 §2). A snapshot taken before the
   * estate endorsed anything carries none, which is an empty ledger
   * rather than a failure.
   */
  endorsements: async (): Promise<EndorsementDoc[]> =>
    (await snapshot()).estate.endorsements ?? [],

  /** Setup guidance, generated on view from the snapshot's documents
   * exactly as the server generates it from its own (ADR-0060 §4). */
  setup: async (tier: string): Promise<SetupGuidance> => {
    const s = await snapshot()
    const guidance = setupGuidance(
      { ...s.estate, settings: settingsOf(s) },
      s.catalogues.active,
      tier,
    )
    if (!guidance) {
      // Say "cannot know", never fabricate: an unknown Tier is an error,
      // not invented guidance.
      throw new Error(`no Tier ${tier} is on this estate`)
    }
    return guidance
  },

  proposeTier: async (_request: TierProposalRequest): Promise<ProposalOutcome> => ({
    problems: refusal('proposing this Tier'),
  }),
}
