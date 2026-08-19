import { describe, expect, it } from 'vitest'
import { CANVAS_TOP_PADDING, topAnchoredViewportY } from '../src/surfaces/topology/viewport'

// The bands read top-down (ADR-0044 §2), so a graph shorter than the
// canvas hangs from the top rather than floating in the middle of it.

describe('topAnchoredViewportY', () => {
  it('pulls a short graph up to the top padding', () => {
    // A 400px-tall graph fitted at 1× into an 900px canvas: the fit
    // centred it 250px down, saying nothing with the space above.
    expect(topAnchoredViewportY({ y: 250, zoom: 1 }, 400, 900)).toBe(CANVAS_TOP_PADDING)
  })

  it('accounts for the fitted zoom, not the raw geometry', () => {
    // Half scale halves the drawn height, so a graph that would overflow
    // at 1× still hangs from the top.
    expect(topAnchoredViewportY({ y: 180, zoom: 0.5 }, 1200, 900)).toBe(CANVAS_TOP_PADDING)
  })

  it('leaves a graph that fills the canvas exactly where the fit put it', () => {
    // Pulling this one up would push its last band out of view, and
    // showing all of it is what the fit was for.
    expect(topAnchoredViewportY({ y: 40, zoom: 1 }, 900, 900)).toBe(40)
  })
})
