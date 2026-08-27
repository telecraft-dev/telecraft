/**
 * The two number-to-words helpers every surface shares. Each existed two
 * or three times as a local copy before this module, and the copies had
 * already drifted: the shelf said `1 findings` while Home pluralised, and
 * the shelf said `21614 matched` while the canvas said `21,614`.
 */

/**
 * `1 Tiers carry a finding` reads as a defect in the page rather than a
 * count of one, and every count in the console can legitimately be one.
 */
export function count(n: number, one: string, many = `${one}s`): string {
  return `${formatCount(n)} ${n === 1 ? one : many}`
}

/** Fixed-locale grouping so a large population reads at a glance. */
export function formatCount(n: number): string {
  return n.toLocaleString('en-GB')
}
