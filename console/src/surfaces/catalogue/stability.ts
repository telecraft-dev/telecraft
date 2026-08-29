import type { StabilityLevel } from '../../api/types'

/**
 * What each upstream stability level means, for the hover on a stability
 * chip. The level names are upstream vocabulary and stay as written
 * (ADR-0001); the reader who has not lived in the collector's release
 * notes still deserves the meaning without leaving the page.
 */
export const STABILITY_TITLE: Record<StabilityLevel, string> = {
  development: 'Upstream is still building this. Anything can change.',
  alpha: 'Early upstream support. Configuration and behaviour can change.',
  beta: 'Upstream support that is settling. Small changes can still arrive.',
  stable: 'Settled upstream support.',
  deprecated: 'Upstream is retiring this.',
  unmaintained: 'Upstream no longer maintains this.',
}

/** One stability chip in a browse row. */
export interface StabilityChip {
  key: string
  label: string
  level: StabilityLevel
  /**
   * The one signal the chip speaks for, where it speaks for one. The
   * collapsed "all signals" chip names no lane and so carries none: it
   * must not take a lane colour (ADR-0047 §5).
   */
  signal?: string
}

/**
 * The chips a browse row shows for an entry's per-signal stability. An
 * entry whose signals all carry one level reads as a single "all signals"
 * chip: repeating the level per signal says nothing the one chip does not.
 * Mixed levels keep a chip per signal, and a single-signal entry keeps its
 * named chip, because "all signals" would overclaim a vocabulary of one.
 */
export function stabilityChips(stability: Record<string, StabilityLevel>): StabilityChip[] {
  const signals = Object.entries(stability).sort(([a], [b]) => a.localeCompare(b))
  const levels = [...new Set(signals.map(([, level]) => level))]
  const [level] = levels
  if (signals.length > 1 && levels.length === 1 && level !== undefined) {
    return [{ key: 'all-signals', label: `all signals: ${level}`, level }]
  }
  return signals.map(([signal, level]) => ({
    key: signal,
    label: `${signal}: ${level}`,
    level,
    signal,
  }))
}
