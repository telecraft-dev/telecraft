import { describe, expect, it } from 'vitest'
import type { BlueprintDoc, PaletteEntry, RequirementVerdict } from '../src/api/types'
import { addEntry, addSuggestion, localNameFor, removeEntry, stampClaim } from '../src/surfaces/compose/draft'

// The draft-edit helpers: three add gestures, one semantics (ADR-0043 §4).
// Every gesture funnels through addEntry, so click-add, targeted add and
// drag-authoring cannot diverge; a type add creates a local Component
// (ADR-0024 §3) and everything lands at the lane tail, never re-sorted.

const doc = (): BlueprintDoc => ({
  id: 'data-flow/edge-standard',
  name: 'edge-standard',
  version: 2,
  team: 'data-flow',
  tier: 'data-flow/edge',
  locals: {
    'otlp-in': { class: 'receiver', type: 'otlp' },
    batcher: { class: 'processor', type: 'batch' },
  },
  lanes: {
    traces: ['otlp-in', 'batcher', 'data-flow/gateway-exporter@2'],
    logs: ['otlp-in', 'batcher', 'data-flow/gateway-exporter@2'],
  },
  extensions: [],
  satisfies: [],
})

const typeEntry = (over: Partial<PaletteEntry> = {}): PaletteEntry => ({
  key: 'type:processor/memory_limiter',
  label: 'memory_limiter',
  class: 'processor',
  type: 'memory_limiter',
  residence: 'type',
  signals: ['traces', 'logs', 'metrics'],
  stability: { traces: 'beta', logs: 'beta', metrics: 'beta' },
  add: { signals: ['traces', 'logs', 'metrics'] },
  state: 'allowed',
  origin: 'allow-list',
  ...over,
})

const sharedEntry: PaletteEntry = {
  key: 'shared:infosec/pii-redaction',
  label: 'infosec/pii-redaction',
  class: 'processor',
  type: 'transform',
  residence: 'shared',
  signals: ['traces', 'logs', 'metrics'],
  stability: { traces: 'beta', logs: 'beta', metrics: 'beta' },
  add: { ref: 'infosec/pii-redaction@3', signals: ['traces', 'logs', 'metrics'] },
  state: 'allowed',
  origin: 'grant',
}

describe('addEntry (ADR-0043 §4)', () => {
  it('click-add targets every supported signal the Blueprint routes', () => {
    const next = addEntry(doc(), typeEntry(), typeEntry().add.signals)
    // metrics is supported but not routed: no lane is invented.
    expect(Object.keys(next.lanes)).toEqual(['traces', 'logs'])
    expect(next.lanes['traces']).toEqual([
      'otlp-in',
      'batcher',
      'data-flow/gateway-exporter@2',
      'memory-limiter',
    ])
    expect(next.lanes['logs']?.at(-1)).toBe('memory-limiter')
    // One add, one local Component, shared by both lanes (ADR-0024 §3).
    expect(next.locals['memory-limiter']).toEqual({ class: 'processor', type: 'memory_limiter' })
  })

  it('a targeted add lands on the named lane only, at the tail', () => {
    const next = addEntry(doc(), sharedEntry, ['logs'])
    expect(next.lanes['traces']).toHaveLength(3)
    expect(next.lanes['logs']?.at(-1)).toBe('infosec/pii-redaction@3')
    expect(next.locals).toEqual(doc().locals)
  })

  it('fresh local names never collide', () => {
    const withOne = addEntry(doc(), typeEntry(), ['traces'])
    const withTwo = addEntry(withOne, typeEntry(), ['traces'])
    expect(withTwo.lanes['traces']?.slice(-2)).toEqual(['memory-limiter', 'memory-limiter-2'])
    expect(localNameFor(withTwo, 'memory_limiter')).toBe('memory-limiter-3')
  })
})

describe('removeEntry', () => {
  it('removes one reference by position, other lanes untouched', () => {
    const next = removeEntry(doc(), 'traces', 1)
    expect(next.lanes['traces']).toEqual(['otlp-in', 'data-flow/gateway-exporter@2'])
    expect(next.lanes['logs']).toHaveLength(3)
  })
})

describe('claims (REQ-031, ADR-0026 §4)', () => {
  it('stamps a version-stamped claim once, replacing a stale stamp', () => {
    const stamped = stampClaim(doc(), 'req-pii-redaction', 3)
    expect(stamped.satisfies).toEqual(['req-pii-redaction@3'])
    expect(stampClaim(stamped, 'req-pii-redaction', 4).satisfies).toEqual(['req-pii-redaction@4'])
  })

  it('the suggestion add places the component on the named lanes and claims', () => {
    const row: RequirementVerdict = {
      id: 'req-pii-redaction',
      version: 3,
      summary: 'PII is redacted before telemetry leaves the collector',
      remediation: 'Add infosec/pii-redaction to every lane carrying user data.',
      claimed: false,
      met: false,
      suggestion: { ref: 'infosec/pii-redaction', signals: ['traces', 'logs'] },
    }
    const next = addSuggestion(doc(), row, [typeEntry(), sharedEntry])
    expect(next.lanes['traces']?.at(-1)).toBe('infosec/pii-redaction@3')
    expect(next.lanes['logs']?.at(-1)).toBe('infosec/pii-redaction@3')
    expect(next.satisfies).toEqual(['req-pii-redaction@3'])
  })
})
