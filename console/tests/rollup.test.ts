import { describe, expect, it } from 'vitest'
import type { Band, BandName, CardFace, TeamNode } from '../src/api/types'
import { rollupTree } from '../src/estate/rollup'

// Roll-up per ADR-0017: ratio-plus-worst per finding kind, never blended,
// waived counts always alongside; every node aggregates its whole subtree,
// and teams owning nothing stay visible.

const okBand: Band = { state: 'ok', worstSeverity: 'none' }

function card(
  tier: string,
  team: string,
  environment: string,
  bands: Partial<Record<BandName, Band>> = {},
  extras: Partial<Pick<CardFace, 'findingCounts' | 'waivedCounts'>> = {},
): CardFace {
  return {
    contractVersion: 2,
    tier,
    name: tier,
    team,
    environment,
    bands: { delivery: okBand, expectation: okBand, conformance: okBand, ...bands },
    findingCounts: extras.findingCounts ?? {},
    waivedCounts: extras.waivedCounts,
    population: { matched: 1, floorSource: 'absent', state: 'ok' },
    signals: [],
    churn: { known: true, asOf: '2026-08-18T12:00:00Z', incarnations: 0 },
  }
}

const tree: TeamNode = {
  id: 'engineering',
  name: 'Engineering',
  teams: [
    { id: 'platform', name: 'Platform', teams: [{ id: 'data-flow', name: 'Data Flow' }] },
    { id: 'infosec', name: 'InfoSec' },
  ],
}

const cards = [
  card(
    'data-flow/gateway',
    'data-flow',
    'production',
    { conformance: { state: 'finding', worstSeverity: 'violation' } },
    { findingCounts: { conformance: 1 }, waivedCounts: { conformance: 1 } },
  ),
  card('data-flow/edge', 'data-flow', 'production'),
  card('data-flow/gateway-staging', 'data-flow', 'staging', {
    delivery: { state: 'pending_settle', worstSeverity: 'none' },
    expectation: { state: 'unknown', worstSeverity: 'none' },
    conformance: { state: 'not_applicable', worstSeverity: 'none' },
  }),
]

describe('rollupTree', () => {
  it('aggregates each subtree with ratio-plus-worst per kind, never blended', () => {
    const rows = rollupTree(tree, cards, 'production')
    const engineering = rows.find((row) => row.team.id === 'engineering')!
    expect(engineering.kinds.conformance).toEqual({
      passing: 1,
      counted: 2,
      worst: 'violation',
      waived: 1,
    })
    expect(engineering.kinds.delivery).toEqual({
      passing: 2,
      counted: 2,
      worst: 'none',
      waived: 0,
    })
  })

  it('keeps waived counts visible at every level of the tree', () => {
    const rows = rollupTree(tree, cards, 'production')
    for (const id of ['engineering', 'platform', 'data-flow']) {
      expect(rows.find((row) => row.team.id === id)!.kinds.conformance.waived).toBe(1)
    }
  })

  it('keeps teams owning nothing visible instead of hiding them as healthy', () => {
    const rows = rollupTree(tree, cards, 'production')
    const infosec = rows.find((row) => row.team.id === 'infosec')!
    expect(infosec.tiersTotal).toBe(0)
    expect(infosec.kinds.conformance.counted).toBe(0)
  })

  it('treats the lens as evaluation context and keeps other Environments countable', () => {
    const production = rollupTree(tree, cards, 'production')
    const staging = rollupTree(tree, cards, 'staging')
    const prodDataFlow = production.find((row) => row.team.id === 'data-flow')!
    const stagingDataFlow = staging.find((row) => row.team.id === 'data-flow')!
    expect(prodDataFlow.tiersInEnvironment).toBe(2)
    expect(stagingDataFlow.tiersInEnvironment).toBe(1)
    // The staging card carries no verdict: it counts as neutral, never passing.
    expect(stagingDataFlow.kinds.delivery.counted).toBe(0)
    expect(stagingDataFlow.neutral).toBe(1)
    // The all-Environments totals do not move with the lens.
    expect(prodDataFlow.findingsAllEnvironments).toBe(1)
    expect(stagingDataFlow.findingsAllEnvironments).toBe(1)
    expect(stagingDataFlow.waivedAllEnvironments).toBe(1)
  })
})
