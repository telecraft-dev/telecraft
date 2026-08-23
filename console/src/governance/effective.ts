import type {
  CatalogueEntry,
  GovernancePayload,
  GrantDoc,
  TeamNode,
} from '../api/types'

// The effective palette, derived for presentation from the authored policy
// the API serves (ADR-0021), the same derived-presentation pattern as the
// estate roll-up. The Go evaluator (internal/allowlist) is the judgement of
// record at render; this mirror exists so "why is this allowed?" resolves
// in the console with total provenance: everything a team may use traces to
// the lists surviving intersection, the default posture, or a named Grant.
//
// The composition rules are the ratified ADR-0021 semantics: narrowing-only
// inheritance (each declared list on the chain intersects), with Grants as
// the one widening mechanism. A Grant overrides its own target's declared
// list (union after intersection), applies to the target's subtree, and is
// narrowed back out by a descendant's list like anything else.

/** Why a component is in, or out of, an effective palette. */
export type PaletteOrigin = 'default-allow' | 'allow-list' | 'grant'

export interface PaletteRow {
  entry: CatalogueEntry
  allowed: boolean
  /** Set when allowed: the audit-chain origin. */
  origin?: PaletteOrigin
  /** Grant provenance, set only when origin is `grant`. */
  grant?: GrantDoc
  /** The granting authority: the Grant owner's team. */
  grantedBy?: string
  /** Set when not allowed: the team whose declared list narrowed it out. */
  narrowedBy?: string
}

export interface TeamPalette {
  team: string
  /** Every catalogue entry, allowed or not, in catalogue order. */
  rows: PaletteRow[]
  /** The teams on the chain with declared lists, root-first: provenance for `allow-list` rows. */
  declaredLists: string[]
}

/**
 * The allow-list pattern vocabulary is literals plus `*` and `?` only
 * (mirroring internal/allowlist parseEntry); everything else is escaped so
 * no authored entry can be a malformed pattern at match time.
 */
function globToRegExp(pattern: string): RegExp {
  const escaped = pattern
    .replace(/[.+^${}()|[\]\\]/g, '\\$&')
    .replace(/\*/g, '.*')
    .replace(/\?/g, '.')
  return new RegExp(`^${escaped}$`)
}

/**
 * Whether one authored `class/type-pattern` entry selects the catalogue
 * entry. The class side is exact; the pattern is tried against the
 * canonical type and the `deprecated_type` alias: aliases resolve on
 * every lookup (ADR-0020 §3).
 */
export function entrySelects(pattern: string, entry: CatalogueEntry): boolean {
  const cut = pattern.indexOf('/')
  if (cut <= 0) return false
  if (pattern.slice(0, cut) !== entry.class) return false
  const type = globToRegExp(pattern.slice(cut + 1))
  if (type.test(entry.type)) return true
  return entry.deprecatedType !== undefined && type.test(entry.deprecatedType)
}

/** The teams from the root down to `team`, inclusive; undefined when the tree lacks it. */
export function chainOf(tree: TeamNode, team: string): string[] | undefined {
  if (tree.id === team) return [tree.id]
  for (const child of tree.teams ?? []) {
    const below = chainOf(child, team)
    if (below) return [tree.id, ...below]
  }
  return undefined
}

/**
 * Computes one team's effective palette per ADR-0021, walking the chain
 * from the root down: each declared list intersects, then each Grant
 * targeting that team unions back in, so a Grant widens from its target's
 * subtree downward and a descendant's list narrows it back out.
 */
export function effectivePalette(args: {
  tree: TeamNode
  team: string
  entries: CatalogueEntry[]
  governance: GovernancePayload
}): TeamPalette | undefined {
  const { tree, team, entries, governance } = args
  const chain = chainOf(tree, team)
  if (!chain) return undefined

  const lists = new Map(governance.allowLists.map((list) => [list.team, list]))
  const ownerTeams = new Map(governance.owners.map((owner) => [owner.id, owner.team]))
  const byTarget = new Map<string, GrantDoc[]>()
  for (const grant of governance.grants) {
    const targeting = byTarget.get(grant.team) ?? []
    targeting.push(grant)
    byTarget.set(grant.team, targeting)
  }
  // Grants apply in id order: the id is the audit chain's name for them.
  for (const targeting of byTarget.values()) {
    targeting.sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
  }

  const declaredLists = chain.filter((u) => lists.has(u))

  const rows: PaletteRow[] = entries.map((entry) => {
    let allowed = true
    let via: GrantDoc | undefined
    let narrowedBy: string | undefined
    for (const u of chain) {
      const list = lists.get(u)
      if (list && !list.allow.some((pattern) => entrySelects(pattern, entry))) {
        // The intersection removes it, including a component a Grant
        // higher up admitted: narrowed back out below.
        allowed = false
        via = undefined
        narrowedBy = u
      }
      if (!allowed) {
        for (const grant of byTarget.get(u) ?? []) {
          if (grant.adds.some((pattern) => entrySelects(pattern, entry))) {
            allowed = true
            via = grant
            narrowedBy = undefined
            break
          }
        }
      }
    }

    if (!allowed) return { entry, allowed, narrowedBy }
    if (via) {
      return {
        entry,
        allowed,
        origin: 'grant',
        grant: via,
        grantedBy: ownerTeams.get(via.owner),
      }
    }
    return { entry, allowed, origin: declaredLists.length > 0 ? 'allow-list' : 'default-allow' }
  })

  return { team, rows, declaredLists }
}
