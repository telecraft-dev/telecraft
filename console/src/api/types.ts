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
  population: Population
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
