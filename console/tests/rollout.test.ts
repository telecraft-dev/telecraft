import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import type { FixtureRollout, FixtureRolloutReading } from '../tools/rollout'
import {
  bucket,
  cohortLabel,
  evaluateRollout,
  member,
  rolloutProgress,
  runningArtefact,
} from '../tools/rollout'

// The rollout cohort progress reading (ADR-0029): membership is a pure
// function evaluated fresh on every request, never stored (§4); the
// running reading answers by acknowledged config hash on the served path
// and by the telecraft.tier stamp readings on the foreign path (§7); the
// evaluation blocks passively, aborts past the threshold, and never counts
// foreign lag as failure.

interface FixtureEstate {
  rollouts: FixtureRollout[]
  rolloutReadings: Record<string, FixtureRolloutReading>
  collectors: { id: string; attributes?: Record<string, string> }[]
}

const estate = JSON.parse(
  readFileSync(fileURLToPath(new URL('../fixtures/estate.json', import.meta.url)), 'utf8'),
) as FixtureEstate

// A fixed evaluation instant: 26h into the canary's active stage, past its
// 24h minimum soak, so every verdict below is decided by halts and
// running evidence, never by the wall clock.
const NOW = new Date('2026-08-18T08:00:00Z')

const canary = estate.rollouts[0]!
const trial = estate.rollouts[1]!

describe('cohort membership: a pure function, never stored (ADR-0029 §4)', () => {
  const fractional = {
    hashAttributes: ['host.name'],
    stages: [{ cohort: { percent: 30 } }, { cohort: { percent: 60 } }],
  }
  const hosts = Array.from({ length: 200 }, (_, i) => ({ 'host.name': `node-${i}.internal` }))

  it('is deterministic: identical attributes yield identical membership', () => {
    for (const attrs of hosts) {
      expect(member(fractional, 0, attrs)).toBe(member(fractional, 0, attrs))
      expect(bucket(['host.name'], attrs)).toBe(bucket(['host.name'], attrs))
    }
  })

  it('widens as a strict superset: no collector flaps backwards', () => {
    const narrow = hosts.filter((attrs) => member(fractional, 0, attrs))
    const wide = hosts.filter((attrs) => member(fractional, 1, attrs))
    expect(narrow.length).toBeGreaterThan(0)
    expect(wide.length).toBeGreaterThan(narrow.length)
    for (const attrs of narrow) {
      expect(member(fractional, 1, attrs)).toBe(true)
    }
  })

  it('keeps a collector missing a pinned attribute deterministically outside', () => {
    expect(bucket(['host.name'], { 'k8s.node.name': 'node-1' })).toBeUndefined()
    expect(member(fractional, 1, { 'k8s.node.name': 'node-1' })).toBe(false)
  })

  it('accumulates stages: the active cohort is the union up to the active stage', () => {
    // The canary's stage 0 enumerated host stays a member at stage 1.
    expect(member(canary, 1, { 'service.instance.id': '9a44be71-gw-0' })).toBe(true)
    // A gateway box outside every enumerated set joins at stage 2 (match).
    const late = { 'telecraft.tier': 'gateway', 'service.instance.id': '9a44be71-gw-9' }
    expect(member(canary, 1, late)).toBe(false)
    expect(member(canary, 2, late)).toBe(true)
  })
})

describe('the running reading across both delivery paths (ADR-0029 §7)', () => {
  const artefacts = canary.artefacts

  it('answers exactly by acknowledged config hash on the served path', () => {
    expect(runningArtefact({ remote: { state: 'applied', configHash: artefacts.to.hash } }, artefacts)).toBe('to')
    expect(runningArtefact({ remote: { state: 'applied', configHash: artefacts.from.hash } }, artefacts)).toBe('from')
  })

  it('never reads a FAILED hash as what runs: it names what was refused', () => {
    // gw-1's shape: FAILED for to, self-reverted (ADR-0010); the stamp
    // reading shows it back on from.
    expect(runningArtefact(estate.rolloutReadings['gw-1'], artefacts)).toBe('from')
  })

  it('answers by the telecraft.tier stamp reading on the foreign path', () => {
    expect(runningArtefact(estate.rolloutReadings['gw-2'], artefacts)).toBe('from')
    expect(runningArtefact(estate.rolloutReadings['gws-0'], trial.artefacts)).toBe('to')
  })

  it('reads unknown rather than guessed when the readings cannot tell', () => {
    expect(runningArtefact(undefined, artefacts)).toBe('unknown')
    // Wiring both artefacts share distinguishes nothing.
    const shared = { from: { hash: 'aa', components: ['otlp/in'] }, to: { hash: 'bb', components: ['otlp/in'] } }
    expect(runningArtefact({ stamp: { tier: 't', commit: 'c', components: ['otlp/in'] } }, shared)).toBe('unknown')
  })

  it('reads a config that is neither artefact as other', () => {
    const reading = { stamp: { tier: 't', commit: 'c', components: ['jaeger/legacy'] } }
    expect(runningArtefact(reading, artefacts)).toBe('other')
  })
})

describe('the fixture canary: blocked below the abort threshold (ADR-0029 §6)', () => {
  const progress = evaluateRollout(estate, canary, NOW)

  it('halts passively: one FAILED member withholds the advance, nothing more', () => {
    expect(progress.decision).toBe('blocked')
    expect(progress.halts).toEqual([
      {
        collector: 'gw-1',
        path: 'served',
        condition: 'failed',
        reason: expect.stringContaining('FAILED for the new version') as unknown as string,
      },
    ])
  })

  it('computes the evidence over the active cohort, both paths in one reading', () => {
    expect(progress.evidence).toMatchObject({
      membersSeen: 3,
      runningTo: 1,
      runningFrom: 2,
      runningOther: 0,
      unknown: 0,
      minSoak: '24h',
    })
  })

  it('renders per-cohort progress: entered cohorts accumulate, pending ones preview', () => {
    expect(progress.cohorts.map((c) => c.state)).toEqual(['entered', 'active', 'pending'])
    expect(progress.cohorts.map((c) => c.widens)).toEqual([1, 2, 0])
    // The canary alone, served, running the to artefact.
    expect(progress.cohorts[0]!.served).toMatchObject({ members: 1, to: 1 })
    expect(progress.cohorts[0]!.foreign.members).toBe(0)
    // The active cohort spans both delivery paths: the foreign member is
    // on from, displayed as lag, and its counts sit apart, advisory.
    expect(progress.cohorts[1]!.served).toMatchObject({ members: 2, to: 1, from: 1 })
    expect(progress.cohorts[1]!.foreign).toMatchObject({ members: 1, from: 1 })
  })

  it('carries the authored provenance for the halt state to deep-link into', () => {
    const keys = progress.provenance.map((entry) => entry.key)
    expect(keys).toContain('stage')
    expect(keys).toContain('bindings')
  })
})

describe('the fixture staging trial: abort at the threshold (ADR-0029 §6)', () => {
  const progress = evaluateRollout(estate, trial, NOW)

  it('proposes the abort when the whole cohort went dark after apply', () => {
    expect(progress.decision).toBe('abort')
    expect(progress.halts).toEqual([
      {
        collector: 'gws-0',
        path: 'foreign',
        condition: 'went_dark',
        reason: expect.stringContaining('has not reported since') as unknown as string,
      },
    ])
  })

  it('honours the went-dark signal identically on the foreign path', () => {
    // Delivery status is unavailable there; the halt came from the stamp
    // reading plus silence, never from a FAILED it cannot report (§7).
    expect(estate.rolloutReadings['gws-0']!.remote).toBeUndefined()
  })
})

describe('advance and hold: foreign lag never blocks (ADR-0029 §5, §7)', () => {
  // The canary with its one halt healed: gw-1 acknowledges the to
  // artefact. gw-2, the foreign member, stays on from.
  const healed = structuredClone(estate)
  healed.rolloutReadings['gw-1'] = {
    path: 'served',
    remote: { state: 'applied', configHash: canary.artefacts.to.hash },
  }

  it('advances once the criteria are met, with the foreign member still on from', () => {
    const progress = evaluateRollout(healed, healed.rollouts[0]!, NOW)
    expect(progress.decision).toBe('advance')
    expect(progress.evidence).toMatchObject({ runningTo: 2, runningFrom: 1 })
    expect(progress.halts).toEqual([])
  })

  it('holds while the minimum soak is still running', () => {
    const early = new Date('2026-08-17T07:00:00Z')
    const progress = evaluateRollout(healed, healed.rollouts[0]!, early)
    expect(progress.decision).toBe('hold')
    expect(progress.reason).toContain('of its 24h minimum')
  })

  it('holds until a member is observed actually running the to artefact', () => {
    const unstarted = structuredClone(healed)
    unstarted.rolloutReadings['gw-0'] = {
      path: 'served',
      remote: { state: 'applied', configHash: canary.artefacts.from.hash },
    }
    unstarted.rolloutReadings['gw-1'] = unstarted.rolloutReadings['gw-0']!
    const progress = evaluateRollout(unstarted, unstarted.rollouts[0]!, NOW)
    expect(progress.decision).toBe('hold')
    expect(progress.reason).toContain('running the new version yet')
  })
})

describe('the payload the ledger consumes', () => {
  it('serves every active Rollout, evaluated fresh, nothing persisted', () => {
    const payload = rolloutProgress(estate, NOW)
    expect(payload.map((r) => r.id)).toEqual([
      'data-flow/gateway-canary',
      'data-flow/gateway-staging-trial',
    ])
    expect(payload.map((r) => r.decision)).toEqual(['blocked', 'abort'])
  })

  it('renders cohort specs for reading', () => {
    expect(cohortLabel({ hosts: { attribute: 'service.instance.id', values: ['a', 'b'] } })).toBe(
      'service.instance.id ∈ {a, b}',
    )
    expect(cohortLabel({ match: { 'telecraft.tier': 'gateway' } })).toBe('telecraft.tier=gateway')
    expect(cohortLabel({ percent: 5 })).toBe('5% of the population')
    expect(cohortLabel({ hosts: { attribute: 'host.name', values: ['a'] }, percent: 5 })).toBe(
      'host.name ∈ {a} + 5% of the population',
    )
  })
})
