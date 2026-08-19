import type { BandState, Severity } from '../api/types'

/**
 * The state-to-mark mapping (ADR-0041, ADR-0047 §6 and §7).
 *
 * Kept apart from the drawing in `Mark.tsx` because this half is the
 * product's vocabulary and is worth testing on its own: states are the
 * contract, marks are how a reader sees them, and hue appears in neither.
 *
 * Seven marks, one per state, and the four honest neutrals each get their
 * own. Before ADR-0047 they collapsed to a single glyph, which told a
 * reader that `unknown` (we should know this and do not) and
 * `not_applicable` (there is nothing here to judge) were the same
 * situation. They are not, and the contract always said so.
 */

export type MarkName =
  | 'ok'
  | 'advisory'
  | 'violation'
  | 'not_applicable'
  | 'unknown'
  | 'pending_settle'
  | 'stale_demoted'

/** The band state and its severity decide the mark; hue never does. */
export function markFor(state: BandState, severity: Severity): MarkName {
  if (state === 'finding') return severity === 'violation' ? 'violation' : 'advisory'
  return state
}

/** The word beside every mark. A mark never travels without one. */
export function stateLabel(state: BandState): string {
  switch (state) {
    case 'ok':
      return 'ok'
    case 'finding':
      return 'finding'
    case 'not_applicable':
      return 'not applicable'
    case 'unknown':
      return 'unknown'
    case 'pending_settle':
      return 'pending settle'
    case 'stale_demoted':
      return 'stale, demoted'
  }
}

/** What a mark means, for the rare surface where no word sits beside it. */
export const MARK_TITLE: Record<MarkName, string> = {
  ok: 'ok',
  advisory: 'advisory finding',
  violation: 'violation',
  not_applicable: 'not applicable',
  unknown: 'unknown',
  pending_settle: 'pending settle',
  stale_demoted: 'stale, demoted',
}
