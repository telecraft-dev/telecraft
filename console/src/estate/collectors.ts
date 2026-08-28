import type { CollectorRow, TeamNode } from '../api/types'

// The Collectors band under the shelf (ADR-0063 §2): a bounded reflection
// of the shelf's scope and selection, never a second filter home. The flat
// list keeps the explicit filters (ADR-0042 §4); this module only chooses
// and orders what the band shows, so the choice is pure and testable.

/** How many rows the band shows before naming what it did not. */
export const BAND_LIMIT = 6

/** Every team id in the scope subtree: the shelf's own grouping walk. */
export function teamIds(root: TeamNode): Set<string> {
  const out = new Set<string>()
  const walk = (team: TeamNode) => {
    out.add(team.id)
    for (const child of team.teams ?? []) walk(child)
  }
  walk(root)
  return out
}

export interface CollectorsBandRows {
  /** The rows the band draws, at most BAND_LIMIT of them. */
  rows: CollectorRow[]
  /** How many collectors the scope holds, shown or not. */
  total: number
  /** How many of the selected Tier's collectors lead the rows. */
  leading: number
}

/**
 * The band's rows: the scope's collectors in payload order, the selected
 * Tier's matched collectors leading when there is a selection. Ungoverned
 * collectors belong to no team, so they are in scope only when the whole
 * estate is; at team scope the shelf's ungoverned band already carries
 * them (ADR-0031 §2).
 */
export function collectorsBand(
  collectors: CollectorRow[],
  scope: Set<string>,
  wholeEstate: boolean,
  selectedTier?: string,
): CollectorsBandRows {
  const inScope = collectors.filter((row) =>
    row.team !== undefined ? scope.has(row.team) : wholeEstate,
  )
  const leads =
    selectedTier === undefined ? [] : inScope.filter((row) => row.tier === selectedTier)
  const rest = inScope.filter((row) => !leads.includes(row))
  return {
    rows: [...leads, ...rest].slice(0, BAND_LIMIT),
    total: inScope.length,
    leading: leads.length,
  }
}
