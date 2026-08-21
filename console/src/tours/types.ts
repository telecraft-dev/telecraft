import type { WorkspacePath } from '../objectref'

/**
 * A Tour is authored data, never a surface (ADR-0051 §1). Everything in
 * this file is what an author writes; `TourRunner.tsx` is the only thing
 * that renders it, and adding a Tour builds nothing.
 *
 * The two rules that shape these types are worth knowing before you write
 * a Step. A Tour narrates the product and never drives it (§2): a Step may
 * carry a destination, because a destination is a URL and this console
 * already treats a URL as state, and it may point at an element — it may
 * not click, author, or invent. And an anchor that resolves nowhere
 * degrades to a centred Step rather than failing (§4), which is why
 * everything below except the prose is optional.
 */

/** Search params a Step's destination needs, beyond the Tour's own. */
export interface StepSearch {
  /** The Estate view (`shelf`, `rollup`, `list`) or the Topology view. */
  view?: string
  /** The Estate shelf scope. */
  scope?: 'team' | 'estate'
  /** The Catalogue & Governance view. */
  surface?: string
}

export interface Step {
  /**
   * Stable within its Tour, and part of the contract: a Step is cited by
   * position in a URL and by id in a bug report.
   */
  id: string
  title: string
  /**
   * The prose, at roughly sixty words (ADR-0051, consequences). A Step
   * that needs more than that is documentation, and the documentation
   * lives in `docs/`.
   */
  body: string
  /**
   * The demo's reading of the same Step (§8). Exactly one Step in a Tour
   * should ever need one: what a reader is looking at differs on the
   * public demo, and nothing else about the console does.
   */
  demoBody?: string
  /**
   * The element this Step points at, matched on `data-tour` (§5) — never
   * on a `data-testid`, which is a test's grip and may be renamed with the
   * test. Absent renders the Step centred, which is what a welcome is (§7).
   */
  anchor?: string
  /** The Workspace this Step is read in. Absent means "wherever the reader is". */
  to?: WorkspacePath
  /** What the destination needs on top of the lens and the Tour's own params. */
  search?: StepSearch
}

export interface Tour {
  id: string
  /** Named in the control that offers it, and in the Step card's heading. */
  title: string
  /** One line: what a reader gets for the minute it takes. */
  summary: string
  steps: readonly Step[]
}
