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
  /** Attribution address: proposals name the acting human (ADR-0014). */
  email: string
  /** The user's team id: the shelf's resting scope (ADR-0042 §2). */
  team: string
}

/** The authored-object kinds jump-to-object can reach (ADR-0042 §1). */
export type ObjectKind = 'tier' | 'service' | 'blueprint' | 'component' | 'team'

export interface ObjectRef {
  kind: ObjectKind
  /** Team-qualified id, for example `data-flow/gateway`. */
  id: string
}

/** GET /api/v1/objects — the jump-to-object index. */
export interface IndexedObject extends ObjectRef {
  name: string
  team: string
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

/** A component class (ADR-0020): where a lane entry may live. */
export type ComponentClass = 'receiver' | 'processor' | 'exporter' | 'extension' | 'connector'

/** The upstream signal names, in the lanes' fixed reading order (ADR-0024 §2). */
export const SIGNAL_ORDER = ['traces', 'logs', 'metrics', 'profiles'] as const
export type Signal = (typeof SIGNAL_ORDER)[number]

/** A local Component, declared inline and owned by the Blueprint's owner (ADR-0024 §3). */
export interface LocalComponent {
  class: ComponentClass
  type: string
}

/**
 * GET /api/v1/blueprints — Blueprint schema v1 documents (ADR-0024): the
 * domain document the Compose Workspace opens and edits. Lane entries are
 * Component references — a bare name is a local Component, `team/name@pin`
 * a shared one; the lists are explicitly ordered and never re-sorted.
 */
export interface BlueprintDoc {
  id: string
  name: string
  /** Explicit monotonic version, bumped in the same PR as the change (ADR-0024 §7). */
  version: number
  team: string
  /** The Tier the Blueprint is bound to (ADR-0025): the evaluation context. */
  tier: string
  /** Local Components by bare name (ADR-0024 §3). */
  locals: Record<string, LocalComponent>
  /** Per-signal lanes of ordered Component references (ADR-0024 §2). */
  lanes: Record<string, string[]>
  /** The collector-wide extensions block (ADR-0024 §2). */
  extensions: string[]
  /**
   * Version-stamped requirement claims (`req-id@3`, ADR-0026 §4): intent,
   * never fact — the UI must not blur the two (REQ-031).
   */
  satisfies: string[]
}

/** How the evaluator judged one palette entry's presence (ADR-0021 §3). */
export type PaletteOrigin = 'default-allow' | 'allow-list' | 'grant'

/**
 * One palette entry, judged for the evaluation context (ADR-0022 §5):
 * allowed shown, floor-breaching greyed with the reason, non-allowed hidden
 * (counted in `ComposePalette.hidden`, never listed). Pure presentation of
 * the evaluator's verdicts — the palette enforces nothing.
 */
export interface PaletteEntry {
  key: string
  label: string
  class: ComponentClass
  type: string
  /** `type` entries add a fresh local Component; `shared` a pinned reference (ADR-0024). */
  residence: 'type' | 'shared'
  /** The signals the entry supports; click-add targets all of them (ADR-0043 §4). */
  signals: string[]
  /** Upstream stability per signal — the per-(component, signal) chips (ADR-0023). */
  stability: Record<string, string>
  /** What an add gesture inserts: the pinned reference for shared entries. */
  add: { ref?: string; signals: string[] }
  state: 'allowed' | 'greyed'
  /** The greyed reason, for example `alpha on traces — below this Service's C1 floor`. */
  reason?: string
  origin: PaletteOrigin
  /** Grant provenance when origin is `grant` — the audit chain is total (ADR-0021 §3). */
  grant?: { id: string; grantedBy: string; grantedTo: string }
  deprecated?: { migration: string }
}

export interface ComposePalette {
  entries: PaletteEntry[]
  /** The admitted count behind "N components hidden by your allow-list". */
  hidden: number
}

/** The finding kinds the engine raises while composing (ADR-0022, ADR-0024 §6). */
export type ComposeFindingKind = 'reference' | 'allow-list' | 'floor' | 'lifecycle' | 'ordering'

/** One live finding on the open draft; only `allow-list` ever blocks. */
export interface ComposeFinding {
  id: string
  kind: ComposeFindingKind
  severity: Severity
  lane?: string
  ref?: string
  summary: string
  remediation: string
}

/**
 * One requirement verdict for the Requirement-first surface: `claimed` is
 * the draft's `satisfies` intent, `met` the engine's judgement — carried
 * side by side, never blended (REQ-031, ADR-0026 §5).
 */
export interface RequirementVerdict {
  id: string
  version: number
  summary: string
  remediation: string
  claimed: boolean
  claimedVersion?: number
  met: boolean
  /** The one-click suggestion: what to add, and to which lanes. */
  suggestion: { ref?: string; type?: string; signals: string[] }
}

/**
 * POST /api/v1/validate — the one evaluator (ADR-0022 §1): the open draft
 * plus its evaluation context in, every verdict out. Stateless and
 * continuous; Save calls the same rulebook with enforcement on.
 */
export interface ComposeVerdict {
  /** The evaluation context echoed back: the lens as context (ADR-0042 §4). */
  context: { team: string; environment: Environment; serviceClass?: string; floor?: string }
  findings: ComposeFinding[]
  palette: ComposePalette
  requirements: RequirementVerdict[]
  /** The one hard block: an allow-list violation disables Save (ADR-0022 §3). */
  save: { blocked: boolean; reasons: string[] }
  /** The rendered-artefact preview for the read-only flyout (REQ-035). */
  yaml: string
}

/**
 * POST /api/v1/proposals — the composer exit (ADR-0043 §6): a change
 * proposal through the forge adapter, render-in-PR, user-attributed
 * (ADR-0028, ADR-0014). The console proposes, the PR decides.
 */
export interface Proposal {
  id: string
  url: string
  branch: string
  attributedTo: string
}

/** GET /api/v1/catalogue — governed Components for browsing (ADR-0020). */
export interface CatalogueComponent {
  id: string
  name: string
  version: number
  team: string
  class: 'receiver' | 'processor' | 'exporter' | 'extension' | 'connector'
  type: string
}
