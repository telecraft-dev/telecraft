import { describe, expect, it } from 'vitest'
import type { BlueprintDoc, EndorsementDoc } from '../src/api/types'
import {
  endorsementFor,
  environmentOptions,
  matchesFilters,
  serviceClassOptions,
  substrateLabel,
  substrateOptions,
} from '../src/surfaces/compose/browse'

// The Blueprints browse view's filter logic (ADR-0061 §3). The two load-
// bearing rules: an undeclared facet never matches a set filter (absent
// means undeclared, never "fits everything", ADR-0061 §1), and the
// endorsed-only filter keeps a stale pin visible (ADR-0061 §2).

function doc(overrides: Partial<BlueprintDoc> & { id: string }): BlueprintDoc {
  return {
    name: overrides.id.split('/').pop() ?? overrides.id,
    version: 1,
    team: 'data-flow',
    locals: {},
    lanes: {},
    extensions: [],
    satisfies: [],
    ...overrides,
  }
}

// Fixture-shaped docs: the same three Blueprints the e2e scenario reads.
const gateway = doc({
  id: 'data-flow/gateway-standard',
  version: 4,
  fits: {
    substrates: ['kubernetes'],
    environments: ['production', 'staging'],
    serviceClasses: ['C1', 'C2'],
  },
})
const edge = doc({
  id: 'data-flow/edge-standard',
  version: 2,
  fits: {
    substrates: ['kubernetes', 'linux'],
    environments: ['production'],
    serviceClasses: ['C1'],
  },
})
const audit = doc({
  id: 'infosec/audit-standard',
  version: 2,
  team: 'infosec',
  fits: {
    substrates: ['linux', 'windows'],
    environments: ['production'],
    serviceClasses: ['C1', 'C2', 'C3'],
  },
})
const undeclared = doc({ id: 'data-flow/legacy-standard' })

const endorsements: EndorsementDoc[] = [
  { blueprint: 'data-flow/gateway-standard', version: 4, owner: 'engineering-lead', team: 'engineering' },
  { blueprint: 'infosec/audit-standard', version: 1, owner: 'engineering-lead', team: 'engineering' },
]

const all = [gateway, edge, audit, undeclared]

describe('matchesFilters', () => {
  it('matches every doc when no filter is set', () => {
    expect(all.filter((bp) => matchesFilters(bp, {}, endorsements))).toEqual(all)
  })

  it('never matches an undeclared facet against a set filter', () => {
    expect(matchesFilters(undeclared, { substrate: 'kubernetes' }, endorsements)).toBe(false)
    expect(matchesFilters(undeclared, { environment: 'production' }, endorsements)).toBe(false)
    expect(matchesFilters(undeclared, { serviceClass: 'C1' }, endorsements)).toBe(false)
  })

  it('never matches one absent facet even when the others are declared', () => {
    const partial = doc({ id: 'data-flow/partial', fits: { substrates: ['kubernetes'] } })
    expect(matchesFilters(partial, { substrate: 'kubernetes' }, endorsements)).toBe(true)
    expect(matchesFilters(partial, { environment: 'production' }, endorsements)).toBe(false)
    expect(matchesFilters(partial, { serviceClass: 'C1' }, endorsements)).toBe(false)
  })

  it('filters by substrate on the declared list', () => {
    const matched = all.filter((bp) => matchesFilters(bp, { substrate: 'linux' }, endorsements))
    expect(matched.map((bp) => bp.id)).toEqual([edge.id, audit.id])
  })

  it('filters by Environment on the declared list', () => {
    const matched = all.filter((bp) => matchesFilters(bp, { environment: 'staging' }, endorsements))
    expect(matched.map((bp) => bp.id)).toEqual([gateway.id])
  })

  it('filters by Service Class on the declared list', () => {
    const matched = all.filter((bp) => matchesFilters(bp, { serviceClass: 'C3' }, endorsements))
    expect(matched.map((bp) => bp.id)).toEqual([audit.id])
  })

  it('combines facet filters: the scenario query keeps gateway and edge', () => {
    const filters = { substrate: 'kubernetes', environment: 'production', serviceClass: 'C1' }
    const matched = all.filter((bp) => matchesFilters(bp, filters, endorsements))
    expect(matched.map((bp) => bp.id)).toEqual([gateway.id, edge.id])
  })

  it('endorsed-only keeps current and stale Endorsements alike', () => {
    const matched = all.filter((bp) => matchesFilters(bp, { endorsedOnly: true }, endorsements))
    expect(matched.map((bp) => bp.id)).toEqual([gateway.id, audit.id])
  })

  it('endorsed-only drops everything while no Endorsements are loaded yet', () => {
    expect(all.filter((bp) => matchesFilters(bp, { endorsedOnly: true }, []))).toEqual([])
  })
})

describe('endorsementFor', () => {
  it('reads a pin at the current version as current', () => {
    expect(endorsementFor(gateway, endorsements)).toEqual({ current: true, version: 4 })
  })

  it('reads a pin behind the current version as stale, version carried', () => {
    expect(endorsementFor(audit, endorsements)).toEqual({ current: false, version: 1 })
  })

  it('returns undefined for a Blueprint nobody endorsed', () => {
    expect(endorsementFor(edge, endorsements)).toBeUndefined()
    expect(endorsementFor(undeclared, endorsements)).toBeUndefined()
  })
})

describe('filter options', () => {
  it('unions declared substrates across docs, sorted and deduplicated', () => {
    expect(substrateOptions(all)).toEqual(['kubernetes', 'linux', 'windows'])
  })

  it('unions declared Environments across docs', () => {
    expect(environmentOptions(all)).toEqual(['production', 'staging'])
  })

  it('unions declared Service Classes across docs', () => {
    expect(serviceClassOptions(all)).toEqual(['C1', 'C2', 'C3'])
  })

  it('offers nothing when no doc declares the facet', () => {
    expect(substrateOptions([undeclared])).toEqual([])
    expect(environmentOptions([undeclared])).toEqual([])
    expect(serviceClassOptions([undeclared])).toEqual([])
  })
})

describe('substrateLabel', () => {
  it('labels the known substrates for display', () => {
    expect(substrateLabel('kubernetes')).toBe('Kubernetes')
    expect(substrateLabel('linux')).toBe('Linux server')
    expect(substrateLabel('windows')).toBe('Windows server')
  })

  it('shows an unknown value as itself', () => {
    expect(substrateLabel('mainframe')).toBe('mainframe')
  })
})
