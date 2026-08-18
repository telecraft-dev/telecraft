// The documented platform API, as the console consumes it (ADR-0045 §6:
// if the console needs it, the API grows it, documented). The contract is
// written up in console/README.md; the fixture backend implements it over
// a fixture estate until the platform binary grows the endpoint.

/** An Environment id, for example `production` or `staging`. */
export type Environment = string

/** GET /api/v1/me — the signed-in user, for scoping and the per-user store. */
export interface Me {
  id: string
  name: string
  /** The user's team id: the shelf's resting scope (ADR-0042 §2). */
  team: string
}

/**
 * The object kinds jump-to-object can reach (ADR-0042 §1): the authored
 * objects, plus `entry` — a Catalogue entry, keyed `class/type` (ADR-0020
 * §3), browsable and deep-linkable though machine-generated, never authored.
 */
export type ObjectKind = 'tier' | 'service' | 'blueprint' | 'component' | 'team' | 'entry'

export interface ObjectRef {
  kind: ObjectKind
  /** Team-qualified id, for example `data-flow/gateway`. */
  id: string
}

/** GET /api/v1/objects — the jump-to-object index. */
export interface IndexedObject extends ObjectRef {
  name: string
  /** Owning team — absent for Catalogue entries, which nobody owns. */
  team?: string
  environment?: Environment
}

/** The three card-face reading bands, in fixed order (ADR-0041 §2). */
export type BandName = 'delivery' | 'expectation' | 'conformance'
export const BAND_ORDER: readonly BandName[] = ['delivery', 'expectation', 'conformance']

/**
 * Band states are the contract; glyphs map from states and hue appears
 * nowhere in the contract (ADR-0041 §2). The honest neutrals stay distinct.
 */
export type BandState =
  | 'ok'
  | 'finding'
  | 'not_applicable'
  | 'unknown'
  | 'pending_settle'
  | 'stale_demoted'

export type Severity = 'violation' | 'advisory' | 'none'

export interface Band {
  state: BandState
  /** Worst finding severity when state is `finding`. */
  worstSeverity: Severity
  /** Optional worst-finding label for the face. */
  worstFinding?: string
}

/** The face payload's population line (ADR-0041 §2, ADR-0035). */
export interface Population {
  matched: number
  floor?: number
  floorSource: 'derived' | 'declared' | 'absent'
}

/**
 * The card face, contract version 1: the card unit is the Tier, and the
 * shelf groups and sorts from these fields alone (ADR-0041, ADR-0042 §2).
 */
export interface CardFace {
  contractVersion: 1
  /** Tier id — the contract keys on Tier id, never on a pair. */
  tier: string
  name: string
  /** Owning team id (a shelf summary field). */
  team: string
  environment: Environment
  /** Derived strictness, for example `C1` (ADR-0025). */
  serviceClass?: string
  bands: Record<BandName, Band>
  /** Finding counts per kind (a shelf summary field). */
  findingCounts: Record<string, number>
  /**
   * Waived finding counts per kind: an Exemption waives the count, never
   * the diagnosis, and waived counts ride every roll-up level
   * (ADR-0017, ADR-0037).
   */
  waivedCounts?: Record<string, number>
  population: Population
}

/** How dampening currently holds a finding (ADR-0035 §3, ADR-0037). */
export type DampeningState = 'none' | 'dampened' | 'waived'

/**
 * The who-acts routing target: the surface that can act on a finding,
 * as an object deep-link (ADR-0042 §3.3). Blueprint-shaped findings land
 * in Compose at the offending lane, grant-shaped in Governance,
 * delivery-shaped on the Tier in Topology. Inspect stays; action travels.
 */
export interface WhoActs {
  target: ObjectRef
  /** The offending signal lane, for Blueprint-shaped findings. */
  lane?: string
  label: string
}

/** A drawer finding: a finding without remediation is a complaint (ADR-0041 §3). */
export interface Finding {
  id: string
  kind: string
  severity: Severity
  dampening: DampeningState
  summary: string
  remediation: string
  whoActs: WhoActs
}

/** One config line implying a derived value (ADR-0041 §3). */
export interface ProvenanceLine {
  file: string
  line: number
  text: string
}

/**
 * A "why?" derivation as structured provenance: claim, the config lines
 * that implied it, and the SHA judged against — fed, never reconstructed
 * (ADR-0041 §3). Spatial derivations carry a trace action (ADR-0042 §5).
 */
export interface Provenance {
  /** Which face value this explains, for example `service-class` or `band:conformance`. */
  key: string
  claim: string
  lines: ProvenanceLine[]
  sha: string
  /** The optional travel action: trace this Service's Paths on the canvas. */
  trace?: { service: string }
}

/** GET /api/v1/drawer?tier= — the on-demand drawer payload (ADR-0041 §3). */
export interface CardDrawer {
  contractVersion: 1
  tier: string
  findings: Finding[]
  provenance: Provenance[]
}

/**
 * GET /api/v1/collectors — per-collector detail, which lives in list
 * surfaces only (ADR-0042 §3.4): collector counts elsewhere are doors to
 * the flat list.
 */
export interface CollectorRow {
  id: string
  tier: string
  team: string
  environment: Environment
  state: 'reporting' | 'stale' | 'never_seen'
  version: string
  /** Last-known reading time, so last-known-plus-age renders (ADR-0040). */
  lastSeen?: string
}

export interface TeamNode {
  id: string
  name: string
  teams?: TeamNode[]
}

/** GET /api/v1/estate — the shelf's bulk face payload (ADR-0041 §2). */
export interface EstatePayload {
  /** Production leads (ADR-0033). */
  environments: Environment[]
  teams: TeamNode
  cards: CardFace[]
}

/** A Tier as the topology canvas draws it. */
export interface TopologyTier {
  id: string
  name: string
  team: string
  environment: Environment
}

/** An ungoverned arrival source: sits in the dedicated band (ADR-0044 §2). */
export interface TopologySource {
  id: string
  name: string
}

/** A Hop: trust is a property of the Hop, never the Tier (ADR-0007). */
export interface TopologyHop {
  from: string
  to: string
  trusted: boolean
  signals: string[]
}

/** A Service's Paths through Tiers (ADR-0007). */
export interface TopologyPath {
  service: string
  through: string[]
}

/** GET /api/v1/topology — Tiers, Hops, and Paths at authored grain. */
export interface TopologyPayload {
  environments: Environment[]
  tiers: TopologyTier[]
  sources: TopologySource[]
  hops: TopologyHop[]
  paths: TopologyPath[]
}

/** GET /api/v1/blueprints — Blueprint summaries for the Compose workspace. */
export interface BlueprintSummary {
  id: string
  name: string
  version: number
  team: string
  /** Component references per signal pipeline, renderer order. */
  pipelines: Record<string, string[]>
  /**
   * The Catalogue key each pipeline item instantiates, so composer palette
   * items deep-link to their Catalogue entries (ADR-0042 §1).
   */
  components?: Record<string, CatalogueKey>
}

/**
 * A component class: upstream `status.class`, adopted verbatim — only the
 * five pipeline classes enter a Catalogue (ADR-0020 §2).
 */
export type ComponentClass = 'receiver' | 'processor' | 'exporter' | 'extension' | 'connector'

/** The Catalogue primary key (ADR-0020 §3): `type` alone collapses real components. */
export interface CatalogueKey {
  class: ComponentClass
  type: string
}

/** Renders the key in its authored `class/type` form — also the `entry` object id. */
export function formatCatalogueKey(key: CatalogueKey): string {
  return `${key.class}/${key.type}`
}

/** GET /api/v1/catalogue — governed Components for browsing (ADR-0020). */
export interface CatalogueComponent {
  id: string
  name: string
  version: number
  team: string
  class: ComponentClass
  type: string
}

/**
 * An upstream stability level, adopted verbatim: the maturity ladder is
 * development < alpha < beta < stable; deprecated and unmaintained are
 * lifecycle end-states, not rungs.
 */
export type StabilityLevel =
  | 'development'
  | 'alpha'
  | 'beta'
  | 'stable'
  | 'deprecated'
  | 'unmaintained'

/** The closed six-level vocabulary, ladder order first, for filter controls. */
export const STABILITY_LEVELS: readonly StabilityLevel[] = [
  'development',
  'alpha',
  'beta',
  'stable',
  'deprecated',
  'unmaintained',
]

/**
 * One Catalogue entry: identity, per-signal stability and lifecycle of a
 * component type (ADR-0020). It states what exists — never what may be used
 * (that is the Allow-list) and never a configured instance (that is a
 * governed Component).
 */
export interface CatalogueEntry extends CatalogueKey {
  /** The historical type alias, resolving on every lookup (ADR-0020 §3). */
  deprecatedType?: string
  displayName?: string
  description?: string
  /** Adopter-authored entries are first-class, marked apart (ADR-0020 §10). */
  source: 'upstream' | 'adopter'
  /** Per-signal stability; the signal vocabulary is open (ADR-0020 §1). */
  stability: Record<string, StabilityLevel>
  /** The upstream deprecation notice per deprecated signal — ready-made remediation. */
  deprecation?: Record<string, { date: string; migration: string }>
}

/** One installed catalogue version (ADR-0020 §9: retained, never replaced). */
export interface CatalogueVersionInfo {
  /** The collector release tag the catalogue is pinned to, e.g. `v0.158.0`. */
  version: string
  /** Whether this is the designated active catalogue authoring judges against. */
  active: boolean
  /** Entry count, for the version picker. */
  components: number
  source: { repository: string; ref: string; commit?: string }
}

/** GET /api/v1/catalogue/versions — the installed catalogues, active designated. */
export interface CatalogueVersionsPayload {
  active: string
  versions: CatalogueVersionInfo[]
}

/** An Owner in the team tree (ADR-0016), as governance authoring needs it. */
export interface OwnerDoc {
  id: string
  name: string
  team: string
}

/**
 * One Team's declared Allow-list, in its authored shape (ADR-0021 §5):
 * entries are `class/type-pattern` shapes, an exact name being the
 * degenerate pattern.
 */
export interface AllowListDoc {
  team: string
  owner: string
  allow: string[]
}

/** One Grant in its authored shape: an ancestor-authored scoped exception (ADR-0021 §3). */
export interface GrantDoc {
  id: string
  owner: string
  team: string
  adds: string[]
}

/**
 * GET /api/v1/governance — the authored, git-resident Allow-list policy
 * (ADR-0021 §5), plus the Owners governance edits attribute to. The console
 * derives each team's effective palette from this and the active catalogue.
 */
export interface GovernancePayload {
  owners: OwnerDoc[]
  allowLists: AllowListDoc[]
  grants: GrantDoc[]
}

/**
 * POST /api/v1/governance/proposals — a governance edit exiting as a PR via
 * the forge adapter (ADR-0042 §6): the complete edited policy, proposed;
 * the console proposes, the PR decides. The server validates fail-closed
 * exactly as loading does and answers 422 with the problems named.
 */
export interface GovernanceProposalRequest {
  title: string
  summary?: string
  allowLists: AllowListDoc[]
  grants: GrantDoc[]
}

/** The opened proposal, mirroring the forge seam: opaque id, URL, branch. */
export interface ProposalRef {
  id: string
  url: string
  branch: string
}

/** The proposal outcome: opened, or refused with the load problems named. */
export interface ProposalOutcome {
  proposal?: ProposalRef
  problems?: string[]
}
