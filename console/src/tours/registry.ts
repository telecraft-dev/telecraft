import type { Step, Tour } from './types'
import { welcome } from './welcome'

/**
 * Every Tour the console knows, in one list (ADR-0051 §1). A Tour is
 * added here and nowhere else; nothing renders from anything but this.
 *
 * The list is a place Tours can accumulate carelessly, in the way ADR-0048
 * warned the primitive layer could. A Tour is a decision about what a
 * reader most needs next, not a changelog for a release.
 */
export const TOURS: readonly Tour[] = [welcome]

/** The Tour offered from the chrome, and the one that opens itself once (§7). */
export const WELCOME_TOUR = welcome.id

/** An unknown id is no Tour, never an error: teaching is never load-bearing (§4). */
export function tourById(id: string | undefined): Tour | undefined {
  if (id === undefined) return undefined
  return TOURS.find((tour) => tour.id === id)
}

/**
 * The Step a `step` search param names, clamped into the Tour.
 *
 * Steps are one-based in the URL, because that is what the card says out
 * loud ("Step 3 of 8") and a URL is read by people. Anything unreadable,
 * fractional, or past either end lands on the nearest real Step rather
 * than ending the Tour (§4).
 */
export function stepIndex(tour: Tour, raw: number | undefined): number {
  if (raw === undefined || !Number.isFinite(raw)) return 0
  const zeroBased = Math.trunc(raw) - 1
  return Math.min(Math.max(zeroBased, 0), tour.steps.length - 1)
}

/** The prose this Step reads with, which the demo changes for exactly one Step (§8). */
export function stepBody(step: Step, demo: boolean): string {
  return demo && step.demoBody !== undefined ? step.demoBody : step.body
}

/** The Workspace a reader who typed nothing lands on: Home (ADR-0056 §1). */
const LANDING = '/'

/**
 * Whether this URL is somebody's own arrival rather than somebody else's
 * link, which is the only place the welcome Tour opens itself (ADR-0051
 * §7). A shared URL carries the sender's context (a selected object, a
 * filtered list, a claim in progress), and a Tour that opened on top of it
 * would bury the thing the link was sent for.
 *
 * The lens is the one param that does not count as context: it is a
 * persisted preference that reaches the URL on its own (ADR-0042 §4).
 */
export function isBareLanding(pathname: string, search: Record<string, unknown>): boolean {
  // Both spellings of the route, which collapse to one now that the landing
  // is `/`: a trailing slash on the root is the root.
  const normalised = pathname.endsWith('/') && pathname.length > 1 ? pathname.slice(0, -1) : pathname
  if (normalised !== LANDING) return false
  return Object.entries(search).every(([key, value]) => key === 'lens' || value === undefined)
}
