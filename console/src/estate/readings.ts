import type {
  ChurnReading,
  FreshnessReading,
  Reading,
  ShapeReading,
  SignalName,
  SignalRow,
  VolumeReading,
} from '../api/types'

// How the per-signal matrix reads (ADR-0040, ADR-0041 §2). Pure
// formatting over contract fields: every honest state the contract can
// carry gets its own rendering, because a reading nobody took and a
// reading of nothing are different situations and a card that showed both
// as `0` would be lying about one of them.

/** The lanes, in the fixed order the matrix draws them. */
export const SIGNAL_ORDER: readonly SignalName[] = ['logs', 'metrics', 'traces']

/** What a reading is doing, before anything is formatted from it. */
export type ReadingState = 'known' | 'silent' | 'unknown'

export function readingState(reading: Reading & { silent?: boolean }): ReadingState {
  if (!reading.known) return 'unknown'
  return reading.silent ? 'silent' : 'known'
}

/**
 * Whether a row has readings to render. A lane the Tier's artefact never
 * wired has none (#98): the counters behind them would read `in 0 / out
 * 0`, which is the same shape a pipeline that has broken reads, and the
 * two mean opposite things. An unknown lane still renders its readings:
 * not knowing whether a lane exists is not knowing that it does not.
 */
export function laneReads(row: SignalRow): row is SignalRow & {
  volume: VolumeReading
  freshness: FreshnessReading
  shape: ShapeReading
} {
  return row.lane !== 'not_applicable'
}

/**
 * Whether a wired lane has produced any reading at all yet. A card whose
 * every lane answers false is waiting for its first readings, and nine
 * cells of "no reading" say that worse than one line does (ADR-0041 §2
 * still holds: the one line is words, never a fabricated number).
 */
export function laneUnread(row: SignalRow): boolean {
  if (!laneReads(row)) return false
  // A wired lane's readings can be absent outright, not merely unknown: a
  // never_seen Tier's payload carries rows with no readings at all,
  // because no collector has ever taken one (ADR-0060 §3). An absent
  // reading is as unread as an unknown one, so the card still collapses
  // to the one quiet line and keeps the card-grain height (ADR-0042 §2).
  const { volume, freshness, shape } = row as SignalRow
  return (
    volume?.known !== true &&
    (freshness === undefined || readingState(freshness) === 'unknown') &&
    shape?.known !== true
  )
}

/** What a row says in place of the readings it has no lane to take. */
export const NO_LANE = 'no lane on this Tier'

/* Why, at length, for the cell's own title. A lane the configuration never
   wired has nothing to meter, which is why the cell shows no reading. */
export const NO_LANE_TITLE = 'The configuration has no pipeline for this signal.'

/** The words a card shows where a number would be a fabrication. */
export const NO_READING = 'no reading'

/** The one line a card shows while every wired lane awaits its first reading. */
export const NO_READINGS_YET = 'no readings yet'

/** Item counts, short enough for a card row. Items are the unit (ADR-0040 §2). */
export function formatItems(items: number): string {
  const abs = Math.abs(items)
  if (abs >= 1_000_000_000) return `${trim(items / 1_000_000_000)}B`
  if (abs >= 1_000_000) return `${trim(items / 1_000_000)}M`
  if (abs >= 1_000) return `${trim(items / 1_000)}k`
  return `${items}`
}

function trim(value: number): string {
  const rounded = value >= 100 ? Math.round(value) : Math.round(value * 10) / 10
  return `${rounded}`
}

/** An age, coarsest unit first: the reader wants "3h", not 10,842 seconds. */
export function formatAge(seconds: number): string {
  if (seconds < 60) return `${Math.max(0, Math.round(seconds))}s`
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  if (seconds < 86_400) return `${Math.round(seconds / 3600)}h`
  return `${Math.round(seconds / 86_400)}d`
}

/**
 * The volume cell: items in, items out, and the reduction between them.
 * The word is "reduction" and never anything that implies fault: the
 * meter presents the delta and passes no judgement (ADR-0040 §3).
 */
export function formatVolume(volume: VolumeReading): string {
  if (!volume.known) return NO_READING
  return `${formatItems(volume.in)} → ${formatItems(volume.out)}`
}

/** The reduction, as a share of items in; absent when nothing came in. */
export function formatReduction(volume: VolumeReading): string | undefined {
  if (!volume.known || volume.in <= 0) return undefined
  const share = Math.round((volume.reduction / volume.in) * 100)
  if (share === 0) return undefined
  return share > 0 ? `${share}% reduction` : `${Math.abs(share)}% fan-out`
}

/** Every error-rate reading with a count: the only reds the meter sources. */
export function errorReadings(volume: VolumeReading): { label: string; items: number }[] {
  if (!volume.known) return []
  return [
    { label: 'refused', items: volume.refused },
    { label: 'send failed', items: volume.sendFailed },
    { label: 'enqueue failed', items: volume.enqueueFailed },
  ].filter((reading) => reading.items > 0)
}

/**
 * The freshness cell. A known-empty window says so in words: "silent" is
 * a reading, where "no reading" would claim we could not see.
 */
export function formatFreshness(freshness: FreshnessReading): string {
  switch (readingState(freshness)) {
    case 'unknown':
      return NO_READING
    case 'silent':
      return 'silent'
    default:
      return freshness.ageSeconds === undefined ? NO_READING : formatAge(freshness.ageSeconds)
  }
}

/** The shape cell: how many required attributes the landed telemetry misses. */
export function formatShape(shape: ShapeReading): string {
  if (!shape.known) return NO_READING
  if (shape.required === 0) return 'none required'
  if (shape.missing === 0) return `${shape.required} present`
  return `${shape.missing} of ${shape.required} missing`
}

/** The churn line: incarnations in the window, presented and not judged. */
export function formatChurn(churn: ChurnReading): string {
  if (!churn.known) return NO_READING
  return `${churn.incarnations} incarnation${churn.incarnations === 1 ? '' : 's'}`
}

/**
 * The last-known-plus-age line every unknown reading gets instead of a
 * number: when the reading was taken, and why it says nothing (ADR-0041
 * §2, ADR-0008).
 */
export function readingTitle(reading: Reading, now: Date = new Date()): string {
  const takenAt = new Date(reading.asOf)
  const age = Number.isNaN(takenAt.getTime())
    ? undefined
    : formatAge((now.getTime() - takenAt.getTime()) / 1000)
  const when = age ? `read ${age} ago` : `read at ${reading.asOf}`
  return reading.known ? when : `${when}: ${reading.cause ?? 'no reading'}`
}
