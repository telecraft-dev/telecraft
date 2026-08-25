import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import type { BlueprintDoc, CatalogueEntry } from '../src/api/types'
import { propose, validate } from '../tools/evaluator'

// The one evaluator behind /api/v1/validate and /api/v1/proposals
// (ADR-0022), judging with the same authored governance policy
// /api/v1/governance serves and the active catalogue's entries: palette
// membership with Grants (ADR-0021), the show/grey/hide semantics (§5),
// the single allow-list hard block (§3), floors as environment-scoped
// findings (ADR-0023), ordering wisdom as findings (ADR-0024 §6),
// requirement claims judged as intent beside fact (REQ-031), and the
// rendered preview's provenance-carrying ids (ADR-0024 §5).

const load = (name: string) =>
  JSON.parse(readFileSync(fileURLToPath(new URL(`../fixtures/${name}`, import.meta.url)), 'utf8'))

const estate = load('estate.json') as { blueprints: BlueprintDoc[] }
const catalogues = load('catalogues.json') as {
  active: string
  versions: { version: string; components: CatalogueEntry[] }[]
}
const entries = catalogues.versions.find((v) => v.version === catalogues.active)?.components ?? []

const judge = (draft: BlueprintDoc, environment: string) =>
  validate(estate, entries, draft, environment)

const doc = (id: string): BlueprintDoc => {
  const found = estate.blueprints.find((b) => b.id === id)
  if (!found) throw new Error(`no fixture blueprint ${id}`)
  return structuredClone(found)
}

describe('the palette (ADR-0021, ADR-0022 §5)', () => {
  const pal = judge(doc('data-flow/edge-standard'), 'production').palette

  it('hides non-allowed entries entirely, with an admitted count', () => {
    const keys = pal.entries.map((e) => e.key)
    // The chain narrows: platform's list drops opencensus; data-flow's own
    // list drops filelog, pii_scrub, debug and the connector.
    expect(keys).not.toContain('type:exporter/debug')
    expect(keys).not.toContain('type:processor/pii_scrub')
    expect(keys).not.toContain('type:exporter/opencensus')
    expect(pal.hidden).toBe(5)
  })

  it('admits a Grant-origin entry with its named Grant: the audit chain is total', () => {
    const kafka = pal.entries.find((e) => e.key === 'type:receiver/kafka')
    expect(kafka?.origin).toBe('grant')
    expect(kafka?.grant).toEqual({
      id: 'kafka-egress-for-data-flow',
      grantedBy: 'platform',
      grantedTo: 'data-flow',
    })
    const shared = pal.entries.find((e) => e.key === 'shared:infosec/pii-redaction')
    expect(shared?.origin).toBe('allow-list')
    expect(shared?.add.ref).toBe('infosec/pii-redaction@3')
  })

  it('greys a floor-breaching entry with the reason, in production only', () => {
    const filter = pal.entries.find((e) => e.key === 'type:processor/filter')
    expect(filter?.state).toBe('greyed')
    expect(filter?.reason).toContain('alpha on traces')
    expect(filter?.reason).toContain('C1 floor in production')

    const staging = judge(doc('data-flow/edge-standard'), 'staging').palette
    const inStaging = staging.entries.find((e) => e.key === 'type:processor/filter')
    expect(inStaging?.state).toBe('allowed')
  })

  it('shows the whole catalogue under the default-allow posture, deprecation as data', () => {
    // infosec declares no list on its chain: the effective list is the
    // whole active Catalogue (ADR-0021 §4), deprecated entries included.
    const infosec = judge(doc('infosec/audit-standard'), 'production').palette
    expect(infosec.hidden).toBe(0)
    const opencensus = infosec.entries.find((e) => e.key === 'type:exporter/opencensus')
    expect(opencensus?.origin).toBe('default-allow')
    expect(opencensus?.deprecated?.migration).toContain('OTLP')
  })
})

describe('the save gate (ADR-0022 §3)', () => {
  it('blocks only on an allow-list violation', () => {
    const blocked = judge(doc('data-flow/gateway-standard'), 'production')
    expect(blocked.save.blocked).toBe(true)
    expect(blocked.save.reasons.join()).toContain('debug-tap')
    expect(blocked.findings.some((f) => f.kind === 'allow-list' && f.lane === 'metrics')).toBe(true)

    const clean = judge(doc('data-flow/edge-standard'), 'production')
    expect(clean.save.blocked).toBe(false)
  })

  it('never blocks on floors: the finding appears, Save stays open', () => {
    const draft = doc('data-flow/edge-standard')
    draft.locals['scrub'] = { class: 'processor', type: 'filter' }
    draft.lanes['traces'] = ['otlp-in', 'scrub', 'batcher', 'data-flow/gateway-exporter@2']
    const verdict = judge(draft, 'production')
    expect(verdict.findings.some((f) => f.kind === 'floor' && f.ref === 'scrub')).toBe(true)
    expect(verdict.save.blocked).toBe(false)
  })

  it('persists the allow-list finding across the environment toggle; floors clear', () => {
    const draft = doc('data-flow/gateway-standard')
    draft.locals['scrub'] = { class: 'processor', type: 'filter' }
    draft.lanes['traces'] = [...(draft.lanes['traces'] ?? []), 'scrub']
    const production = judge(draft, 'production')
    expect(production.findings.some((f) => f.kind === 'floor')).toBe(true)
    expect(production.save.blocked).toBe(true)

    const staging = judge(draft, 'staging')
    expect(staging.findings.some((f) => f.kind === 'floor')).toBe(false)
    expect(staging.save.blocked).toBe(true)
  })
})

describe('ordering findings (ADR-0024 §6)', () => {
  it('raises a finding when batch is not the last processor, never a re-sort', () => {
    const draft = doc('data-flow/edge-standard')
    draft.locals['scrub'] = { class: 'processor', type: 'transform' }
    draft.lanes['traces'] = ['otlp-in', 'batcher', 'scrub', 'data-flow/gateway-exporter@2']
    const verdict = judge(draft, 'production')
    const ordering = verdict.findings.find((f) => f.kind === 'ordering' && f.lane === 'traces')
    expect(ordering?.summary).toContain('batch belongs last')
    expect(ordering?.severity).toBe('advisory')
    // The lane the response reflects is the authored order, untouched.
    expect(verdict.yaml).toContain('- batch/batcher\n        - transform/scrub')
  })

  it('raises a finding when memory_limiter is not first among processors', () => {
    const draft = doc('data-flow/edge-standard')
    draft.locals['guard'] = { class: 'processor', type: 'memory_limiter' }
    draft.lanes['traces'] = ['otlp-in', 'batcher', 'guard', 'data-flow/gateway-exporter@2']
    const verdict = judge(draft, 'production')
    expect(
      verdict.findings.some(
        (f) => f.kind === 'ordering' && f.summary.includes('memory_limiter belongs first'),
      ),
    ).toBe(true)
  })
})

describe('reference and lifecycle findings (ADR-0024, ADR-0023 §6)', () => {
  it('flags a lane routing a signal the component does not declare, never a block', () => {
    const draft = doc('infosec/audit-standard')
    draft.locals['scrub'] = { class: 'processor', type: 'pii_scrub' }
    draft.lanes['metrics'] = ['otlp-in', 'scrub', 'batcher']
    const verdict = judge(draft, 'production')
    expect(
      verdict.findings.some(
        (f) => f.kind === 'reference' && f.summary.includes('declares no metrics support'),
      ),
    ).toBe(true)
    expect(verdict.save.blocked).toBe(false)
  })

  it('raises a lifecycle finding with the migration text for a deprecated entry', () => {
    const draft = doc('infosec/audit-standard')
    draft.locals['legacy-out'] = { class: 'exporter', type: 'opencensus' }
    draft.lanes['logs'] = [...(draft.lanes['logs'] ?? [])]
    draft.lanes['traces'] = ['otlp-in', 'batcher', 'legacy-out']
    const verdict = judge(draft, 'production')
    const lifecycle = verdict.findings.find((f) => f.kind === 'lifecycle' && f.ref === 'legacy-out')
    expect(lifecycle?.severity).toBe('advisory')
    expect(lifecycle?.remediation).toContain('OTLP')
    expect(verdict.save.blocked).toBe(false)
  })
})

describe('requirement verdicts (REQ-031, ADR-0026)', () => {
  it('carries claim and judgement side by side: claimed is never met', () => {
    const verdicts = judge(doc('data-flow/edge-standard'), 'production').requirements
    const pii = verdicts.find((r) => r.id === 'req-pii-redaction')
    expect(pii?.claimed).toBe(true)
    expect(pii?.claimedVersion).toBe(3)
    expect(pii?.met).toBe(false)
    const completeness = verdicts.find((r) => r.id === 'req-payment-completeness')
    expect(completeness?.claimed).toBe(false)
    expect(completeness?.met).toBe(false)
  })

  it('judges a requirement met once the suggested component reaches every named lane', () => {
    const draft = doc('data-flow/edge-standard')
    for (const signal of ['traces', 'logs']) {
      draft.lanes[signal] = ['otlp-in', 'infosec/pii-redaction@3', 'batcher', 'data-flow/gateway-exporter@2']
    }
    const pii = judge(draft, 'production').requirements.find((r) => r.id === 'req-pii-redaction')
    expect(pii?.met).toBe(true)
  })
})

describe('the rendered preview (REQ-035, ADR-0024 §5)', () => {
  it('compiles provenance-carrying ids: type/name local, type/team.name shared', () => {
    const yaml = judge(doc('data-flow/gateway-standard'), 'production').yaml
    expect(yaml).toContain('transform/infosec.pii-redaction: {}')
    expect(yaml).toContain('memory_limiter/guard: {}')
    expect(yaml).toContain('otlphttp/data-flow.gateway-exporter: {}')
    expect(yaml).toContain('  pipelines:\n    traces:')
    expect(yaml).toContain('# Tier data-flow/gateway (production)')
  })
})

describe('the proposal exit (ADR-0028)', () => {
  it('fails closed on the hard block: no proposal, and the message matches the surface', () => {
    expect(() =>
      propose(estate, entries, doc('data-flow/gateway-standard'), 'production'),
    ).toThrowError(/Save is disabled: .*Allow-list/)
  })

  it('opens a branch-per-draft proposal, attributed to the acting human', () => {
    const proposal = propose(estate, entries, doc('data-flow/edge-standard'), 'production')
    expect(proposal.branch).toBe('compose/data-flow/edge-standard')
    expect(proposal.url).toMatch(/\/pull\/\d+$/)
    expect(proposal.attributedTo).toBe('Demo user <demo-user@estate.internal>')
  })
})
