import { describe, expect, it } from 'vitest'
import contract from '../fixtures/card-contract.json'
import estate from '../fixtures/estate.json'
import type { CardDrawer, CardFace } from '../src/api/types'
import { BAND_ORDER, CARD_CONTRACT_VERSION } from '../src/api/types'
import {
  SIGNAL_ORDER,
  formatChurn,
  formatFreshness,
  formatShape,
  formatVolume,
  readingTitle,
} from '../src/estate/readings'

// The card contract, from the console's side (ADR-0041 §4). The fixture
// this reads is written by the engine's own test —
// `go test ./internal/card -update` — so one artefact holds both sides:
// a field added or renamed in Go without the console following is a
// failing test here, and the version number is what makes the change a
// reviewable event rather than silent drift.

const face = (contract as { face: CardFace }).face
const drawer = (contract as { drawer: CardDrawer }).drawer

const BAND_STATES = [
  'ok',
  'finding',
  'not_applicable',
  'unknown',
  'pending_settle',
  'stale_demoted',
]

describe('the card data contract', () => {
  it('is versioned, and both payloads carry the same version', () => {
    expect(face.contractVersion).toBe(CARD_CONTRACT_VERSION)
    expect(drawer.contractVersion).toBe(CARD_CONTRACT_VERSION)
  })

  it('keys on the Tier id, never on a Tier and Environment pair', () => {
    expect(face.tier).toBe(drawer.tier)
    expect(face.environment).toBeTruthy()
    expect(face.tier).not.toContain(face.environment)
  })

  it('carries the three bands in fixed order, as enum states', () => {
    for (const band of BAND_ORDER) {
      expect(face.bands[band]).toBeDefined()
      expect(BAND_STATES).toContain(face.bands[band].state)
    }
  })

  // P4 rule 2, held from this side too: nothing in the payload names a
  // colour, so a renderer cannot make hue load-bearing by reading a
  // field. Band position and glyph are the contract.
  it('makes hue underivable from the payload', () => {
    const hue = /\b(colours?|colors?|hue|red|amber|green|rgb|hsl)\b|#[0-9a-f]{3,8}\b/i
    expect(JSON.stringify(contract)).not.toMatch(hue)
  })

  it('carries one matrix row per signal lane, each reading its own as-of and Known', () => {
    expect(face.signals.map((row) => row.signal)).toEqual([...SIGNAL_ORDER])
    for (const row of face.signals) {
      for (const reading of [row.volume, row.freshness, row.shape]) {
        expect(typeof reading.known).toBe('boolean')
        expect(Date.parse(reading.asOf)).not.toBeNaN()
        if (!reading.known) expect(reading.cause).toBeTruthy()
      }
    }
  })

  it('renders every row the engine can produce, including the unreadable ones', () => {
    for (const row of face.signals) {
      expect(() => formatVolume(row.volume)).not.toThrow()
      expect(() => formatFreshness(row.freshness)).not.toThrow()
      expect(() => formatShape(row.shape)).not.toThrow()
      if (!row.volume.known) {
        // An unreadable lane is last-known-plus-age, never a zero.
        expect(formatVolume(row.volume)).toBe('—')
        expect(readingTitle(row.volume)).toContain(row.volume.cause ?? '')
      }
    }
    expect(() => formatChurn(face.churn)).not.toThrow()
  })

  it('carries the population line ADR-0035 produces, floor source included', () => {
    expect(['derived', 'declared', 'absent']).toContain(face.population.floorSource)
    expect(['ok', 'never_seen', 'under_populated']).toContain(face.population.state)
    expect(typeof face.population.matched).toBe('number')
  })

  it('carries the shelf summary fields, so the shelf never fetches a drawer to sort', () => {
    expect(face.team).toBeTruthy()
    expect(Object.keys(face.findingCounts).length).toBeGreaterThan(0)
    expect(face.waivedCounts).toBeDefined()
  })

  it('gives every drawer finding a remediation and somewhere to act', () => {
    expect(drawer.findings.length).toBeGreaterThan(0)
    for (const finding of drawer.findings) {
      expect(finding.remediation).toBeTruthy()
      expect(finding.whoActs.label).toBeTruthy()
      expect(finding.whoActs.target.id).toBeTruthy()
    }
  })

  it('feeds every "why?" as structured provenance, never as prose to reconstruct', () => {
    for (const entry of drawer.provenance) {
      expect(entry.key).toBeTruthy()
      expect(entry.claim).toBeTruthy()
      expect(entry.sha).toBeTruthy()
      expect(entry.lines.length).toBeGreaterThan(0)
    }
  })
})

describe('the fixture estate', () => {
  it('serves faces at the same contract version the console expects', () => {
    for (const card of estate.cards as unknown as CardFace[]) {
      expect(card.contractVersion).toBe(CARD_CONTRACT_VERSION)
      expect(card.signals.map((row) => row.signal)).toEqual([...SIGNAL_ORDER])
      expect(card.churn).toBeDefined()
      expect(card.population.state).toBeTruthy()
    }
  })
})
