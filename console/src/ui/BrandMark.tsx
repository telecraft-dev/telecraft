import { MARK_GRID, MARK_HEIGHT, MARK_RECTS, MARK_VIEWBOX } from './brand'

/**
 * The mark, drawn beside the word it supports rather than in place of it
 * (docs/branding/identity.md). The word carries the name, so the drawing is
 * hidden from a screen reader: announcing "Telecraft" twice in the first
 * two stops of the page is noise.
 *
 * The fills are tokens, so the mark is themed by the same two blocks
 * everything else on the surface is.
 */
export function BrandMark({ height = MARK_HEIGHT }: { height?: number }) {
  return (
    <svg
      className="brand-mark"
      width={(height * MARK_GRID.width) / MARK_GRID.height}
      height={height}
      viewBox={MARK_VIEWBOX}
      aria-hidden={true}
      data-testid="brand-mark"
    >
      {MARK_RECTS.map((rect) => (
        <rect
          key={rect.token}
          x={rect.x}
          y={rect.y}
          width={rect.width}
          height={rect.height}
          rx={rect.rx}
          fill={`var(${rect.token})`}
        />
      ))}
    </svg>
  )
}
