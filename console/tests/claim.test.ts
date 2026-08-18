import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import type { CollectorRow } from '../src/api/types'
import { formatSelector, parseSelector, suggestSelector } from '../src/estate/claim'
import {
  claimContextProblems,
  previewClaim,
  submitClaim,
  ungovernedSummary,
} from '../tools/claims'

// The claim flow (ADR-0042 §6, ADR-0031): herd-first, generalise-never-
// enumerate. The console-side suggestion generalises over the herd's
// shared identity attributes and drops instance-naming keys; the server
// side enforces the same rule independently, ranks attach candidates by
// selector proximity, reports the blast radius, and exits as a proposal —
// or refuses with the problems named.

const estate = JSON.parse(
  readFileSync(fileURLToPath(new URL('../fixtures/estate.json', import.meta.url)), 'utf8'),
) as { collectors: CollectorRow[] }

const herd = estate.collectors.filter((row) => row.ungoverned === 'served')

describe('suggestSelector (ADR-0042 §6)', () => {
  it('generalises over the identity attributes the whole herd shares', () => {
    expect(suggestSelector(herd)).toEqual({
      'deployment.environment': 'production',
      'k8s.cluster': 'prod-east',
      'k8s.namespace': 'payments',
      'telecraft.tier': 'edge',
    })
  })

  it('never offers an instance-naming key, even when the herd is one collector', () => {
    const one = herd.slice(0, 1)
    expect(one[0]?.attributes?.['service.instance.id']).toBeDefined()
    expect(Object.keys(suggestSelector(one))).not.toContain('service.instance.id')
  })

  it('drops attributes the herd disagrees on — a mixed herd keeps only common ground', () => {
    const foreign = estate.collectors.filter((row) => row.ungoverned === 'foreign')
    const mixed = suggestSelector([...herd, ...foreign])
    expect(mixed).toEqual({ 'deployment.environment': 'production' })
  })

  it('suggests nothing for an empty herd', () => {
    expect(suggestSelector([])).toEqual({})
  })
})

describe('the selector wire shape', () => {
  it('round-trips through the URL form, keys sorted', () => {
    const selector = { 'k8s.namespace': 'payments', 'telecraft.tier': 'edge' }
    const wire = formatSelector(selector)
    expect(wire).toBe('k8s.namespace=payments,telecraft.tier=edge')
    expect(parseSelector(wire)).toEqual(selector)
  })
})

describe('the ungoverned band summary (ADR-0031 §2)', () => {
  it('splits by how the collector is read: served the Unmatched artefact, or foreign', () => {
    expect(ungovernedSummary(estate)).toEqual({ served: 3, foreign: 2 })
  })
})

const suggested = suggestSelector(herd)

describe('previewClaim (ADR-0042 §6)', () => {
  it('counts the ungoverned collectors the selector matches, by referent', () => {
    const preview = previewClaim(estate, { selector: suggested, environment: 'production' })
    expect(preview.matched).toEqual({ total: 3, served: 3, foreign: 0 })
  })

  it('ranks attach candidates by selector proximity, shared pairs as the widened selector', () => {
    const preview = previewClaim(estate, { selector: suggested, environment: 'production' })
    const first = preview.candidates[0]
    expect(first?.tier).toBe('data-flow/edge')
    expect(first?.satisfied).toBe(2)
    expect(first?.of).toBe(3)
    expect(first?.widened).toEqual({
      'telecraft.tier': 'edge',
      'deployment.environment': 'production',
    })
    // gateway-staging contradicts on both pairs and never appears.
    expect(preview.candidates.map((c) => c.tier)).not.toContain('data-flow/gateway-staging')
  })

  it('reports the blast radius when a constrained selector stops contradicting a governed Tier', () => {
    const narrow = previewClaim(estate, { selector: suggested, environment: 'production' })
    expect(narrow.overlaps).toEqual([])
    const { 'k8s.namespace': _dropped, ...widened } = suggested
    const wide = previewClaim(estate, { selector: widened, environment: 'production' })
    expect(wide.overlaps.map((o) => o.tier)).toContain('product/storefront-edge')
  })

  it('judges attach with the widened selector — what merge would actually serve', () => {
    const preview = previewClaim(estate, {
      selector: suggested,
      environment: 'production',
      team: 'data-flow',
      mode: 'attach',
      tier: 'data-flow/edge',
    })
    // Widened to {telecraft.tier: edge, deployment.environment: production}:
    // the herd matches, and the storefront population is in reach.
    expect(preview.matched.total).toBe(3)
    expect(preview.overlaps.map((o) => o.tier)).toContain('product/storefront-edge')
    expect(preview.rendered).toContain('selector widened by the claim')
    expect(preview.rendered).toContain('telecraft.tier: edge')
    expect(preview.rendered).not.toContain('k8s.namespace')
  })

  it('renders the drafted Tier binding the PR would carry', () => {
    const preview = previewClaim(estate, {
      selector: suggested,
      environment: 'production',
      team: 'data-flow',
      mode: 'draft',
      tier: 'data-flow/payments-edge',
    })
    expect(preview.rendered).toContain('teams/data-flow/tiers/payments-edge.yaml')
    expect(preview.rendered).toContain('blueprint: data-flow/payments-edge-standard@1')
    expect(preview.rendered).toContain('k8s.namespace: payments')
    // The binding names a population, never an instance.
    expect(preview.rendered).not.toContain('service.instance.id')
    expect(preview.rendered).not.toContain('pay-edge')
  })
})

describe('submitClaim — fail closed, problems named', () => {
  it('refuses a selector that enumerates instance ids, whatever the UI did', () => {
    const outcome = submitClaim(estate, {
      selector: { 'service.instance.id': '7f3a2c91-pay-0' },
      environment: 'production',
      team: 'data-flow',
      mode: 'draft',
      tier: 'data-flow/payments-edge',
      title: 'Claim',
    })
    expect(outcome.problems?.join(' ')).toContain('never enumerates instance ids')
  })

  it('refuses an empty selector, an unknown team, and a missing title', () => {
    const outcome = submitClaim(estate, {
      selector: {},
      environment: 'production',
      team: 'nobody',
      mode: 'attach',
      tier: 'data-flow/edge',
      title: ' ',
    })
    expect(outcome.problems?.some((p) => p.includes('selector is empty'))).toBe(true)
    expect(outcome.problems?.some((p) => p.includes('not in the team tree'))).toBe(true)
    expect(outcome.problems?.some((p) => p.includes('no title'))).toBe(true)
  })

  it('refuses attach when no selector pair is shared — attach widens, it never enumerates', () => {
    const outcome = submitClaim(estate, {
      selector: { region: 'eu-west' },
      environment: 'production',
      team: 'data-flow',
      mode: 'attach',
      tier: 'data-flow/edge',
      title: 'Claim',
    })
    expect(outcome.problems?.join(' ')).toContain('shares no selector pair')
  })

  it('opens a user-attributed proposal on the happy attach path (ADR-0014)', () => {
    const outcome = submitClaim(estate, {
      selector: suggested,
      environment: 'production',
      team: 'data-flow',
      mode: 'attach',
      tier: 'data-flow/edge',
      title: 'Claim 3 ungoverned collectors into data-flow/edge',
    })
    expect(outcome.problems).toBeUndefined()
    expect(outcome.proposal?.branch).toBe('claim/data-flow/edge')
    expect(outcome.proposal?.attributedTo).toContain('demo-user@estate.internal')
    expect(outcome.proposal?.url).toMatch(/^https:\/\/forge\.example\//)
  })
})

describe('the Compose draft path carries the same rulebook', () => {
  it('refuses a claim context whose Tier already exists', () => {
    const problems = claimContextProblems(estate, {
      selector: suggested,
      tier: 'data-flow/edge',
      team: 'data-flow',
      environment: 'production',
    })
    expect(problems.join(' ')).toContain('already exists')
  })

  it('accepts the drafted claim the panel hands to Compose', () => {
    expect(
      claimContextProblems(estate, {
        selector: suggested,
        tier: 'data-flow/payments-edge',
        team: 'data-flow',
        environment: 'production',
      }),
    ).toEqual([])
  })
})
