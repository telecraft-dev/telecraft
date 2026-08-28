import { describe, expect, it } from 'vitest'
import type { CollectorRow, TeamNode } from '../src/api/types'
import { BAND_LIMIT, collectorsBand, teamIds } from '../src/estate/collectors'

// The Collectors band's row choice (ADR-0063 §2): the scope's collectors
// in payload order, the selected Tier's matched collectors leading, and a
// bound that names its truncation. The band filters nothing itself: the
// flat list keeps the explicit filters (ADR-0042 §4), so what is held
// here is a reflection, not a filter.

const teams: TeamNode = {
  id: 'engineering',
  name: 'Engineering',
  teams: [
    {
      id: 'platform',
      name: 'Platform Engineering',
      teams: [{ id: 'data-flow', name: 'Data Flow' }],
    },
    { id: 'product', name: 'Product' },
  ],
}

function collector(id: string, tier?: string, team?: string): CollectorRow {
  return {
    id,
    tier,
    team,
    ungoverned: tier === undefined ? 'served' : undefined,
    environment: 'production',
    state: 'reporting',
    version: '0.104.0',
    lastSeen: '2026-08-18T07:59:00Z',
  }
}

const rows: CollectorRow[] = [
  collector('edge-0', 'data-flow/edge', 'data-flow'),
  collector('edge-1', 'data-flow/edge', 'data-flow'),
  collector('gw-0', 'data-flow/gateway', 'data-flow'),
  collector('gw-1', 'data-flow/gateway', 'data-flow'),
  collector('sf-0', 'product/storefront-edge', 'product'),
  collector('loose-0'),
]

describe('teamIds', () => {
  it('walks the whole scope subtree', () => {
    expect(teamIds(teams)).toEqual(new Set(['engineering', 'platform', 'product', 'data-flow']))
    expect(teamIds(teams.teams![0]!)).toEqual(new Set(['platform', 'data-flow']))
  })
})

describe('collectorsBand', () => {
  it('holds the scope, in payload order, with nothing selected', () => {
    const band = collectorsBand(rows, new Set(['data-flow']), false)
    expect(band.rows.map((row) => row.id)).toEqual(['edge-0', 'edge-1', 'gw-0', 'gw-1'])
    expect(band.total).toBe(4)
    expect(band.leading).toBe(0)
  })

  it('leads with the selected Tier, then the rest of the scope', () => {
    const band = collectorsBand(rows, new Set(['data-flow']), false, 'data-flow/gateway')
    expect(band.rows.map((row) => row.id)).toEqual(['gw-0', 'gw-1', 'edge-0', 'edge-1'])
    expect(band.leading).toBe(2)
  })

  it('admits ungoverned collectors only when the whole estate is the scope', () => {
    const scoped = collectorsBand(rows, new Set(['data-flow', 'product']), false)
    expect(scoped.rows.map((row) => row.id)).not.toContain('loose-0')
    const estate = collectorsBand(rows, teamIds(teams), true)
    expect(estate.rows.map((row) => row.id)).toContain('loose-0')
    expect(estate.total).toBe(6)
  })

  it('bounds the rows and counts the whole scope', () => {
    const many = Array.from({ length: BAND_LIMIT + 3 }, (_, i) =>
      collector(`c-${i}`, 'data-flow/edge', 'data-flow'),
    )
    const band = collectorsBand(many, new Set(['data-flow']), false)
    expect(band.rows).toHaveLength(BAND_LIMIT)
    expect(band.total).toBe(BAND_LIMIT + 3)
  })

  it('leads with nothing when the selected Tier has no collectors in scope', () => {
    const band = collectorsBand(rows, new Set(['data-flow']), false, 'data-flow/payments-gateway')
    expect(band.leading).toBe(0)
    expect(band.rows.map((row) => row.id)).toEqual(['edge-0', 'edge-1', 'gw-0', 'gw-1'])
  })
})
