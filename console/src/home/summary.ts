import {
  BAND_ORDER,
  type CardFace,
  type Environment,
  type EstatePayload,
  type RolloutDecision,
  type RolloutProgress,
  type Severity,
  type TeamNode,
  type UngovernedSummary,
} from '../api/types'
import { cardStanding, orderCards, totalFindings } from '../estate/order'
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

/** The total ungoverned population, both referents (ADR-0030, ADR-0031). */
export function ungovernedTotal(ungoverned: UngovernedSummary): number {
  return ungoverned.served + ungoverned.foreign
}

/** A card's finding total, re-exported so the surface reads one module. */
export { totalFindings }
