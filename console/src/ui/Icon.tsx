import { Command, Compass, Monitor, Moon, Search, Sun, X, type LucideIcon } from 'lucide-react'

/**
 * Utility icons (ADR-0047 §6, tier three): Lucide (ISC), self-hosted with
 * the rest of the bundle and tree-shaken to the named few below. They carry
 * no product meaning — that is the whole reason they can come from a pack,
 * where state marks in `Mark.tsx` cannot.
 *
 * Lucide's default is a 2px stroke on a 24px grid, which reads thin beside
 * Atkinson. Everything here is set to the same 16px grid and 1.75 stroke
 * the drawn marks use, so a pack icon and a drawn mark sit on one line
 * without one of them looking borrowed.
 *
 * Importing through this module rather than from `lucide-react` directly is
 * what keeps that true, and keeps the icon list short enough to review.
 */

// Every entry here is used. An icon nobody renders is dead weight the
// bundler happens to remove for us, and a list nobody can review.
const ICONS = {
  close: X,
  command: Command,
  search: Search,
  tour: Compass,
  'theme-system': Monitor,
  'theme-light': Sun,
  'theme-dark': Moon,
} satisfies Record<string, LucideIcon>

export type IconName = keyof typeof ICONS

/**
 * An icon is decoration unless it is the control's only content, in which
 * case the control carries the accessible name — not the icon.
 */
export function Icon({
  name,
  size = 16,
  className,
}: {
  name: IconName
  size?: number
  className?: string
}) {
  const Glyph = ICONS[name]
  return (
    <Glyph
      className={className ? `icon ${className}` : 'icon'}
      size={size}
      strokeWidth={1.75}
      absoluteStrokeWidth
      aria-hidden="true"
    />
  )
}
