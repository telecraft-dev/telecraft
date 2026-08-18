import { describe, expect, it } from 'vitest'
import type { Band, BandName, CardFace } from '../src/api/types'
import { cardStanding, orderCards, sectionAllHealthy } from '../src/estate/order'

// Shelf ordering from face summary fields alone (ADR-0042 §2): worst
// severity first, tie-broken on finding counts, neutrals sinking to the
// tail but never hidden.

const okBand: Band = { state: 'ok', worstSeverity: 'none' }

function card(
  tier: string,
  bands: Partial<Record<BandName, Band>>,
  findingCounts: Record<string, number> = {},
): CardFace {
  return {
    contractVersion: 1,
    tier,
    name: tier,
    team: 'data-flow',
    environment: 'production',
    bands: {
      delivery: okBand,
      expectation: okBand,
      conformance: okBand,
      ...bands,
    },
    findingCounts,
    population: { matched: 1, floorSource: 'absent' },
  }
}

const violating = card(
  'violating',
  { conformance: { state: 'finding', worstSeverity: 'violation' } },
  { conformance: 1 },
)
const advisory = card(
  'advisory',
  { expectation: { state: 'finding', worstSeverity: 'advisory' } },
  { expectation: 1 },
)
const healthy = card('healthy', {})
const neutral = card('neutral', {
  delivery: { state: 'pending_settle', worstSeverity: 'none' },
  expectation: { state: 'unknown', worstSeverity: 'none' },
  conformance: { state: 'not_applicable', worstSeverity: 'none' },
})

describe('cardStanding', () => {
  it('reads the worst band from face fields alone', () => {
    expect(cardStanding(violating)).toBe('violation')
    expect(cardStanding(advisory)).toBe('advisory')
    expect(cardStanding(healthy)).toBe('ok')
    expect(cardStanding(neutral)).toBe('neutral')
  })
})

describe('orderCards', () => {
  it('orders worst-first with neutrals sinking to the tail, never dropped', () => {
    const ordered = orderCards([neutral, healthy, advisory, violating])
    expect(ordered.map((c) => c.tier)).toEqual(['violating', 'advisory', 'healthy', 'neutral'])
  })

  it('breaks severity ties on finding counts', () => {
    const one = card(
      'one-finding',
      { conformance: { state: 'finding', worstSeverity: 'violation' } },
      { conformance: 1 },
    )
    const three = card(
      'three-findings',
      { conformance: { state: 'finding', worstSeverity: 'violation' } },
      { conformance: 2, expectation: 1 },
    )
    expect(orderCards([one, three]).map((c) => c.tier)).toEqual(['three-findings', 'one-finding'])
  })
})

describe('sectionAllHealthy', () => {
  it('collapses only sections with no findings', () => {
    expect(sectionAllHealthy([healthy, neutral])).toBe(true)
    expect(sectionAllHealthy([healthy, advisory])).toBe(false)
    expect(sectionAllHealthy([violating])).toBe(false)
  })
})
