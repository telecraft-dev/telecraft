import { describe, expect, it } from 'vitest'
import { stabilityChips } from '../src/surfaces/catalogue/stability'

// The browse table's chip collapse: an entry whose signals all carry one
// level reads as a single "all signals" chip, mixed levels keep a chip per
// signal, and a single-signal entry keeps its named chip.
//
// A chip that names one lane carries that lane, and the collapsed chip
// names none and carries none: it must not take a lane colour, because a
// signal colour never appears without its lane name (ADR-0047 §5).

describe('stability chips', () => {
  it('collapses uniform levels to one "all signals" chip', () => {
    expect(stabilityChips({ traces: 'beta', metrics: 'beta', logs: 'beta' })).toEqual([
      { key: 'all-signals', label: 'all signals: beta', level: 'beta' },
    ])
  })

  it('collapses a two-signal entry when both carry one level', () => {
    expect(stabilityChips({ traces: 'deprecated', metrics: 'deprecated' })).toEqual([
      { key: 'all-signals', label: 'all signals: deprecated', level: 'deprecated' },
    ])
  })

  it('keeps a chip per signal when levels are mixed, sorted by signal', () => {
    expect(stabilityChips({ traces: 'stable', metrics: 'stable', logs: 'beta' })).toEqual([
      { key: 'logs', label: 'logs: beta', level: 'beta', signal: 'logs' },
      { key: 'metrics', label: 'metrics: stable', level: 'stable', signal: 'metrics' },
      { key: 'traces', label: 'traces: stable', level: 'stable', signal: 'traces' },
    ])
  })

  it('keeps the named chip on a single-signal entry', () => {
    expect(stabilityChips({ traces_to_metrics: 'alpha' })).toEqual([
      {
        key: 'traces_to_metrics',
        label: 'traces_to_metrics: alpha',
        level: 'alpha',
        signal: 'traces_to_metrics',
      },
    ])
  })
})
