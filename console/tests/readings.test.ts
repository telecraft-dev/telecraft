import { describe, expect, it } from 'vitest'
import type { FreshnessReading, ShapeReading, SignalRow, VolumeReading } from '../src/api/types'
import {
  errorReadings,
  formatAge,
  formatFreshness,
  formatItems,
  formatReduction,
  formatShape,
  formatVolume,
  laneReads,
  readingState,
  readingTitle,
} from '../src/estate/readings'

const asOf = '2026-08-18T11:59:00Z'
const now = new Date('2026-08-18T12:00:00Z')

function volume(over: Partial<VolumeReading> = {}): VolumeReading {
  return {
    known: true,
    asOf,
    in: 0,
    out: 0,
    reduction: 0,
    refused: 0,
    sendFailed: 0,
    enqueueFailed: 0,
    ...over,
  }
}

describe('reading formatting', () => {
  it('counts items, which are the unit', () => {
    expect(formatItems(940)).toBe('940')
    expect(formatItems(12_400)).toBe('12.4k')
    expect(formatItems(1_000_000)).toBe('1M')
    expect(formatItems(12_000_000_000)).toBe('12B')
  })

  it('ages in the coarsest unit that still says something', () => {
    expect(formatAge(42)).toBe('42s')
    expect(formatAge(600)).toBe('10m')
    expect(formatAge(7_200)).toBe('2h')
    expect(formatAge(259_200)).toBe('3d')
  })

  // ADR-0040 §3: a filter dropping ninety per cent is doing its job. The
  // card says "reduction" and nothing that implies fault.
  it('presents reduction as a share, and never as a fault', () => {
    const filtered = volume({ in: 1_000_000, out: 100_000, reduction: 900_000 })
    expect(formatVolume(filtered)).toBe('1M → 100k')
    expect(formatReduction(filtered)).toBe('90% reduction')
    expect(errorReadings(filtered)).toEqual([])
  })

  it('names a negative reduction fan-out, because a connector honestly sends more', () => {
    const fanned = volume({ in: 100, out: 200, reduction: -100 })
    expect(formatReduction(fanned)).toBe('100% fan-out')
  })

  it('shows no reduction line when nothing was reduced or nothing came in', () => {
    expect(formatReduction(volume({ in: 500, out: 500 }))).toBeUndefined()
    expect(formatReduction(volume({ in: 0, out: 0 }))).toBeUndefined()
  })

  it('surfaces the error-rate readings, the only reds the meter sources', () => {
    const erroring = volume({ in: 48_000, out: 47_900, reduction: 100, refused: 100, sendFailed: 3 })
    expect(errorReadings(erroring)).toEqual([
      { label: 'refused', items: 100 },
      { label: 'send failed', items: 3 },
    ])
  })

  it('never puts a number where nobody took a reading', () => {
    const unread = volume({ known: false, cause: 'backend unreachable' })
    expect(formatVolume(unread)).toBe('no reading')
    expect(formatReduction(unread)).toBeUndefined()
    expect(errorReadings(unread)).toEqual([])
    expect(readingTitle(unread, now)).toBe('read 1m ago: backend unreachable')
  })

  // ADR-0008: a known-empty window and an unreadable one are different
  // situations, and stay different on the card.
  it('separates a silent lane from an unreadable one', () => {
    const silent: FreshnessReading = { known: true, asOf, silent: true }
    const unreadable: FreshnessReading = { known: false, asOf, cause: 'no index matches' }
    const fresh: FreshnessReading = { known: true, asOf, ageSeconds: 90 }

    expect(readingState(silent)).toBe('silent')
    expect(readingState(unreadable)).toBe('unknown')
    expect(readingState(fresh)).toBe('known')

    expect(formatFreshness(silent)).toBe('silent')
    expect(formatFreshness(unreadable)).toBe('no reading')
    expect(formatFreshness(fresh)).toBe('2m')
  })

  it('reads a lane shape, including the lane nobody demanded anything of', () => {
    const clean: ShapeReading = { known: true, asOf, required: 4, missing: 0 }
    const missing: ShapeReading = { known: true, asOf, required: 4, missing: 1 }
    const unasked: ShapeReading = { known: true, asOf, required: 0, missing: 0 }
    const unread: ShapeReading = { known: false, asOf, cause: 'no landed telemetry', required: 0, missing: 0 }

    expect(formatShape(clean)).toBe('4 present')
    expect(formatShape(missing)).toBe('1 of 4 missing')
    expect(formatShape(unasked)).toBe('none required')
    expect(formatShape(unread)).toBe('no reading')
  })
})

// #98: the three situations that all meter `in 0 / out 0` and mean
// different things. Which one a row is in is not derivable from the
// numbers, which is why the lane state carries it.
describe('the lane state', () => {
  const row = (over: Partial<SignalRow>): SignalRow => ({
    signal: 'metrics',
    lane: 'present',
    volume: volume(),
    freshness: { known: true, asOf, silent: true },
    shape: { known: true, asOf, required: 0, missing: 0 },
    ...over,
  })

  it('keeps the readings of a lane that exists and carried nothing', () => {
    const stopped = row({ lane: 'present' })
    expect(laneReads(stopped)).toBe(true)
    // The zero is a reading, and on a wired lane it is a finding.
    expect(formatVolume(stopped.volume!)).toBe('0 → 0')
  })

  it('drops the readings of a lane the artefact never wired', () => {
    expect(laneReads(row({ lane: 'not_applicable', volume: undefined }))).toBe(false)
  })

  // Not knowing whether a lane exists is not knowing that it does not
  // (ADR-0008): the readings still render, as whatever they are.
  it('keeps the readings of a lane nobody could look for', () => {
    expect(laneReads(row({ lane: 'unknown' }))).toBe(true)
  })
})
