/**
 * Where the fitted graph sits vertically in the canvas.
 *
 * The bands read top-down — ungoverned arrivals above the environment rows
 * (ADR-0044 §2) — so a graph shorter than the canvas should hang from the
 * top. The canvas substrate's fit centres it instead, which on a small
 * estate leaves a wide empty band above the first band, saying nothing.
 *
 * This is the rendering shell's business, not the engine's (ADR-0045 §2):
 * the geometry is untouched, only where the viewport is pointed at it.
 */

/** The gap left above the first band. */
export const CANVAS_TOP_PADDING = 12

/**
 * Returns the viewport y that hangs the graph from the top of the canvas.
 * A graph that already fills the canvas keeps the fit's own placement:
 * pulling it up there would push its last band out of view, and the whole
 * point of fitting was to show all of it.
 */
export function topAnchoredViewportY(
  fitted: { y: number; zoom: number },
  contentHeight: number,
  canvasHeight: number,
  padding = CANVAS_TOP_PADDING,
): number {
  if (contentHeight * fitted.zoom >= canvasHeight - padding * 2) return fitted.y
  return padding
}
