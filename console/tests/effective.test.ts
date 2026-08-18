import { describe, expect, it } from 'vitest'
import type { CatalogueEntry, GovernancePayload, TeamNode } from '../src/api/types'
import { chainOf, effectivePalette, entrySelects } from '../src/governance/effective'

// The ratified ADR-0021 semantics (issue #15), pinned against the console's
// derived-presentation mirror: narrowing-only inheritance, default-allow,
// and Grants that override their own target's declared list (union after
// intersection) while lists below the target still narrow them out.

const tree: TeamNode = {
  id: 'root',
  name: 'Root',
  teams: [
    { id: 'mid', name: 'Mid', teams: [{ id: 'leaf', name: 'Leaf' }] },
    { id: 'aside', name: 'Aside' },
  ],
}

function entry(key: string, deprecatedType?: string): CatalogueEntry {
  const cut = key.indexOf('/')
  return {
    class: key.slice(0, cut) as CatalogueEntry['class'],
    type: key.slice(cut + 1),
    deprecatedType,
    source: 'upstream',
    stability: { traces: 'beta' },
  }
}

const OTLP = entry('receiver/otlp')
const KAFKA = entry('exporter/kafka')
const DEBUG = entry('exporter/debug')
const SPAN_METRICS = entry('connector/span_metrics', 'spanmetrics')
const ENTRIES = [OTLP, KAFKA, DEBUG, SPAN_METRICS]

const OWNERS = [
  { id: 'root-lead', name: 'Root lead', team: 'root' },
  { id: 'mid-lead', name: 'Mid lead', team: 'mid' },
]

function governance(over: Partial<GovernancePayload>): GovernancePayload {
  return { owners: OWNERS, allowLists: [], grants: [], ...over }
}

function row(palette: ReturnType<typeof effectivePalette>, target: CatalogueEntry) {
  const found = palette?.rows.find((r) => r.entry === target)
  if (!found) throw new Error('entry missing from palette rows')
  return found
}

describe('entry patterns', () => {
  it('matches exact names, families, and single characters — class side exact', () => {
    expect(entrySelects('receiver/otlp', OTLP)).toBe(true)
    expect(entrySelects('exporter/kafka*', KAFKA)).toBe(true)
    expect(entrySelects('exporter/kafk?', KAFKA)).toBe(true)
    expect(entrySelects('receiver/kafka', KAFKA)).toBe(false)
    expect(entrySelects('exporter/*', OTLP)).toBe(false)
  })

  it('resolves the deprecated_type alias on every lookup (ADR-0020 §3)', () => {
    expect(entrySelects('connector/spanmetrics', SPAN_METRICS)).toBe(true)
    expect(entrySelects('connector/span_metrics', SPAN_METRICS)).toBe(true)
  })

  it('treats regex metacharacters in types as literals', () => {
    const dotted = entry('receiver/otlp.http')
    expect(entrySelects('receiver/otlp.http', dotted)).toBe(true)
    expect(entrySelects('receiver/otlpxhttp', dotted)).toBe(false)
  })
})

describe('the chain', () => {
  it('runs from the root down to the team, inclusive', () => {
    expect(chainOf(tree, 'leaf')).toEqual(['root', 'mid', 'leaf'])
    expect(chainOf(tree, 'root')).toEqual(['root'])
    expect(chainOf(tree, 'nowhere')).toBeUndefined()
  })
})

describe('the effective palette (ADR-0021)', () => {
  it('defaults to allow: absent any list, the whole catalogue (§4)', () => {
    const palette = effectivePalette({ tree, team: 'leaf', entries: ENTRIES, governance: governance({}) })
    for (const target of ENTRIES) {
      expect(row(palette, target)).toMatchObject({ allowed: true, origin: 'default-allow' })
    }
  })

  it('narrows only: each declared list on the chain intersects (§2)', () => {
    const palette = effectivePalette({
      tree,
      team: 'leaf',
      entries: ENTRIES,
      governance: governance({
        allowLists: [
          { team: 'root', owner: 'root-lead', allow: ['receiver/otlp', 'exporter/*'] },
          { team: 'leaf', owner: 'root-lead', allow: ['receiver/otlp', 'exporter/debug'] },
        ],
      }),
    })
    expect(row(palette, OTLP)).toMatchObject({ allowed: true, origin: 'allow-list' })
    expect(row(palette, DEBUG)).toMatchObject({ allowed: true, origin: 'allow-list' })
    // narrowedBy names the deepest excluding list: a rescuing Grant must
    // target at or below it, so it is the provenance a request needs.
    expect(row(palette, KAFKA)).toMatchObject({ allowed: false, narrowedBy: 'leaf' })
    expect(row(palette, SPAN_METRICS)).toMatchObject({ allowed: false, narrowedBy: 'leaf' })
    expect(palette?.declaredLists).toEqual(['root', 'leaf'])
  })

  it('a Grant overrides its own target\'s declared list — union after intersection (§3)', () => {
    const palette = effectivePalette({
      tree,
      team: 'leaf',
      entries: ENTRIES,
      governance: governance({
        allowLists: [
          { team: 'root', owner: 'root-lead', allow: ['receiver/otlp'] },
          { team: 'leaf', owner: 'root-lead', allow: ['receiver/otlp'] },
        ],
        grants: [{ id: 'kafka-for-leaf', owner: 'mid-lead', team: 'leaf', adds: ['exporter/kafka'] }],
      }),
    })
    // Excluded by the root list AND the target's own list, admitted by the
    // Grant regardless: the audit chain names it.
    expect(row(palette, KAFKA)).toMatchObject({
      allowed: true,
      origin: 'grant',
      grantedBy: 'mid',
    })
    expect(row(palette, KAFKA).grant?.id).toBe('kafka-for-leaf')
    expect(row(palette, DEBUG)).toMatchObject({ allowed: false })
  })

  it('lists below the target still narrow a granted component back out (§3)', () => {
    const grants = [{ id: 'kafka-for-mid', owner: 'root-lead', team: 'mid', adds: ['exporter/kafka'] }]
    const allowLists = [
      { team: 'root', owner: 'root-lead', allow: ['receiver/otlp'] },
      { team: 'leaf', owner: 'root-lead', allow: ['receiver/otlp'] },
    ]
    const mid = effectivePalette({
      tree,
      team: 'mid',
      entries: ENTRIES,
      governance: governance({ allowLists, grants }),
    })
    expect(row(mid, KAFKA)).toMatchObject({ allowed: true, origin: 'grant' })
    // The leaf's declared list narrows the mid-targeted Grant back out.
    const leaf = effectivePalette({
      tree,
      team: 'leaf',
      entries: ENTRIES,
      governance: governance({ allowLists, grants }),
    })
    expect(row(leaf, KAFKA)).toMatchObject({ allowed: false, narrowedBy: 'leaf' })
  })

  it('a Grant applies from its target subtree downward, never sideways or above', () => {
    const gov = governance({
      allowLists: [{ team: 'root', owner: 'root-lead', allow: ['receiver/otlp'] }],
      grants: [{ id: 'kafka-for-leaf', owner: 'root-lead', team: 'leaf', adds: ['exporter/kafka'] }],
    })
    expect(
      row(effectivePalette({ tree, team: 'mid', entries: ENTRIES, governance: gov }), KAFKA),
    ).toMatchObject({ allowed: false })
    expect(
      row(effectivePalette({ tree, team: 'aside', entries: ENTRIES, governance: gov }), KAFKA),
    ).toMatchObject({ allowed: false })
    expect(
      row(effectivePalette({ tree, team: 'leaf', entries: ENTRIES, governance: gov }), KAFKA),
    ).toMatchObject({ allowed: true, origin: 'grant' })
  })

  it('an entry written against the historical type name keeps selecting (§ADR-0020)', () => {
    const palette = effectivePalette({
      tree,
      team: 'leaf',
      entries: ENTRIES,
      governance: governance({
        allowLists: [{ team: 'root', owner: 'root-lead', allow: ['connector/spanmetrics'] }],
      }),
    })
    expect(row(palette, SPAN_METRICS)).toMatchObject({ allowed: true, origin: 'allow-list' })
  })
})
