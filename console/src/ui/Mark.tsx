import type { BandState, Severity } from '../api/types'
import { MARK_TITLE, markFor, type MarkName } from './marks'

export { markFor, stateLabel, type MarkName } from './marks'

/**
 * State marks (ADR-0047 §6, §7).
 *
 * Drawn here rather than taken from a pack, because ADR-0041 makes glyphs a
 * mapping from states: they are product vocabulary, and no pack draws
 * `pending_settle` the way this product means it. Unicode is not used for
 * any of them — measured in-browser, Atkinson Hyperlegible contains U+2713
 * and U+25B2 but not U+2717, so a verdict's tick and its cross would render
 * from different typefaces, at different weights, chosen by the reader's
 * machine.
 *
 * Seven marks, one per band state, and the four honest neutrals each get
 * their own (ADR-0047 §7): `not_applicable`, `unknown`, `pending_settle`
 * and `stale_demoted` are distinct states in the ADR-0041 contract and
 * collapsed to one glyph before this pass. That changed the state-to-glyph
 * mapping and its tests, not the contract.
 *
 * All of them share one 16-unit grid, 1.75 stroke, round caps and joins,
 * and inherit `currentColor`. Read any card in greyscale and the marks
 * still tell the states apart, which is what lets hue stay decorative
 * (ADR-0041 §2).
 */

/** Each mark's geometry, on the shared grid. */
function shape(name: MarkName) {
  switch (name) {
    case 'ok':
      // A tick: the only mark that reads as an assertion rather than a shape.
      return <path d="M3.25 8.5 6.5 11.75 12.75 4.5" />
    case 'advisory':
      // Solid, because a filled triangle survives being 12px wide.
      return <path d="M8 3 14 13H2Z" fill="currentColor" />
    case 'violation':
      return <path d="M4.25 4.25 11.75 11.75M11.75 4.25 4.25 11.75" />
    case 'not_applicable':
      // A plain rule: nothing to judge, so nothing is drawn but the datum.
      return <path d="M3.25 8H12.75" />
    case 'unknown':
      // A broken outline: we should know this and do not. The gaps are the
      // point — a closed ring would read as a settled state.
      return <circle cx="8" cy="8" r="5" strokeDasharray="3.1 2.7" />
    case 'pending_settle':
      // A clock: the ADR-0038 settling window has not closed yet.
      return (
        <>
          <circle cx="8" cy="8" r="5.25" />
          <path d="M8 5.25V8l2.25 1.5" />
        </>
      )
    case 'stale_demoted':
      // A closed outline and a chevron down: judged once, then demoted.
      return (
        <>
          <circle cx="8" cy="8" r="5.25" />
          <path d="M5.75 6.75 8 9.25l2.25-2.5" />
        </>
      )
  }
}

/**
 * A mark on its own is decoration: it carries `aria-hidden` and the
 * surface beside it carries the word. Pass `labelled` only where no word
 * accompanies it, which should be nowhere on a data surface.
 */
export function Mark({
  name,
  size = 16,
  labelled = false,
  className,
}: {
  name: MarkName
  size?: number
  labelled?: boolean
  className?: string
}) {
  return (
    <svg
      className={className ? `mark ${className}` : 'mark'}
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      role={labelled ? 'img' : undefined}
      aria-label={labelled ? MARK_TITLE[name] : undefined}
      aria-hidden={labelled ? undefined : true}
      data-mark={name}
    >
      {shape(name)}
    </svg>
  )
}

/**
 * The band mark: the state's mark, tinted by the severity that produced it.
 * The tint is the only thing hue does here.
 */
export function BandMark({ state, severity }: { state: BandState; severity: Severity }) {
  return <Mark name={markFor(state, severity)} />
}
