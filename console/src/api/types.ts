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
  /**
   * The teams whose objects this user may author changes to: their team
   * and every team beneath it, derived server-side from the ownership
   * tree (ADR-0019 §2, ADR-0016/0017). Surfaces offer authoring actions
   * exactly on objects owned inside this set.
   */
  editableTeams: string[]
}

/**
 * GET /api/v1/auth/providers — how sign-in works on this instance
 * (REQ-017, ADR-0019 §1). A `password` provider renders as a credential
 * form; a `redirect` provider renders as a link to
 * `/api/v1/auth/{name}/start`.
 */
export interface AuthProviderInfo {
  name: string
  flow: 'password' | 'redirect'
}

/**
 * The object kinds jump-to-object can reach (ADR-0042 §1): the authored
 * objects, plus `entry` — a Catalogue entry, keyed `class/type` (ADR-0020
 * §3), browsable and deep-linkable though machine-generated, never authored.
 */
export type ObjectKind =
  | 'tier'
  | 'service'
  | 'blueprint'
  | 'component'
  | 'team'
  | 'entry'
  | 'rollout'

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
 * How an ungoverned collector is read (ADR-0031 §1): `served` runs the
 * Unmatched artefact (ADR-0030) — commit-stamped, health-visible,
 * governed by nobody; `foreign` is read through the estate provider and
 * never served. No stigma attaches to the delivery path — only to
 * matching no selector.
 */
export type UngovernedKind = 'served' | 'foreign'

/**
 * GET /api/v1/collectors — per-collector detail, which lives in list
 * surfaces only (ADR-0042 §3.4): collector counts elsewhere are doors to
 * the flat list.
 */
export interface CollectorRow {
  id: string
  /** The selector-matched Tier — absent when no Tier selector matches (ADR-0031). */
  tier?: string
  /** Present exactly when `tier` is absent: how the ungoverned collector is read. */
  ungoverned?: UngovernedKind
  /** The matched Tier's owning team; an ungoverned collector has none. */
  team?: string
  environment: Environment
  state: 'reporting' | 'stale' | 'never_seen'
  version: string
  /** Last-known reading time, so last-known-plus-age renders (ADR-0040). */
  lastSeen?: string
  /**
   * Reported identifying attributes — the identity Tier selectors match
   * on (ADR-0013). For a served ungoverned collector this is the Unmatched
   * artefact's self-telemetry evidence (ADR-0030), the raw material the
   * claim flow's suggested selector generalises over (ADR-0042 §6).
   */
  attributes?: Record<string, string>
}

export interface TeamNode {
  id: string
  name: string
  teams?: TeamNode[]
}

/**
 * Ungoverned collectors in view (ADR-0031 §2): concern, never failure —
 * they appear with the onboard CTA and count in no compliance
 * denominator. The split names the two ways an ungoverned collector is
 * read; per-collector detail stays flat-list material (ADR-0042 §3.4).
 */
export interface UngovernedSummary {
  served: number
  foreign: number
}

/** GET /api/v1/estate — the shelf's bulk face payload (ADR-0041 §2). */
export interface EstatePayload {
  /** Production leads (ADR-0033). */
  environments: Environment[]
  teams: TeamNode
  cards: CardFace[]
  ungoverned: UngovernedSummary
}

/** The served vs git-delivered split: delivery path is a visible property
 * of a collector (ADR-0007), aggregated to Tier grain for the canvas. */
export interface DeliverySplit {
  served: number
  git: number
}

/**
 * A Tier as the topology canvas draws it: the authored node plus its
 * selector-matched counts. Collectors are never drawn — they are matched
 * into a Tier by selector and appear only as these numbers (ADR-0007);
 * the count is a door to the flat list (ADR-0042 §3.4).
 */
export interface TopologyTier {
  id: string
  name: string
  team: string
  environment: Environment
  /** Derived strictness, mirrored from the card face (ADR-0025). */
  serviceClass?: string
  /** Selector-matched collector count. */
  matched: number
  delivery: DeliverySplit
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

/**
 * How a rollout cohort member's config arrives (ADR-0029 §7): `served` by
 * the OpAMP path — membership computed per connect, acknowledgement by
 * config hash — or `foreign`, the adopter's own GitOps tooling, read
 * through the telecraft.tier stamp readings (ADR-0039 §5). The Foreign
 * population reads everything and blocks nothing: advisory, lag never
 * failure.
 */
export type RolloutPath = 'served' | 'foreign'

/**
 * Which of a Rollout's two artefacts a member actually runs, as far as its
 * readings can tell (ADR-0029 §7). Not knowing is a normal state
 * (ADR-0008): wiring both artefacts share reads `unknown`, never guessed.
 */
export type RolloutRunning = 'to' | 'from' | 'other' | 'unknown'

/** One delivery path's running split over a cohort's members. */
export interface RolloutPathProgress {
  members: number
  to: number
  from: number
  other: number
  unknown: number
}

/**
 * A stage's relation to the active stage: `entered` cohorts accumulate —
 * advancing only ever widens (ADR-0029 §4) — `active` is the stage the
 * evaluation judges, `pending` counts are the membership preview,
 * information for the reviewer, never the authoritative decision.
 */
export type RolloutCohortState = 'entered' | 'active' | 'pending'

/**
 * One cohort's progress: the cumulative membership up to and including
 * this stage (the union the server evaluates per connect), split by
 * delivery path, computed from stamps and the membership function.
 */
export interface RolloutCohortProgress {
  index: number
  /** The authored cohort spec, rendered for reading. */
  cohort: string
  /** The stage's authored minimum soak, for example `24h`. */
  soak: string
  state: RolloutCohortState
  /** Members this stage admits beyond the previous stage's cohort. */
  widens: number
  served: RolloutPathProgress
  /** Advisory (ADR-0029 §7): displayed, never blocking. */
  foreign: RolloutPathProgress
}

/** One halted cohort member (ADR-0029 §6) — the condition set is extensible. */
export interface RolloutHalt {
  collector: string
  path: RolloutPath
  condition: string
  reason: string
}

/**
 * The evaluation's verdict on the active stage (ADR-0029 §5, §6): halting
 * is passive — `blocked` is a withheld advance, nothing races; `abort` is
 * proposed at or past the threshold; `advance` is proposed for a human to
 * merge.
 */
export type RolloutDecision = 'hold' | 'blocked' | 'advance' | 'abort'

/** The numbers the verdict rests on, computed over the active cohort. */
export interface RolloutEvidence {
  membersSeen: number
  runningTo: number
  runningFrom: number
  runningOther: number
  unknown: number
  soaked: string
  minSoak: string
}

/**
 * GET /api/v1/rollouts — one active Rollout's cohort progress across both
 * delivery paths (ADR-0029): membership from the pure function, delivery
 * status against the rollout artefacts from commit stamps, halt and abort
 * states with provenance.
 */
export interface RolloutProgress {
  /** Team-qualified Rollout id, for example `data-flow/gateway-canary`. */
  id: string
  name: string
  team: string
  owner: string
  /** The one Tier this Rollout stages (ADR-0029 §2). */
  tier: string
  tierName: string
  environment: Environment
  /** The dual bindings, `team/name@version` (ADR-0029 §3). */
  from: string
  to: string
  /** The active stage, 0-based. */
  stage: number
  decision: RolloutDecision
  reason: string
  evidence: RolloutEvidence
  cohorts: RolloutCohortProgress[]
  halts: RolloutHalt[]
  /** The "why?" chain for the authored facts (ADR-0041 §3). */
  provenance: Provenance[]
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

/** The upstream signal names, in the lanes' fixed reading order (ADR-0024 §2). */
export const SIGNAL_ORDER = ['traces', 'logs', 'metrics', 'profiles'] as const
export type Signal = (typeof SIGNAL_ORDER)[number]

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
  tier?: string
  /** Local Components by bare name, each the Catalogue key it instantiates (ADR-0024 §3). */
  locals: Record<string, CatalogueKey>
  /** Per-signal lanes of ordered Component references (ADR-0024 §2). */
  lanes: Record<string, string[]>
  /** The collector-wide extensions block (ADR-0024 §2). */
  extensions: string[]
  /**
   * Version-stamped requirement claims (`req-id@3`, ADR-0026 §4): intent,
   * never fact — the UI must not blur the two (REQ-031).
   */
  satisfies: string[]
  /**
   * The Catalogue key each lane item instantiates — shared references
   * included — so composer lane items deep-link to their Catalogue entries
   * (ADR-0042 §1).
   */
  components?: Record<string, CatalogueKey>
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

/**
 * A Tier selector as authored (ADR-0007): attribute pairs matched by
 * string equality against a collector's reported identifying attributes —
 * every pair must equal, the most specific satisfied selector wins.
 */
export type Selector = Record<string, string>

/**
 * POST /api/v1/claims/preview — the claim flow's continuous evaluation
 * (ADR-0042 §6): the constrained selector plus context in, the impact out.
 * `mode` and `tier` join once the one question (attach or draft) is
 * answered, and the rendered Tier binding joins the answer.
 */
export interface ClaimPreviewRequest {
  selector: Selector
  environment: Environment
  team?: string
  mode?: 'attach' | 'draft'
  tier?: string
}

/** An existing Tier as an attach candidate, ranked by selector proximity. */
export interface ClaimCandidate {
  tier: string
  name: string
  team: string
  environment: Environment
  selector: Selector
  /** Selector pairs the claim already satisfies — the ranking key. */
  satisfied: number
  /** Pairs in the candidate's authored selector. */
  of: number
  /** The selector attach would leave behind: the shared pairs alone —
   * widened, never enumerated (ADR-0042 §6). */
  widened: Selector
}

/** A governed population the claim selector does not contradict — blast radius. */
export interface ClaimOverlap {
  tier: string
  matched: number
}

export interface ClaimPreview {
  /** Ungoverned collectors the selector matches now, by how they are read. */
  matched: { total: number; served: number; foreign: number }
  overlaps: ClaimOverlap[]
  candidates: ClaimCandidate[]
  /** The Tier binding as the PR would carry it, once a path is chosen. */
  rendered?: string
}

/**
 * POST /api/v1/claims — the attach exit (ADR-0042 §6): the named Tier's
 * selector widens to the claim's, and the change exits as a PR via the
 * forge adapter, user-attributed, carrying the rendered impact preview.
 * The console proposes, the PR decides.
 */
export interface ClaimRequest {
  selector: Selector
  environment: Environment
  team: string
  mode: 'attach' | 'draft'
  /** attach: the existing Tier to widen; draft: the new Tier's id. */
  tier: string
  title: string
}

/** The claim outcome: opened, or refused with the problems named (422). */
export interface ClaimOutcome {
  proposal?: Proposal
  problems?: string[]
}

/**
 * The claim context riding a Compose proposal for the draft-new-Tier path
 * (ADR-0042 §6): Compose opens with the selector pre-filled, and the PR
 * authors the Tier binding beside the drafted Blueprint.
 */
export interface ClaimContext {
  selector: Selector
  tier: string
  team: string
  environment: Environment
}
