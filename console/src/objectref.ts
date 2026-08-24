import type { ObjectKind, ObjectRef } from './api/types'

// Object access is served by global jump-to-object search, never by
// object-first navigation (ADR-0042 §1): an object lands in the Workspace
// serving the activity that reads it, carried in the `object` search param
// so every selection is URL-addressable (ADR-0042 §3.5).

const KINDS: readonly ObjectKind[] = [
  'tier',
  'service',
  'blueprint',
  'component',
  'team',
  'entry',
  'rollout',
]

/** Encodes a ref for the `object` search param, for example `tier:data-flow/gateway`. */
export function formatObjectRef(ref: ObjectRef): string {
  return `${ref.kind}:${ref.id}`
}

/** Decodes an `object` search param; malformed values read as no selection. */
export function parseObjectRef(raw: string | undefined): ObjectRef | undefined {
  if (!raw) return undefined
  const sep = raw.indexOf(':')
  if (sep <= 0) return undefined
  const kind = raw.slice(0, sep) as ObjectKind
  const id = raw.slice(sep + 1)
  if (!KINDS.includes(kind) || id === '') return undefined
  return { kind, id }
}

export type WorkspacePath = '/' | '/estate' | '/topology' | '/compose' | '/catalogue'

/**
 * The Workspace an object kind deep-links into: Tiers summon their card on
 * the shelf, Services trace their Paths on the canvas (ADR-0044 §4),
 * Rollouts summon their panel on the Topology rollout ledger, Blueprints
 * open in Compose, Components and Catalogue entries in Catalogue &
 * Governance, and a team lands on its shelf section.
 */
export function workspaceFor(kind: ObjectKind): WorkspacePath {
  switch (kind) {
    case 'tier':
      return '/estate'
    case 'team':
      return '/estate'
    case 'service':
      return '/topology'
    case 'rollout':
      return '/topology'
    case 'blueprint':
      return '/compose'
    case 'component':
      return '/catalogue'
    case 'entry':
      return '/catalogue'
  }
}

export interface DeepLink {
  to: WorkspacePath
  object: string
}

export function deepLinkFor(ref: ObjectRef): DeepLink {
  return { to: workspaceFor(ref.kind), object: formatObjectRef(ref) }
}
