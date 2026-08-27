import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import type { TierProposalRequest } from '../src/api/types'
import { setupGuidance, tierProblems } from '../tools/tiers'

// The Tier-first onboarding flow's server side (ADR-0060): the proposal
// exit validates fail-closed with every problem named in the reader's
// words, and the setup guidance is generated on view from the Tier, the
// activated Catalogue version, and the estate settings; never committed,
// never judged.

const load = (name: string) =>
  JSON.parse(readFileSync(fileURLToPath(new URL(`../fixtures/${name}`, import.meta.url)), 'utf8'))

const estate = load('estate.json')
const catalogues = load('catalogues.json') as { active: string }

const valid: TierProposalRequest = {
  title: 'Add the checkout gateway Tier',
  name: 'checkout-gateway',
  team: 'data-flow',
  owner: 'dataflow-lead',
  environment: 'production',
  blueprint: 'data-flow/gateway-standard',
  blueprintVersion: 4,
  selector: {
    'deployment.environment': 'production',
    'telecraft.tier': 'checkout-gateway',
  },
  minExpected: 3,
}

describe('tierProblems (ADR-0060 §2)', () => {
  it('accepts a well-formed proposal with nothing to refuse', () => {
    expect(tierProblems(estate, valid)).toEqual([])
  })

  it('refuses a proposal with no title', () => {
    expect(tierProblems(estate, { ...valid, title: '  ' })).toContain(
      'the proposal carries no title',
    )
  })

  it('refuses a Tier with no name', () => {
    expect(tierProblems(estate, { ...valid, name: '' })).toContain('the Tier carries no name')
  })

  it('refuses a name outside the lower-case vocabulary', () => {
    expect(tierProblems(estate, { ...valid, name: 'Checkout Gateway' })).toContain(
      'the name Checkout Gateway can use lower-case letters, digits and hyphens only',
    )
  })

  it('refuses a team the tree does not hold', () => {
    expect(tierProblems(estate, { ...valid, team: 'platform-web' })).toContain(
      'the team platform-web is not in the team tree',
    )
  })

  it('refuses an owner the estate does not declare', () => {
    expect(tierProblems(estate, { ...valid, owner: 'nobody' })).toContain(
      'the owner nobody is not on this estate',
    )
  })

  it('refuses an owner outside the owning team', () => {
    expect(tierProblems(estate, { ...valid, owner: 'infosec-lead' })).toContain(
      'the owner infosec-lead is not in the team data-flow',
    )
  })

  it('refuses an environment the estate does not declare', () => {
    expect(tierProblems(estate, { ...valid, environment: 'sandbox' })).toContain(
      'the environment sandbox is not declared on this estate',
    )
  })

  it('refuses a Blueprint the estate does not hold', () => {
    expect(tierProblems(estate, { ...valid, blueprint: 'data-flow/nothing' })).toContain(
      'the Blueprint data-flow/nothing is not on this estate',
    )
  })

  it('refuses a Blueprint pinned at a version it does not sit at', () => {
    expect(tierProblems(estate, { ...valid, blueprintVersion: 3 })).toContain(
      'the Blueprint data-flow/gateway-standard is at version 4, not version 3',
    )
  })

  it('refuses an empty selector', () => {
    expect(tierProblems(estate, { ...valid, selector: {} })).toContain(
      'the selector is empty: keep at least one identity attribute',
    )
  })

  it('refuses a selector pair with no value to match', () => {
    expect(
      tierProblems(estate, { ...valid, selector: { 'telecraft.tier': '' } }),
    ).toContain('the selector key telecraft.tier carries no value: a selector pair needs a string to match')
  })

  it('refuses a minimum expected population below one, or not a whole number', () => {
    const message = 'the minimum expected population must be a whole number of at least 1'
    expect(tierProblems(estate, { ...valid, minExpected: 0 })).toContain(message)
    expect(tierProblems(estate, { ...valid, minExpected: 2.5 })).toContain(message)
    expect(tierProblems(estate, { ...valid, minExpected: 1 })).toEqual([])
  })

  it('refuses a composed id that collides with an existing Tier', () => {
    expect(tierProblems(estate, { ...valid, name: 'payments-gateway' })).toContain(
      'the Tier data-flow/payments-gateway already exists',
    )
  })
})

describe('setupGuidance (ADR-0060 §4)', () => {
  it('fills the guidance in from the Tier, the activated Catalogue version, and the estate settings', () => {
    const guidance = setupGuidance(estate, catalogues.active, 'data-flow/payments-gateway')
    expect(guidance).toEqual({
      tier: 'data-flow/payments-gateway',
      environment: 'production',
      artefactPath: 'rendered/data-flow/payments-gateway.yaml',
      opampEndpoint: 'https://opamp.estate.internal:4320',
      selfTelemetryEndpoint: 'https://otlp.estate.internal:4317',
      identityAttributes: {
        'deployment.environment': 'production',
        'service.namespace': 'payments',
        'telecraft.tier': 'payments-gateway',
      },
      collectorRelease: 'v0.158.0',
    })
  })

  it('answers undefined for a Tier the estate does not hold', () => {
    expect(setupGuidance(estate, catalogues.active, 'nobody/nothing')).toBeUndefined()
  })
})
