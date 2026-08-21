import { describe, expect, it } from 'vitest'
import { CARD_HEIGHT, CARD_WIDTH, GAP, MARGIN, placeCard } from '../src/tours/position'
import { TOURS, isBareLanding, stepBody, stepIndex, tourById } from '../src/tours/registry'
import { anchorSelector } from '../src/tours/useAnchor'

// Tours are authored data rendered by one runner (ADR-0051 §1), so
// everything that decides what a reader sees is a pure function and tests
// headless, the way the canvas engine does. The Playwright suite covers
// what only a browser can answer: that every authored anchor resolves.

const card = { width: CARD_WIDTH, height: CARD_HEIGHT }
const viewport = { width: 1440, height: 900 }

describe('placeCard', () => {
  it('centres a Step with no anchor: the welcome, and the fallback (§4, §7)', () => {
    const placed = placeCard(null, card, viewport)
    expect(placed.placement).toBe('centre')
    expect(placed.left).toBe(Math.round((viewport.width - CARD_WIDTH) / 2))
  })

  it('prefers below, which leaves the thing it points at uncovered', () => {
    const anchor = { top: 100, left: 600, width: 200, height: 60 }
    const placed = placeCard(anchor, card, viewport)
    expect(placed.placement).toBe('below')
    expect(placed.top).toBe(anchor.top + anchor.height + GAP)
    // Centred on the anchor rather than aligned to it.
    expect(placed.left).toBe(Math.round(anchor.left + anchor.width / 2 - CARD_WIDTH / 2))
  })

  it('flips above when the foot of the viewport is too close', () => {
    const anchor = { top: 780, left: 600, width: 200, height: 60 }
    const placed = placeCard(anchor, card, viewport)
    expect(placed.placement).toBe('above')
    expect(placed.top).toBe(anchor.top - GAP - CARD_HEIGHT)
  })

  it('goes beside a full-height anchor, right first', () => {
    const anchor = { top: 0, left: 0, width: 300, height: 900 }
    const placed = placeCard(anchor, card, viewport)
    expect(placed.placement).toBe('right')
    expect(placed.left).toBe(anchor.left + anchor.width + GAP)
  })

  it('goes left when there is no room on the right', () => {
    const anchor = { top: 0, left: 1140, width: 300, height: 900 }
    const placed = placeCard(anchor, card, viewport)
    expect(placed.placement).toBe('left')
    expect(placed.left).toBe(anchor.left - GAP - CARD_WIDTH)
  })

  it('stays on screen when nothing fits, rather than hanging off an edge', () => {
    const small = { width: 360, height: 320 }
    const anchor = { top: 0, left: 0, width: 360, height: 320 }
    const placed = placeCard(anchor, card, small)
    expect(placed.top).toBeGreaterThanOrEqual(MARGIN)
    expect(placed.top + CARD_HEIGHT).toBeLessThanOrEqual(small.height)
    expect(placed.left).toBeGreaterThanOrEqual(0)
  })

  it('never pushes the card off the near edge, whatever the anchor', () => {
    const anchor = { top: 200, left: 4, width: 40, height: 30 }
    const placed = placeCard(anchor, card, viewport)
    expect(placed.left).toBeGreaterThanOrEqual(MARGIN)
  })
})

describe('stepIndex', () => {
  const tour = TOURS[0]!

  it('reads the URL as one-based, because a URL is read by people', () => {
    expect(stepIndex(tour, 1)).toBe(0)
    expect(stepIndex(tour, 3)).toBe(2)
  })

  it('clamps anything unreadable onto a real Step, never off the end (§4)', () => {
    expect(stepIndex(tour, undefined)).toBe(0)
    expect(stepIndex(tour, 0)).toBe(0)
    expect(stepIndex(tour, -7)).toBe(0)
    expect(stepIndex(tour, 2.7)).toBe(1)
    expect(stepIndex(tour, 9999)).toBe(tour.steps.length - 1)
    expect(stepIndex(tour, Number.NaN)).toBe(0)
  })
})

describe('tourById', () => {
  it('answers a Tour that exists', () => {
    expect(tourById('welcome')?.id).toBe('welcome')
  })

  it('answers nothing for an id nobody authored: no Tour, never an error', () => {
    expect(tourById('nonesuch')).toBeUndefined()
    expect(tourById(undefined)).toBeUndefined()
  })
})

describe('isBareLanding', () => {
  it('is a reader arriving at the landing Workspace with nothing in hand', () => {
    expect(isBareLanding('/estate', {})).toBe(true)
    expect(isBareLanding('/estate', { lens: 'staging' })).toBe(true)
    expect(isBareLanding('/estate', { object: undefined })).toBe(true)
  })

  it('is not somebody else’s link, which a Tour never lands on top of (§7)', () => {
    expect(isBareLanding('/estate', { object: 'tier:data-flow/gateway' })).toBe(false)
    expect(isBareLanding('/estate', { view: 'list', ungoverned: true })).toBe(false)
    expect(isBareLanding('/topology', {})).toBe(false)
    expect(isBareLanding('/compose', { claim: 'service.name=api' })).toBe(false)
  })
})

describe('the authored Tours', () => {
  it('name their Steps uniquely: a Step is cited by id in a report', () => {
    for (const tour of TOURS) {
      const ids = tour.steps.map((step) => step.id)
      expect(new Set(ids).size, `${tour.id} repeats a Step id`).toBe(ids.length)
    }
  })

  it('anchor only on names an anchor selector can build (§5)', () => {
    for (const tour of TOURS) {
      for (const step of tour.steps) {
        if (step.anchor === undefined) continue
        expect(anchorSelector(step.anchor), `${tour.id}/${step.id}`).toBe(
          `[data-tour="${step.anchor}"]`,
        )
      }
    }
  })

  it('travel only to a Workspace', () => {
    const workspaces = ['/estate', '/topology', '/compose', '/catalogue']
    for (const tour of TOURS) {
      for (const step of tour.steps) {
        if (step.to === undefined) continue
        expect(workspaces, `${tour.id}/${step.id}`).toContain(step.to)
      }
    }
  })

  it('read differently on the demo in at most one Step, and only there (§8)', () => {
    for (const tour of TOURS) {
      const demoSteps = tour.steps.filter((step) => step.demoBody !== undefined)
      expect(demoSteps.length, `${tour.id} branches on the demo more than once`).toBeLessThanOrEqual(
        1,
      )
      for (const step of tour.steps) {
        expect(stepBody(step, false)).toBe(step.body)
        expect(stepBody(step, true)).toBe(step.demoBody ?? step.body)
      }
    }
  })
})

describe('anchorSelector', () => {
  it('builds a selector for an authored identifier', () => {
    expect(anchorSelector('card-bands')).toBe('[data-tour="card-bands"]')
  })

  it('refuses anything else, which then degrades like a missing element', () => {
    expect(anchorSelector('"] , script')).toBeNull()
    expect(anchorSelector('Card')).toBeNull()
    expect(anchorSelector('')).toBeNull()
  })
})
