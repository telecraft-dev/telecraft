import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import type { CardFace, TopologyHop, TopologyPath, TopologyPayload } from '../src/api/types'
import { layout } from '../src/engine/layout'
import { buildTopologyModel, pathHopPairs, pathTierIds, servicePaths } from '../src/surfaces/topology/model'

// The issue #28 core bet, headless: the fixture estate at the P3-verdict
// scale (~22k matched collectors) compiles to an engine model whose node
// count is the authored-object count, zero per-collector nodes
// (ADR-0007), with every edge derived from the model and every Path
// traversal drawn (ADR-0044 §3).

interface FixtureEstate {
  environments: string[]
  cards: CardFace[]
  topology: {
    sources: { id: string; name: string }[]
    delivery: Record<string, { served: number; git: number }>
    hops: TopologyHop[]
    paths: TopologyPath[]
  }
}

const estate = JSON.parse(
  readFileSync(new URL('../fixtures/estate.json', import.meta.url), 'utf8'),
) as FixtureEstate

// The payload exactly as the fixture backend serves it (tools/
// fixture-backend.mjs): matched counts single-sourced from the card faces.
const payload: TopologyPayload = {
  environments: estate.environments,
  tiers: estate.cards.map((card) => ({
    id: card.tier,
    name: card.name,
    team: card.team,
    environment: card.environment,
    ...(card.serviceClass ? { serviceClass: card.serviceClass } : {}),
    matched: card.population.matched,
    delivery: estate.topology.delivery[card.tier] ?? {
      served: card.population.matched,
      git: 0,
    },
  })),
  sources: estate.topology.sources,
  hops: estate.topology.hops,
  paths: estate.topology.paths,
}

describe('the fixture topology at P3-verdict scale', () => {
  it('carries roughly 22k matched collectors in its counts', () => {
    const total = payload.tiers.reduce((sum, tier) => sum + tier.matched, 0)
    expect(total).toBeGreaterThanOrEqual(20_000)
    expect(total).toBeLessThanOrEqual(25_000)
  })

  it('compiles to one node per authored object, zero per-collector nodes', () => {
    const model = buildTopologyModel(payload)
    expect(model.nodes).toHaveLength(payload.tiers.length + payload.sources.length)
    const geometry = layout(model)
    expect(geometry.nodes).toHaveLength(model.nodes.length)
  })

  it('partitions every matched count into the served/git delivery split', () => {
    for (const tier of payload.tiers) {
      expect(tier.delivery.served + tier.delivery.git).toBe(tier.matched)
    }
  })
})

describe('derived, never drawn edges', () => {
  it('derives every model edge from a Hop, one lane per signal', () => {
    const model = buildTopologyModel(payload)
    const lanes = payload.hops.reduce((sum, hop) => sum + hop.signals.length, 0)
    expect(model.edges).toHaveLength(lanes)
    const hopPairs = new Set(payload.hops.map((hop) => `${hop.from}→${hop.to}`))
    for (const edge of model.edges) {
      expect(hopPairs.has(`${edge.from}→${edge.to}`)).toBe(true)
    }
  })

  it('draws every governed pair a Path traverses', () => {
    const model = buildTopologyModel(payload)
    const drawn = new Set(model.edges.map((edge) => `${edge.from}→${edge.to}`))
    for (const pair of pathHopPairs(payload.paths)) {
      expect(drawn.has(pair)).toBe(true)
    }
  })
})

describe('multiple Paths per Service', () => {
  it('keeps each Path distinct, including the on-ramp shape straight to a gateway Tier', () => {
    const checkout = servicePaths(payload, 'product/checkout')
    expect(checkout.length).toBeGreaterThanOrEqual(2)
    const storefront = servicePaths(payload, 'product/storefront')
    // The no-collector-at-all shape (ADR-0007): a Path of exactly one
    // gateway Tier, legitimate and governed.
    expect(storefront.some((path) => path.through.length === 1)).toBe(true)
    expect(pathTierIds(checkout).size).toBeGreaterThan(0)
  })
})
