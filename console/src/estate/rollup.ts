import {
  BAND_ORDER,
  type BandName,
  type CardFace,
  type Environment,
  type Severity,
  type TeamNode,
} from '../api/types'

// Team roll-up (ADR-0017): aggregation is ratio-plus-worst per finding
// kind, never blended: a passing-over-counted ratio, a worst-outcome
// badge, and the waived count always alongside, so an exemption-heavy 100%
// cannot hide. Every node of the tree aggregates its whole subtree; no
// single blended number exists at any level.

export interface KindRollup {
  /** Cards whose band carries a verdict and no finding of this kind. */
  passing: number
  /** Cards whose band carries a verdict (ok or finding); neutrals do not count. */
  counted: number
  worst: Severity
  /** Waived findings of this kind: waived counts ride every level (ADR-0037). */
  waived: number
}

export interface TeamRollup {
  team: TeamNode
  depth: number
  /** Evaluated under one Environment: the lens as evaluation context (ADR-0042 §4). */
  kinds: Record<BandName, KindRollup>
  /** Cards in the evaluated Environment with no verdict-bearing band at all. */
  neutral: number
  /** Tiers the subtree owns in the evaluated Environment / in total. */
  tiersInEnvironment: number
  tiersTotal: number
  /** Across every Environment, so the lens never hides anything. */
  findingsAllEnvironments: number
  waivedAllEnvironments: number
}

function emptyKinds(): Record<BandName, KindRollup> {
  return {
    delivery: { passing: 0, counted: 0, worst: 'none', waived: 0 },
    expectation: { passing: 0, counted: 0, worst: 'none', waived: 0 },
    conformance: { passing: 0, counted: 0, worst: 'none', waived: 0 },
  }
}

function worse(a: Severity, b: Severity): Severity {
  if (a === 'violation' || b === 'violation') return 'violation'
  if (a === 'advisory' || b === 'advisory') return 'advisory'
  return 'none'
}

function subtreeCards(team: TeamNode, cards: CardFace[]): CardFace[] {
  const ids = new Set<string>()
  const walk = (node: TeamNode) => {
    ids.add(node.id)
    for (const child of node.teams ?? []) walk(child)
  }
  walk(team)
  return cards.filter((card) => ids.has(card.team))
}

function rollupOne(team: TeamNode, depth: number, cards: CardFace[], env: Environment): TeamRollup {
  const owned = subtreeCards(team, cards)
  const inEnv = owned.filter((card) => card.environment === env)
  const kinds = emptyKinds()
  let neutral = 0

  for (const card of inEnv) {
    let verdicts = 0
    for (const band of BAND_ORDER) {
      const { state, worstSeverity } = card.bands[band]
      if (state === 'ok') {
        kinds[band].counted += 1
        kinds[band].passing += 1
        verdicts += 1
      } else if (state === 'finding') {
        kinds[band].counted += 1
        kinds[band].worst = worse(kinds[band].worst, worstSeverity)
        verdicts += 1
      }
    }
    if (verdicts === 0) neutral += 1
    for (const band of BAND_ORDER) {
      kinds[band].waived += card.waivedCounts?.[band] ?? 0
    }
  }

  const sum = (counts: Record<string, number> | undefined) =>
    Object.values(counts ?? {}).reduce((n, c) => n + c, 0)

  return {
    team,
    depth,
    kinds,
    neutral,
    tiersInEnvironment: inEnv.length,
    tiersTotal: owned.length,
    findingsAllEnvironments: owned.reduce((n, card) => n + sum(card.findingCounts), 0),
    waivedAllEnvironments: owned.reduce((n, card) => n + sum(card.waivedCounts), 0),
  }
}

/**
 * One row per tree node, depth-first, each aggregating its whole subtree:
 * a parent team's view is bigger than the sum of its services (ADR-0017).
 * Teams owning nothing stay visible: hiding reads as healthy (ADR-0042 §2).
 */
export function rollupTree(root: TeamNode, cards: CardFace[], env: Environment): TeamRollup[] {
  const rows: TeamRollup[] = []
  const walk = (team: TeamNode, depth: number) => {
    rows.push(rollupOne(team, depth, cards, env))
    for (const child of team.teams ?? []) walk(child, depth + 1)
  }
  walk(root, 0)
  return rows
}
