import { describe, expect, it } from 'vitest'
import { edgePath, layout } from '../src/engine/layout'
import type { EngineModel } from '../src/engine/types'

// The engine is a pure library, model in, geometry out, unit-testable
// without a browser (ADR-0044 §1, ADR-0045 §2). These tests hold the
// invariants: determinism, band containment, the ungoverned band's
// position, and Manhattan routing.

const model: EngineModel = {
  bands: [
    // Environment rows listed first on purpose: the ungoverned band must
    // still come out on top (ADR-0044 §2).
    { id: 'production', kind: 'environment', label: 'production' },
    { id: 'staging', kind: 'environment', label: 'staging' },
    { id: 'ungoverned', kind: 'ungoverned', label: 'ungoverned arrivals' },
  ],
  nodes: [
    { id: 'internet', band: 'ungoverned', label: 'internet' },
    { id: 'edge', band: 'production', label: 'edge' },
    { id: 'gateway', band: 'production', label: 'gateway' },
    { id: 'gateway-staging', band: 'staging', label: 'gateway-staging' },
  ],
  edges: [
    { id: 'e1', from: 'internet', to: 'gateway', signal: 'traces' },
    { id: 'e2', from: 'internet', to: 'gateway', signal: 'logs' },
    { id: 'e3', from: 'edge', to: 'gateway', signal: 'traces' },
  ],
}

describe('layout', () => {
  it('is deterministic: the same model yields identical geometry', () => {
    expect(layout(model)).toEqual(layout(model))
  })

  it('places the ungoverned band above every Environment row', () => {
    const geometry = layout(model)
    const ungoverned = geometry.bands.find((b) => b.kind === 'ungoverned')!
    for (const band of geometry.bands.filter((b) => b.kind === 'environment')) {
      expect(ungoverned.y).toBeLessThan(band.y)
    }
  })

  it('keeps every node inside its band', () => {
    const geometry = layout(model)
    const bandById = new Map(geometry.bands.map((b) => [b.id, b]))
    for (const node of geometry.nodes) {
      const band = bandById.get(node.band)!
      expect(node.y).toBeGreaterThanOrEqual(band.y)
      expect(node.y + node.height).toBeLessThanOrEqual(band.y + band.height)
    }
  })

  it('keeps a node in its band whatever arrangement offset is stored', () => {
    const arranged = layout({ ...model, arrangement: { gateway: 400 } })
    const plain = layout(model)
    const movedNode = arranged.nodes.find((n) => n.id === 'gateway')!
    const plainNode = plain.nodes.find((n) => n.id === 'gateway')!
    expect(movedNode.x).toBe(plainNode.x + 400)
    expect(movedNode.y).toBe(plainNode.y)
  })

  it('routes every edge as axis-aligned segments', () => {
    const geometry = layout(model)
    for (const edge of geometry.edges) {
      expect(edge.points.length).toBeGreaterThanOrEqual(2)
      for (let i = 0; i + 1 < edge.points.length; i++) {
        const a = edge.points[i]!
        const b = edge.points[i + 1]!
        expect(a.x === b.x || a.y === b.y).toBe(true)
      }
    }
  })

  it('separates parallel signals with distinct bend offsets', () => {
    const geometry = layout(model)
    const traces = geometry.edges.find((e) => e.id === 'e1')!
    const logs = geometry.edges.find((e) => e.id === 'e2')!
    expect(traces.points).not.toEqual(logs.points)
  })

  it('refuses a node naming an unknown band', () => {
    expect(() =>
      layout({
        bands: [{ id: 'production', kind: 'environment', label: 'production' }],
        nodes: [{ id: 'stray', band: 'nowhere', label: 'stray' }],
        edges: [],
      }),
    ).toThrow(/unknown band/)
  })

  it('refuses an edge naming an unknown node', () => {
    expect(() =>
      layout({
        bands: [{ id: 'production', kind: 'environment', label: 'production' }],
        nodes: [{ id: 'edge', band: 'production', label: 'edge' }],
        edges: [{ id: 'e', from: 'edge', to: 'ghost', signal: 'traces' }],
      }),
    ).toThrow(/unknown node/)
  })
})

describe('edgePath', () => {
  it('renders a polyline as an SVG path', () => {
    expect(
      edgePath({
        id: 'e',
        from: 'a',
        to: 'b',
        signal: 'traces',
        points: [
          { x: 0, y: 10 },
          { x: 20, y: 10 },
        ],
      }),
    ).toBe('M 0 10 L 20 10')
  })
})
