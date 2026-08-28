import { describe, expect, it } from 'vitest'
import type {
  ActivationsPayload,
  BlueprintDoc,
  CatalogueComponent,
  CatalogueEntry,
  ComponentClass,
  GovernancePayload,
  TeamNode,
} from '../src/api/types'
import { effectivePalette } from '../src/governance/effective'
import {
  allowListStanding,
  claimRows,
  grantsInForce,
  offerReports,
} from '../src/surfaces/compose/standing'

// The Compose landing's derivations (ADR-0064). The landing derives and
// never judges: the Allow-list standing comes through the same
// governance/effective.ts palette the Effective palette view reads,
// because only an Allow-list violation blocks a save (ADR-0022 §3), so a
// row's standing here cannot disagree with the composer the row opens.

const teams: TeamNode = {
  id: 'engineering',
  name: 'Engineering',
  teams: [
    { id: 'platform', name: 'Platform', teams: [{ id: 'data-flow', name: 'Data Flow' }] },
    { id: 'infosec', name: 'Information Security' },
  ],
}

const entry = (cls: ComponentClass, type: string): CatalogueEntry => ({
  class: cls,
  type,
  source: 'upstream',
  stability: {},
})

const entries: CatalogueEntry[] = [
  entry('receiver', 'otlp'),
  entry('processor', 'batch'),
  entry('processor', 'transform'),
  entry('exporter', 'otlphttp'),
  entry('exporter', 'debug'),
  entry('exporter', 'kafka'),
  entry('receiver', 'kafka'),
]

const governance: GovernancePayload = {
  owners: [{ id: 'platform-lead', name: 'Platform lead', team: 'platform' }],
  allowLists: [
    {
      team: 'platform',
      owner: 'platform-lead',
      allow: ['receiver/otlp', 'processor/*', 'exporter/otlphttp', 'exporter/debug'],
    },
    {
      team: 'data-flow',
      owner: 'platform-lead',
      allow: ['receiver/otlp', 'processor/batch', 'processor/transform', 'exporter/otlphttp'],
    },
  ],
  grants: [
    {
      id: 'kafka-egress-for-data-flow',
      owner: 'platform-lead',
      team: 'data-flow',
      adds: ['receiver/kafka', 'exporter/kafka'],
    },
  ],
}

const shared: CatalogueComponent[] = [
  {
    id: 'infosec/pii-redaction',
    name: 'pii-redaction',
    version: 3,
    team: 'infosec',
    class: 'processor',
    type: 'transform',
  },
]

const doc = (over: Partial<BlueprintDoc> = {}): BlueprintDoc => ({
  id: 'data-flow/gateway-standard',
  name: 'gateway-standard',
  version: 4,
  team: 'data-flow',
  locals: {
    'otlp-in': { class: 'receiver', type: 'otlp' },
    batcher: { class: 'processor', type: 'batch' },
    'debug-tap': { class: 'exporter', type: 'debug' },
  },
  lanes: {
    traces: ['otlp-in', 'batcher'],
  },
  extensions: [],
  satisfies: [],
  ...over,
})

const palette = effectivePalette({ tree: teams, team: 'data-flow', entries, governance })
if (palette === undefined) throw new Error('the test team is missing from its own tree')

const standing = (d: BlueprintDoc) => allowListStanding({ doc: d, palette, shared, entries })

describe('allowListStanding (ADR-0064, mirroring ADR-0022 §3)', () => {
  it('reads clear when every lane reference is inside the effective Allow-list', () => {
    expect(standing(doc())).toEqual({ blocked: false, facts: [] })
  })

  it('reads blocked with the fact when a reference falls outside the Allow-list', () => {
    const judged = standing(doc({ lanes: { metrics: ['otlp-in', 'batcher', 'debug-tap'] } }))
    expect(judged.blocked).toBe(true)
    expect(judged.facts).toEqual([
      "debug-tap (exporter/debug) is outside this team's effective Allow-list",
    ])
  })

  it('names an offending reference once, however many lanes route it', () => {
    const judged = standing(
      doc({
        lanes: {
          traces: ['debug-tap'],
          logs: ['debug-tap'],
          metrics: ['debug-tap'],
        },
      }),
    )
    expect(judged.facts).toHaveLength(1)
  })

  it('a Grant-admitted entry reads clear, as the engine judges it', () => {
    const judged = standing(
      doc({
        locals: { 'kafka-out': { class: 'exporter', type: 'kafka' } },
        lanes: { traces: ['kafka-out'] },
      }),
    )
    expect(judged.blocked).toBe(false)
  })

  it('a key the active Catalogue does not carry is outside every Allow-list', () => {
    const judged = standing(
      doc({
        locals: { legacy: { class: 'exporter', type: 'opencensus' } },
        lanes: { traces: ['legacy'] },
      }),
    )
    expect(judged.blocked).toBe(true)
    expect(judged.facts).toEqual([
      "legacy (exporter/opencensus) is outside this team's effective Allow-list",
    ])
  })

  it('an unresolvable reference never blocks: that is a reference finding, the composer raises it', () => {
    const judged = standing(doc({ lanes: { traces: ['ghost'] } }))
    expect(judged).toEqual({ blocked: false, facts: [] })
  })

  it('a shared reference resolves through the shared Component list when the doc carries no key map', () => {
    const judged = standing(
      doc({ locals: {}, lanes: { logs: ['infosec/pii-redaction@3'] } }),
    )
    expect(judged.blocked).toBe(false)
  })

  it('a shared reference resolves through the doc key map first', () => {
    const judged = standing(
      doc({
        locals: {},
        components: { 'acme/tap@1': { class: 'exporter', type: 'debug' } },
        lanes: { logs: ['acme/tap@1'] },
      }),
    )
    expect(judged.facts).toEqual([
      "acme/tap@1 (exporter/debug) is outside this team's effective Allow-list",
    ])
  })
})

describe('claimRows (ADR-0064, REQ-031: intent, never fact)', () => {
  const first = doc({ id: 'a', name: 'a', satisfies: ['req-one@3', 'req-two@2'] })
  const second = doc({ id: 'b', name: 'b', satisfies: ['req-one@3'] })

  it('groups by claim in first-appearance order, one row per claiming Blueprint', () => {
    expect(claimRows([first, second]).map((row) => [row.claim, row.blueprint.id])).toEqual([
      ['req-one@3', 'a'],
      ['req-one@3', 'b'],
      ['req-two@2', 'a'],
    ])
  })

  it('names a Blueprint once per claim, however often the doc stamps it', () => {
    const stuttering = doc({ id: 'a', satisfies: ['req-one@3', 'req-one@3'] })
    expect(claimRows([stuttering])).toHaveLength(1)
  })

  it('is empty when no Blueprint claims anything', () => {
    expect(claimRows([doc()])).toEqual([])
  })
})

describe('grantsInForce (ADR-0064)', () => {
  it('names a Grant once, however many entries it admits', () => {
    const grants = grantsInForce(palette)
    expect(grants.map((grant) => grant.id)).toEqual(['kafka-egress-for-data-flow'])
    expect(grants[0]?.adds).toEqual(['receiver/kafka', 'exporter/kafka'])
  })

  it('is empty when nothing on the palette is Grant-admitted', () => {
    const infosec = effectivePalette({ tree: teams, team: 'infosec', entries, governance })
    if (infosec === undefined) throw new Error('the test team is missing from its own tree')
    expect(grantsInForce(infosec)).toEqual([])
  })
})

describe('offerReports (ADR-0064, repeating the ADR-0020 §6 report verbatim)', () => {
  const activations: ActivationsPayload = {
    substrates: [
      {
        kind: 'catalogue',
        name: 'Catalogue',
        active: 'v0.158.0',
        candidates: [
          {
            version: 'v0.155.0',
            summary: '1 component in use is removed.',
            lines: ['processor/transform is removed. 1 Blueprint uses it.'],
          },
          {
            version: 'v0.140.0',
            summary: '',
            lines: [],
            blocked: 'The import predates impact reports.',
          },
        ],
        history: [],
      },
      {
        kind: 'schema_registry',
        name: 'Schema Registry',
        active: '',
        candidates: [
          { version: 'v9', summary: 'unrelated', lines: ['not a Catalogue line'] },
        ],
        history: [],
      },
    ],
  }

  it('carries each Catalogue candidate with its report lines', () => {
    expect(offerReports(activations)).toEqual([
      {
        version: 'v0.155.0',
        lines: ['processor/transform is removed. 1 Blueprint uses it.'],
      },
      { version: 'v0.140.0', lines: [] },
    ])
  })

  it('reads only the Catalogue substrate, and no substrate as no reports', () => {
    expect(offerReports({ substrates: [] })).toEqual([])
  })
})
