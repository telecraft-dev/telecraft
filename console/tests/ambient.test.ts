import { describe, expect, it } from 'vitest'
import { CARD_CONTRACT_VERSION } from '../src/api/types'
import type {
  ActivationsPayload,
  Band,
  BandName,
  CardFace,
  EstatePayload,
  TeamNode,
} from '../src/api/types'
import { catalogueReading, elsewhereReadings, standingReading } from '../src/chrome/ambient'

// The context strip's ambient readings (ADR-0062). The strip derives and
// never judges: these tests hold the derivation to the modules that own
// each judgement, and to the lens as evaluation context, so a reading in
// the strip cannot disagree with the surface its door opens.

const okBand: Band = { state: 'ok', worstSeverity: 'none' }

function card(
  tier: string,
  team: string,
  environment: string,
  bands: Partial<Record<BandName, Band>> = {},
  extras: Partial<Pick<CardFace, 'findingCounts' | 'waivedCounts'>> = {},
): CardFace {
  return {
    contractVersion: CARD_CONTRACT_VERSION,
    tier,
    name: tier,
    team,
    environment,
    bands: { delivery: okBand, expectation: okBand, conformance: okBand, ...bands },
    findingCounts: extras.findingCounts ?? {},
    waivedCounts: extras.waivedCounts,
    population: { matched: 1, floorSource: 'absent', state: 'ok' },
    signals: [],
    churn: { known: true, asOf: '2026-08-18T12:00:00Z', incarnations: 0 },
  }
}

const violation: Partial<Record<BandName, Band>> = {
  conformance: { state: 'finding', worstSeverity: 'violation' },
}
const advisory: Partial<Record<BandName, Band>> = {
  delivery: { state: 'finding', worstSeverity: 'advisory' },
}

const teams: TeamNode = {
  id: 'engineering',
  name: 'Engineering',
  teams: [{ id: 'platform', name: 'Platform' }],
}

const settings = {
  opampEndpoint: 'https://opamp.example.internal:4320',
  selfTelemetryEndpoint: 'https://otlp.example.internal:4317',
}

function payload(cards: CardFace[]): EstatePayload {
  return {
    environments: ['production', 'staging'],
    teams,
    cards,
    ungoverned: { served: 0, foreign: 0 },
    settings,
  }
}

describe('standingReading', () => {
  it('reads worst, findings and exempt under the lens Environment only', () => {
    const estate = payload([
      card('platform/gateway', 'platform', 'production', violation, {
        findingCounts: { conformance: 2 },
        waivedCounts: { delivery: 1 },
      }),
      card('platform/edge', 'platform', 'production', advisory, {
        findingCounts: { delivery: 1 },
      }),
      // Staging carries its own finding, and the lens must not read it.
      card('platform/gateway-staging', 'platform', 'staging', violation, {
        findingCounts: { conformance: 3 },
      }),
    ])
    expect(standingReading(estate, 'production')).toEqual({
      worst: 'violation',
      findings: 3,
      exempt: 1,
    })
    expect(standingReading(estate, 'staging')).toEqual({
      worst: 'violation',
      findings: 3,
      exempt: 0,
    })
  })

  it('reads a clean Environment as clear rather than inventing a verdict', () => {
    const estate = payload([card('platform/gateway', 'platform', 'production')])
    expect(standingReading(estate, 'production')).toEqual({
      worst: 'none',
      findings: 0,
      exempt: 0,
    })
  })

  it('keeps the exempt count beside a clean reading, so exemptions never hide', () => {
    const estate = payload([
      card('platform/gateway', 'platform', 'production', {}, { waivedCounts: { delivery: 2 } }),
    ])
    expect(standingReading(estate, 'production')).toEqual({
      worst: 'none',
      findings: 0,
      exempt: 2,
    })
  })
})

describe('catalogueReading', () => {
  const activations = (substrates: ActivationsPayload['substrates']): ActivationsPayload => ({
    substrates,
  })

  it('reads the Catalogue designation and what is on offer', () => {
    const reading = catalogueReading(
      activations([
        {
          kind: 'catalogue',
          name: 'Catalogue',
          active: 'v0.158.0',
          candidates: [{ version: 'v0.155.0', summary: '1 component in use is removed', lines: [] }],
          history: [],
        },
        {
          kind: 'schema_registry',
          name: 'Schema Registry',
          active: '',
          candidates: [],
          history: [],
        },
      ]),
    )
    expect(reading).toEqual({ active: 'v0.158.0', onOffer: ['v0.155.0'] })
  })

  it('reads nothing when no Catalogue substrate is declared', () => {
    expect(catalogueReading(activations([]))).toBeUndefined()
  })
})

describe('elsewhereReadings', () => {
  it('reads every non-lens Environment, and never the lens itself', () => {
    const estate = payload([
      card('platform/gateway', 'platform', 'production', violation, {
        findingCounts: { conformance: 2 },
      }),
      card('platform/gateway-staging', 'platform', 'staging', advisory, {
        findingCounts: { delivery: 1 },
      }),
    ])
    expect(elsewhereReadings(estate, 'production')).toEqual([
      { environment: 'staging', findings: 1 },
    ])
    expect(elsewhereReadings(estate, 'staging')).toEqual([
      { environment: 'production', findings: 2 },
    ])
  })

  it('reads a quiet Environment as zero findings rather than dropping it', () => {
    const estate = payload([card('platform/gateway', 'platform', 'production')])
    expect(elsewhereReadings(estate, 'production')).toEqual([
      { environment: 'staging', findings: 0 },
    ])
  })
})
