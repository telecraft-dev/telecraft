/**
 * Where a Step's card sits, as a pure function of four rectangles: the
 * anchor, the card, the viewport, and the margins. No DOM, so it tests
 * headless the way the canvas engine does (ADR-0044).
 *
 * The order of preference is below, above, right, left. Below first
 * because a Step points at something the reader is meant to look at, and a
 * card under it leaves the thing itself uncovered.
 */

export interface Box {
  top: number
  left: number
  width: number
  height: number
}

export type Placement = 'centre' | 'below' | 'above' | 'right' | 'left'

export interface Placed {
  placement: Placement
  top: number
  left: number
}

/** The gap between the card and the thing it points at. */
export const GAP = 14

/** How close to the viewport edge the card may sit. */
export const MARGIN = 12

/**
 * The card's size before it has been measured. The runner measures the
 * real card and re-places it on the next frame, so these only decide where
 * the first frame lands.
 */
export const CARD_WIDTH = 340
export const CARD_HEIGHT = 220

export function placeCard(
  anchor: Box | null,
  card: { width: number; height: number },
  viewport: { width: number; height: number },
): Placed {
  // No anchor is the welcome's shape (ADR-0051 §7), and also what a Step
  // whose anchor resolved nowhere degrades to (§4). The centred card is
  // laid out by CSS inside a modal dialog; these coordinates are what the
  // same card would occupy, so the function stays total and testable.
  if (anchor === null) {
    return {
      placement: 'centre',
      top: Math.max(MARGIN, Math.round((viewport.height - card.height) / 2)),
      left: Math.max(MARGIN, Math.round((viewport.width - card.width) / 2)),
    }
  }

  const below = anchor.top + anchor.height + GAP
  const above = anchor.top - GAP - card.height
  const right = anchor.left + anchor.width + GAP
  const left = anchor.left - GAP - card.width

  if (below + card.height <= viewport.height - MARGIN) {
    return { placement: 'below', top: below, left: alignX(anchor, card, viewport) }
  }
  if (above >= MARGIN) {
    return { placement: 'above', top: above, left: alignX(anchor, card, viewport) }
  }
  if (right + card.width <= viewport.width - MARGIN) {
    return { placement: 'right', top: alignY(anchor, card, viewport), left: right }
  }
  if (left >= MARGIN) {
    return { placement: 'left', top: alignY(anchor, card, viewport), left }
  }
  // Nothing fits beside it, which a small viewport and a large anchor can
  // always produce. The card stays on screen and overlaps rather than
  // hanging off an edge where it cannot be read.
  return {
    placement: 'below',
    top: clamp(below, MARGIN, viewport.height - card.height - MARGIN),
    left: alignX(anchor, card, viewport),
  }
}

/** Centred on the anchor horizontally, and never off either edge. */
function alignX(anchor: Box, card: { width: number }, viewport: { width: number }): number {
  const centred = anchor.left + anchor.width / 2 - card.width / 2
  return clamp(centred, MARGIN, viewport.width - card.width - MARGIN)
}

/** Centred on the anchor vertically, and never off either edge. */
function alignY(anchor: Box, card: { height: number }, viewport: { height: number }): number {
  const centred = anchor.top + anchor.height / 2 - card.height / 2
  return clamp(centred, MARGIN, viewport.height - card.height - MARGIN)
}

function clamp(value: number, low: number, high: number): number {
  // A viewport smaller than the card makes `high` the lower of the two;
  // the top-left margin wins, because a card read from its start is worth
  // more than one centred on nothing.
  return Math.round(Math.max(low, Math.min(value, Math.max(low, high))))
}
