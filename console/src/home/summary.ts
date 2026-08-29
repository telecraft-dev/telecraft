import {
  BAND_ORDER,
  type BandName,
  type CardFace,
  type Environment,
  type EstatePayload,
  type RolloutDecision,
  type RolloutProgress,
  type Severity,
  type SignalRow,
  type TeamNode,
  type UngovernedSummary,
} from '../api/types'
import { cardStanding, orderCards, totalFindings } from '../estate/order'
import { errorReadings, formatItems, laneReads, readingState } from '../estate/readings'
import { rollupTree, type TeamRollup } from '../estate/rollup'

// Home's derivation (ADR-0056 §2): the landing surface answers "where do I
// look first?" and judges nothing of its own. Every number below is read
// from a module that already owns the judgement: `estate/rollup.ts` for
// ratio-plus-worst per finding kind (ADR-0017), `estate/order.ts` for a
// card's standing and the shelf's worst-first order (ADR-0042 §2). A
// summary that computed its own verdict could disagree with the surface it
// points at, and a reader would have no way to tell which one was right.
//
// Nothing here blends. There is no estate health score, at this grain or
// any other: the root row carries a ratio, a worst, and a waived count per
// kind, exactly as the tree-table's rows do (ADR-0056 §3).

/** The finding kinds' on-screen labels, shared by the tiles and the rows. */
export const KIND_LABEL: Record<BandName, string> = {
  delivery: 'Delivery',
  expectation: 'Expectation',
  conformance: 'Conformance',
}

/** How many Tiers a standing tile names before it starts counting instead. */
export const STANDING_TIERS_NAMED = 3

/**
 * The Tiers behind one tile's number, named within a bound (ADR-0056 §5):
 * the bound reports what it dropped, because a truncation that does not
 * say it truncated reads as an all-clear.
 */
export interface NamedTiers {
  /** The cards named on the tile, in the shelf's own order. */
  shown: CardFace[]
  /** Cards behind the number the tile did not name. */
  more: number
}

/** How many worst Tiers Home draws before it starts counting instead. */
export const WORST_TIERS_SHOWN = 6

/** How many Team subtrees Home draws before it starts counting instead. */
export const TEAMS_SHOWN = 6

/** How many Rollouts Home draws before it starts counting instead. */
export const ROLLOUTS_SHOWN = 4

/** A card wants attention when its worst standing carries a finding. */
export function needsAttention(card: CardFace): boolean {
  const standing = cardStanding(card)
  return standing === 'violation' || standing === 'advisory'
}

const SEVERITY_RANK: Record<Severity, number> = { violation: 2, advisory: 1, none: 0 }

/** A roll-up row's worst severity across the three finding kinds. */
export function teamStanding(row: TeamRollup): Severity {
  let worst: Severity = 'none'
  for (const kind of BAND_ORDER) {
    if (SEVERITY_RANK[row.kinds[kind].worst] > SEVERITY_RANK[worst]) worst = row.kinds[kind].worst
  }
  return worst
}

/**
 * The order a Rollout's verdict earns on Home (ADR-0029 §5, §6). `abort`
 * and `advance` are both proposals waiting on a person to merge, and
 * `blocked` is an advance withheld because something is wrong; `hold` is
 * the steady state and sinks to the tail, where it is counted rather than
 * drawn.
 */
const DECISION_RANK: Record<RolloutDecision, number> = {
  abort: 3,
  blocked: 2,
  advance: 1,
  hold: 0,
}

/** Whether a Rollout's verdict is waiting on a person rather than on time. */
export function rolloutWaiting(rollout: RolloutProgress): boolean {
  return rollout.decision !== 'hold'
}

/** The top-level Team subtrees: the root's children, or the root alone. */
function topLevel(root: TeamNode): TeamNode[] {
  const children = root.teams ?? []
  return children.length > 0 ? [...children] : [root]
}

/**
 * Home's whole model (ADR-0056). The lens is the evaluation context, as it
 * is for the tree-table roll-up (ADR-0042 §4), and every lens-judged count
 * carries its all-Environments companion so the lens hides nothing.
 *
 * Every list is bounded and every bound reports what it left out
 * (ADR-0056 §5): a truncation that does not say it truncated reads as an
 * all-clear, which is the sin the denominator rule already forbids.
 */
export interface HomeSummary {
  lens: Environment
  /** The estate root row of the same roll-up the tree-table draws. */
  standing: TeamRollup
  /**
   * Per finding kind, the Tiers behind the tile's number: the Tiers whose
   * band carries a finding when any does, otherwise the Tiers the ratio
   * counted as passing. Read off the same band states the roll-up counts,
   * so the names and the number cannot disagree.
   */
  standingTiers: Record<BandName, NamedTiers>
  /** The Tiers with no verdict-bearing band, behind the neutral count. */
  neutralTiers: NamedTiers
  /** The worst Tiers in the lens Environment, in the shelf's own order. */
  worstTiers: CardFace[]
  /** Tiers wanting attention in the lens Environment, drawn or not. */
  attentionInLens: number
  /** And in every other Environment, so the lens conceals no finding. */
  attentionElsewhere: number
  /** Top-level Team subtrees, worst-first, each aggregating its whole subtree. */
  teams: TeamRollup[]
  /** Team subtrees in total, so the bound above names what it dropped. */
  teamsTotal: number
  /** The ungoverned population: a concern with a CTA, never a failure (ADR-0031). */
  ungoverned: UngovernedSummary
  /** Rollouts whose verdict wants a person, worst-verdict first. */
  rollouts: RolloutProgress[]
  /** Rollouts waiting on a person, drawn or not. */
  rolloutsWaiting: number
  /** Rollouts holding steady: counted, because nothing there needs doing. */
  rolloutsSteady: number
}

export function summarise(
  payload: EstatePayload,
  rollouts: RolloutProgress[],
  lens: Environment,
): HomeSummary {
  const rows = rollupTree(payload.teams, payload.cards, lens)
  // rollupTree walks depth-first from the root, so the first row is the
  // estate, and every row aggregates its own whole subtree.
  const [standing] = rows
  if (!standing) {
    // Unreachable: rollupTree pushes a node before walking its children, so
    // it always returns at least the root. The check is here rather than a
    // `!` because an assertion is a claim and this is a check.
    throw new Error('the estate roll-up produced no root row')
  }
  const byTeam = new Map(rows.map((row) => [row.team.id, row]))

  const inLens = payload.cards.filter((card) => card.environment === lens)
  const attention = inLens.filter(needsAttention)
  const worstTiers = orderCards(attention).slice(0, WORST_TIERS_SHOWN)

  const named = (pool: CardFace[]): NamedTiers => {
    const ordered = orderCards(pool)
    return {
      shown: ordered.slice(0, STANDING_TIERS_NAMED),
      more: Math.max(0, ordered.length - STANDING_TIERS_NAMED),
    }
  }
  // Each tile names what its number counts, from the band states the
  // roll-up counts (rollupOne's own rule): a kind carrying findings names
  // the Tiers with them, a clean kind names the Tiers it counted, and the
  // neutral tile names the cards the roll-up left out of every ratio.
  const standingTiers = {} as Record<BandName, NamedTiers>
  for (const kind of BAND_ORDER) {
    const withFinding = inLens.filter((card) => card.bands[kind].state === 'finding')
    standingTiers[kind] = named(
      withFinding.length > 0
        ? withFinding
        : inLens.filter((card) => card.bands[kind].state === 'ok'),
    )
  }
  const neutralTiers = named(inLens.filter((card) => cardStanding(card) === 'neutral'))

  const teamRows = topLevel(payload.teams)
    .map((team) => byTeam.get(team.id))
    .filter((row): row is TeamRollup => row !== undefined)
    .sort((a, b) => {
      const severity = SEVERITY_RANK[teamStanding(b)] - SEVERITY_RANK[teamStanding(a)]
      if (severity !== 0) return severity
      const findings = b.findingsAllEnvironments - a.findingsAllEnvironments
      if (findings !== 0) return findings
      return a.team.name.localeCompare(b.team.name)
    })

  const waiting = rollouts
    .filter(rolloutWaiting)
    .sort((a, b) => {
      const decision = DECISION_RANK[b.decision] - DECISION_RANK[a.decision]
      if (decision !== 0) return decision
      return a.name.localeCompare(b.name)
    })

  return {
    lens,
    standing,
    standingTiers,
    neutralTiers,
    worstTiers,
    attentionInLens: attention.length,
    attentionElsewhere: payload.cards.filter(
      (card) => card.environment !== lens && needsAttention(card),
    ).length,
    teams: teamRows.slice(0, TEAMS_SHOWN),
    teamsTotal: teamRows.length,
    ungoverned: payload.ungoverned,
    rollouts: waiting.slice(0, ROLLOUTS_SHOWN),
    rolloutsWaiting: waiting.length,
    rolloutsSteady: rollouts.length - waiting.length,
  }
}

/**
 * One segment of a Tier row's second line. A segment that reads a lane
 * names it, and the name is carried apart from the words around it so the
 * row can set it in the lane's colour without tinting the reading it sits
 * in: an error count is a severity reading and must not take a lane hue
 * (ADR-0047 §5, ADR-0041 §2).
 */
export interface TierSegment {
  /** The words before the lane's name, or the whole segment where it reads none. */
  lead: string
  /** The lane the segment reads, where it reads one. */
  signal?: string
  /** The words after the lane's name. */
  trail?: string
}

/**
 * A Tier row's second line, from card-face fields alone (ADR-0056 §2): the
 * worst finding band's face label where the face carries one, then the
 * per-lane facts the card's own matrix shows. No drawer is fetched, and the
 * lane facts read through `estate/readings.ts`, the module the matrix
 * itself reads through, so this line and the card cannot disagree.
 */
export function tierDetail(card: CardFace): TierSegment[] {
  const segments: TierSegment[] = []

  let lead: BandName | undefined
  for (const kind of BAND_ORDER) {
    const band = card.bands[kind]
    if (band.state !== 'finding') continue
    if (
      lead === undefined ||
      SEVERITY_RANK[band.worstSeverity] > SEVERITY_RANK[card.bands[lead].worstSeverity]
    ) {
      lead = kind
    }
  }
  if (lead !== undefined) {
    segments.push({ lead: card.bands[lead].worstFinding ?? `${KIND_LABEL[lead]}: finding` })
  }

  for (const row of card.signals) {
    if (!laneReads(row)) {
      segments.push({ lead: 'no ', signal: row.signal, trail: ' lane on this Tier' })
      continue
    }
    // A wired lane's readings can still be absent outright on a card no
    // collector has reported for, so each is read only where it exists.
    const { volume, freshness, shape } = row as SignalRow
    if (freshness !== undefined && readingState(freshness) === 'silent') {
      segments.push({ lead: '', signal: row.signal, trail: ' silent' })
    }
    if (shape?.known === true && shape.missing > 0) {
      segments.push({
        lead: `${shape.missing} of ${shape.required} `,
        signal: row.signal,
        trail: ' missing',
      })
    }
    if (volume !== undefined) {
      for (const error of errorReadings(volume)) {
        segments.push({
          lead: `${formatItems(error.items)} `,
          signal: row.signal,
          trail: ` ${error.label}`,
        })
      }
    }
  }
  return segments
}

/* Halt conditions arrive as the evaluator's enum names, and the set is
   explicitly extensible (ADR-0029 §6): the known ones read exactly as the
   rollout ledger words them, and an unmapped one falls back to its name
   with the underscores spaced out. The map is repeated from the ledger
   rather than imported, because the ledger is a lazily loaded Workspace
   and Home is the eagerly loaded entry (ADR-0056): importing it here would
   pull that whole surface into the entry chunk. */
const HALT_CONDITION_LABEL: Record<string, string> = {
  failed: 'apply failed',
  went_dark: 'went dark',
}

function haltConditionLabel(condition: string): string {
  return HALT_CONDITION_LABEL[condition] ?? condition.replace(/_/g, ' ')
}

/**
 * A Rollout row's position, from the payload Home already reads (ADR-0056
 * §2): the active stage, the running split in the ledger's own words, and
 * the halted member by name. Halts beyond the first are counted rather
 * than named (§5).
 */
export function rolloutPosition(rollout: RolloutProgress): string[] {
  const segments: string[] = []
  if (rollout.cohorts.length > 0) {
    segments.push(`stage ${rollout.stage + 1} of ${rollout.cohorts.length}`)
  }
  const seen = rollout.evidence.membersSeen
  if (typeof seen === 'number' && seen > 0) {
    segments.push(`${rollout.evidence.runningTo} of ${seen} on the new version`)
  }
  const [first] = rollout.halts
  if (first !== undefined) {
    segments.push(`${first.collector} ${haltConditionLabel(first.condition)}`)
    if (rollout.halts.length > 1) segments.push(`and ${rollout.halts.length - 1} more halted`)
  }
  return segments
}

/** The total ungoverned population, both referents (ADR-0030, ADR-0031). */
export function ungovernedTotal(ungoverned: UngovernedSummary): number {
  return ungoverned.served + ungoverned.foreign
}

/** A card's finding total, re-exported so the surface reads one module. */
export { totalFindings }
