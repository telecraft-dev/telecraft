import { BAND_ORDER, type CardFace, type Severity } from '../api/types'

// Shelf ordering (ADR-0042 §2): cards order worst-severity-first from face
// summary fields alone, tie-broken on finding counts; neutral and
// no-verdict cards sink to the row's tail but are never hidden.

const NEUTRAL_STATES = new Set(['not_applicable', 'unknown', 'pending_settle', 'stale_demoted'])

export type CardStanding = 'violation' | 'advisory' | 'ok' | 'neutral'

const STANDING_RANK: Record<CardStanding, number> = {
  violation: 3,
  advisory: 2,
  ok: 1,
  neutral: 0,
}

/** The card's worst standing across its three bands, from face fields alone. */
export function cardStanding(card: CardFace): CardStanding {
  let sawOk = false
  let worst: Severity = 'none'
  for (const band of BAND_ORDER) {
    const { state, worstSeverity } = card.bands[band]
    if (state === 'finding') {
      if (worstSeverity === 'violation') return 'violation'
      if (worstSeverity === 'advisory') worst = 'advisory'
    } else if (!NEUTRAL_STATES.has(state)) {
      sawOk = true
    }
  }
  if (worst === 'advisory') return 'advisory'
  return sawOk ? 'ok' : 'neutral'
}

export function totalFindings(card: CardFace): number {
  return Object.values(card.findingCounts).reduce((sum, n) => sum + n, 0)
}

/** Orders one row's cards worst-first; neutrals sink to the tail. */
export function orderCards(cards: CardFace[]): CardFace[] {
  return [...cards].sort((a, b) => {
    const rank = STANDING_RANK[cardStanding(b)] - STANDING_RANK[cardStanding(a)]
    if (rank !== 0) return rank
    const findings = totalFindings(b) - totalFindings(a)
    if (findings !== 0) return findings
    return a.tier.localeCompare(b.tier)
  })
}

/** Whether every card in a section is healthy, so it collapses to a summary line. */
export function sectionAllHealthy(cards: CardFace[]): boolean {
  return cards.every((card) => {
    const standing = cardStanding(card)
    return standing === 'ok' || standing === 'neutral'
  })
}
