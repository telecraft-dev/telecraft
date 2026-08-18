import type { CollectorRow, Selector } from '../api/types'

// The claim flow's selector suggestion (ADR-0042 §6): the one place the
// console synthesises policy from observed state, bounded to draft input
// reviewed under the impact-preview machinery. Herd-first: the suggestion
// is computed over a selected population, generalising over the identity
// attributes its members share — and never enumerating instance ids. The
// fixture backend enforces the same rule server-side (tools/claims.mjs);
// this module is what makes enumeration inexpressible in the UI.

/**
 * Attribute keys that name one instance, never a population. The
 * generaliser drops them before suggesting, so a selector built here
 * cannot enumerate — and the server refuses them independently.
 */
export const INSTANCE_KEYS: ReadonlySet<string> = new Set([
  'service.instance.id',
  'host.name',
  'host.id',
  'k8s.pod.name',
  'k8s.pod.uid',
])

/**
 * The suggested selector: every identity attribute the whole herd agrees
 * on, instance-naming keys dropped. The user constrains this by removing
 * pairs — widening, never enumerating (ADR-0042 §6).
 */
export function suggestSelector(herd: CollectorRow[]): Selector {
  const [first, ...rest] = herd
  if (!first) return {}
  const out: Selector = {}
  for (const key of Object.keys(first.attributes ?? {}).sort()) {
    if (INSTANCE_KEYS.has(key)) continue
    const value = first.attributes?.[key]
    if (value !== undefined && rest.every((row) => row.attributes?.[key] === value)) {
      out[key] = value
    }
  }
  return out
}

/** Renders a selector as `key=value` pairs, comma-joined, keys sorted. */
export function formatSelector(selector: Selector): string {
  return Object.keys(selector)
    .sort()
    .map((key) => `${key}=${selector[key]}`)
    .join(',')
}

/** Parses the `formatSelector` shape back; malformed pairs are dropped. */
export function parseSelector(serialised: string | undefined): Selector {
  const out: Selector = {}
  for (const part of (serialised ?? '').split(',')) {
    const cut = part.indexOf('=')
    if (cut > 0) out[part.slice(0, cut).trim()] = part.slice(cut + 1).trim()
  }
  return out
}
