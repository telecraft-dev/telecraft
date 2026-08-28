/**
 * The brand mark's construction (docs/branding/identity.md, ADR-0047 §6).
 *
 * The geometry is the pack's bare mark on its 18x16 grid
 * (docs/branding/pack/telecraft-mark-on-dark.svg): a datum in the brand
 * amber, and three reading bands falling in length and in tone. The pack
 * draws one file per ground, which a console that resolves its theme at
 * runtime cannot use, so each fill here names the token the pack's value
 * was taken from and the one drawing follows the reader's theme.
 *
 * Kept apart from `BrandMark.tsx` the way `marks.ts` is kept apart from
 * `Mark.tsx`: this half is measurable, and `tests/brand-mark.test.ts`
 * measures it against the pack file. That test is what stands in for the
 * pack's own guarantee, which is that no file in it is drawn twice.
 */

/** One rectangle of the mark, with the token that fills it. */
export type BrandRect = {
  x: number
  y: number
  width: number
  height: number
  rx: number
  token: string
}

/** The bare mark's grid: the datum's top-left corner is the origin. */
export const MARK_GRID = { width: 18, height: 16 }

export const MARK_VIEWBOX = `0 0 ${MARK_GRID.width} ${MARK_GRID.height}`

/** The datum first, then the bands, ink to faint. Pack order throughout. */
export const MARK_RECTS: BrandRect[] = [
  { x: 0, y: 0, width: 2, height: 16, rx: 1, token: '--brand' },
  { x: 5, y: 0, width: 13, height: 3, rx: 1.5, token: '--colour-text' },
  { x: 5, y: 6.5, width: 10, height: 3, rx: 1.5, token: '--colour-text-muted' },
  { x: 5, y: 13, width: 7, height: 3, rx: 1.5, token: '--colour-text-faint' },
]

/**
 * The mark is never drawn below this, in pixels: under it the datum blurs
 * into the first band (docs/branding/pack/README.md, "Minimum sizes").
 */
export const MARK_MIN_HEIGHT = 12

/**
 * The height it is drawn at in the chrome. Above the floor, and close to
 * the ascender height the pack's horizontal lockup sets the mark at beside
 * the word.
 */
export const MARK_HEIGHT = 14
