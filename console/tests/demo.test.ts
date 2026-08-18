import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { beforeAll, describe, expect, it } from 'vitest'
import { demoApi, DemoWriteError } from '../src/api/demo'

// Demo mode (issue #50): the console answering from a build-time snapshot
// instead of a server. The read paths must project the same documents the
// live API serves — the surfaces consume the contract, never the transport
// — and every write path must terminate at the explanatory notice rather
// than pretending to open a pull request.
//
// The snapshot under test is assembled from the fixture estate, which is
// the same shape `telecraft snapshot` writes; the real one is produced by
// the evaluators over the demo estate.

const load = (name: string) =>
  JSON.parse(readFileSync(fileURLToPath(new URL(`../fixtures/${name}`, import.meta.url)), 'utf8'))

const snapshot = {
  meta: {
    generatedAt: '2026-08-19T09:05:00Z',
    commit: '9c2f4e1a0b3d5f7e9c1a3b5d7f9e1c3a5b7d9f1e',
    repository: 'telecraft-dev/estate-demo',
    evaluatedAt: '2026-08-19T09:00:00Z',
  },
  estate: load('estate.json'),
  catalogues: load('catalogues.json'),
}

beforeAll(() => {
  // One fetch, shared: the demo client loads the snapshot once.
  globalThis.fetch = (async () =>
    new Response(JSON.stringify(snapshot), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })) as typeof fetch
})

describe('demo mode read paths (issue #50)', () => {
  it('derives editableTeams as the signed-in user’s team subtree (ADR-0019 §2)', async () => {
    const me = await demoApi.me()
    expect(me.team).toBe(snapshot.estate.me.team)
    expect(me.editableTeams).toContain(snapshot.estate.me.team)
    // The subtree is the edit horizon, not the whole tree.
    expect(me.editableTeams).not.toContain('engineering')
  })

  it('serves the shelf payload with the ungoverned split derived, never stored', async () => {
    const estate = await demoApi.estate()
    expect(estate.cards).toHaveLength(snapshot.estate.cards.length)
    expect(estate.environments[0]).toBe('production')
    const served = snapshot.estate.collectors.filter(
      (c: { ungoverned?: string }) => c.ungoverned === 'served',
    ).length
    expect(estate.ungoverned.served).toBe(served)
  })

  it('answers a Tier with no drawer honestly rather than inventing findings', async () => {
    const drawer = await demoApi.drawer('nobody/nothing')
    expect(drawer).toEqual({
      contractVersion: 1,
      tier: 'nobody/nothing',
      findings: [],
      provenance: [],
    })
  })

  it('joins the Tier delivery split onto the topology payload', async () => {
    const topology = await demoApi.topology()
    const tier = topology.tiers.find((t) => t.id === 'data-flow/edge')
    expect(tier?.matched).toBeGreaterThan(0)
    expect(tier?.delivery.served + tier!.delivery.git).toBeGreaterThan(0)
  })

  it('refuses an uninstalled catalogue version instead of answering empty', async () => {
    await expect(demoApi.catalogueEntries('v0.0.0')).rejects.toThrow(/no catalogue version/)
  })

  it('runs the one evaluator in the browser, so Compose stays live', async () => {
    const draft = snapshot.estate.blueprints[0]
    const verdict = await demoApi.validate(draft, 'production')
    expect(verdict.yaml).toContain('receivers')
    expect(verdict.palette.entries.length).toBeGreaterThan(0)
    expect(verdict.requirements.length).toBeGreaterThan(0)
  })

  it('previews claim impact in the browser, so the claim flow stays live', async () => {
    const preview = await demoApi.claimPreview({
      selector: { 'deployment.environment': 'production' },
      environment: 'production',
    })
    expect(preview.matched.total).toBeGreaterThanOrEqual(0)
    expect(Array.isArray(preview.candidates)).toBe(true)
  })
})

describe('demo mode write paths end at the notice, never at a pull request', () => {
  it('refuses the composer’s proposal exit', async () => {
    await expect(demoApi.propose()).rejects.toBeInstanceOf(DemoWriteError)
  })

  it('refuses the claim exit in the fail-closed problems shape', async () => {
    const outcome = await demoApi.claim()
    expect(outcome.proposal).toBeUndefined()
    expect(outcome.problems?.[0]).toMatch(/read-only demo/)
  })

  it('refuses a governance proposal in the fail-closed problems shape', async () => {
    const outcome = await demoApi.proposeGovernance()
    expect(outcome.proposal).toBeUndefined()
    expect(outcome.problems?.[0]).toMatch(/read-only demo/)
  })

  it('names the pull-request exit it stands in for, so the demo teaches it', () => {
    const message = new DemoWriteError('saving').message
    expect(message).toContain('pull request')
    expect(message).toContain('ADR-0028')
  })
})
