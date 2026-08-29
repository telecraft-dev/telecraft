import { describe, expect, it } from 'vitest'
import { CARD_CONTRACT_VERSION } from '../src/api/types'
import type {
  Band,
  BandName,
  CardFace,
  EstatePayload,
  RolloutDecision,
  RolloutProgress,
  SignalRow,
  TeamNode,
} from '../src/api/types'
import {
  needsAttention,
  ROLLOUTS_SHOWN,
  rolloutPosition,
  rolloutWaiting,
  STANDING_TIERS_NAMED,
  summarise,
  teamStanding,
  tierDetail,
  ungovernedTotal,
  WORST_TIERS_SHOWN,
} from '../src/home/summary'

// Home's derivation (ADR-0056). The surface judges nothing: these tests
// hold it to that, and to the two rules that stop a landing page lying.
// Nothing blends (§3, ADR-0017), and every bound reports what it dropped
// (§5), because a truncation that does not say it truncated reads as an
// all-clear.

const okBand: Band = { state: 'ok', worstSeverity: 'none' }

function card(
  tier: string,
  team: string,
  environment: string,
  bands: Partial<Record<BandName, Band>> = {},
  extras: Partial<Pick<CardFace, 'findingCounts' | 'waivedCounts' | 'signals'>> = {},
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
    signals: extras.signals ?? [],
    churn: { known: true, asOf: '2026-08-18T12:00:00Z', incarnations: 0 },
  }
}

const violation = (kind: BandName = 'conformance') => ({
  [kind]: { state: 'finding', worstSeverity: 'violation' } as Band,
})
const advisory = (kind: BandName = 'delivery') => ({
  [kind]: { state: 'finding', worstSeverity: 'advisory' } as Band,
})

const teams: TeamNode = {
  id: 'engineering',
  name: 'Engineering',
  teams: [
    { id: 'platform', name: 'Platform', teams: [{ id: 'data-flow', name: 'Data Flow' }] },
    { id: 'infosec', name: 'InfoSec' },
  ],
}

// The declared estate settings (ADR-0060 §4): the summary never reads
// them, but the payload carries them.
const settings = {
  opampEndpoint: 'https://opamp.example.internal:4320',
  selfTelemetryEndpoint: 'https://otlp.example.internal:4317',
}

function payload(cards: CardFace[], ungoverned = { served: 0, foreign: 0 }): EstatePayload {
  return { environments: ['production', 'staging'], teams, cards, ungoverned, settings }
}

function rollout(
  id: string,
  decision: RolloutDecision,
  overrides: Partial<RolloutProgress> = {},
): RolloutProgress {
  return {
    id,
    name: id,
    team: 'data-flow',
    owner: 'someone',
    tier: 'data-flow/gateway',
    tierName: 'gateway',
    environment: 'production',
    from: 'data-flow/standard@1',
    to: 'data-flow/standard@2',
    stage: 0,
    decision,
    reason: `${decision} reason`,
    evidence: {} as RolloutProgress['evidence'],
    cohorts: [],
    halts: [],
    provenance: [],
    ...overrides,
  }
}

describe('needsAttention', () => {
  it('is true exactly when the card carries a finding', () => {
    expect(needsAttention(card('a', 'data-flow', 'production', violation()))).toBe(true)
    expect(needsAttention(card('b', 'data-flow', 'production', advisory()))).toBe(true)
    expect(needsAttention(card('c', 'data-flow', 'production'))).toBe(false)
  })

  it('does not treat a neutral card as wanting attention', () => {
    // A neutral is not a finding, and Home must not manufacture one: an
    // `unknown` band means no verdict was given, not that a bad one was.
    const neutral = card('d', 'data-flow', 'production', {
      delivery: { state: 'unknown', worstSeverity: 'none' },
      expectation: { state: 'not_applicable', worstSeverity: 'none' },
      conformance: { state: 'pending_settle', worstSeverity: 'none' },
    })
    expect(needsAttention(neutral)).toBe(false)
  })
})

describe('summarise: the estate standing', () => {
  it('is the roll-up root row, so Home and the tree-table cannot disagree', () => {
    const cards = [
      card(
        'data-flow/gateway',
        'data-flow',
        'production',
        violation(),
        { findingCounts: { conformance: 1 }, waivedCounts: { conformance: 1 } },
      ),
      card('data-flow/edge', 'data-flow', 'production'),
      card('infosec/audit', 'infosec', 'staging', advisory(), {
        findingCounts: { delivery: 2 },
      }),
    ]
    const summary = summarise(payload(cards), [], 'production')

    expect(summary.standing.team.id).toBe('engineering')
    // Ratio plus worst, per kind, never blended (ADR-0017).
    expect(summary.standing.kinds.conformance).toEqual({
      passing: 1,
      counted: 2,
      worst: 'violation',
      waived: 1,
    })
    expect(summary.standing.kinds.delivery.worst).toBe('none')
    expect(summary.standing.tiersInEnvironment).toBe(2)
    expect(summary.standing.tiersTotal).toBe(3)
    // The all-Environments companion, so the lens conceals no finding.
    expect(summary.standing.findingsAllEnvironments).toBe(3)
    expect(summary.standing.waivedAllEnvironments).toBe(1)
  })

  it('exposes no blended score of any kind', () => {
    // The one thing this surface must never grow (ADR-0056 §3). If a
    // percentage or a single health number is ever added, it will be added
    // to this object, and this test is where it is caught.
    const summary = summarise(payload([card('a', 'data-flow', 'production')]), [], 'production')
    const keys = Object.keys(summary)
    expect(keys).not.toContain('score')
    expect(keys).not.toContain('health')
    expect(keys).not.toContain('percentage')
    expect(keys).not.toContain('compliance')
  })
})

describe('summarise: the bounded worst-Tier list', () => {
  const many = Array.from({ length: WORST_TIERS_SHOWN + 3 }, (_, i) =>
    card(`data-flow/t${i}`, 'data-flow', 'production', violation(), {
      findingCounts: { conformance: 1 },
    }),
  )

  it('draws at most the bound and counts the rest', () => {
    const summary = summarise(payload(many), [], 'production')
    expect(summary.worstTiers).toHaveLength(WORST_TIERS_SHOWN)
    expect(summary.attentionInLens).toBe(WORST_TIERS_SHOWN + 3)
  })

  it('orders worst-first, in the shelf order, not an order of its own', () => {
    const cards = [
      card('data-flow/quiet', 'data-flow', 'production'),
      card('data-flow/warn', 'data-flow', 'production', advisory(), {
        findingCounts: { delivery: 1 },
      }),
      card('data-flow/bad', 'data-flow', 'production', violation(), {
        findingCounts: { conformance: 5 },
      }),
    ]
    const summary = summarise(payload(cards), [], 'production')
    expect(summary.worstTiers.map((c) => c.tier)).toEqual([
      'data-flow/bad',
      'data-flow/warn',
    ])
  })

  it('counts findings in other Environments rather than hiding them', () => {
    // The lens is evaluation context, never a filter (ADR-0042 §4). A Tier
    // failing only in staging is absent from a production-lensed list, and
    // saying nothing about it would make the lens a place to hide.
    const cards = [
      card('data-flow/gateway', 'data-flow', 'production', violation(), {
        findingCounts: { conformance: 1 },
      }),
      card('data-flow/staging', 'data-flow', 'staging', violation(), {
        findingCounts: { conformance: 1 },
      }),
      card('infosec/staging', 'infosec', 'staging', advisory(), {
        findingCounts: { delivery: 1 },
      }),
    ]
    const summary = summarise(payload(cards), [], 'production')
    expect(summary.attentionInLens).toBe(1)
    expect(summary.attentionElsewhere).toBe(2)
  })
})

describe('summarise: Teams', () => {
  it('partitions by top-level subtree, each aggregating its whole subtree', () => {
    // `data-flow` sits under `platform`, so its findings must reach the
    // Platform row and must not appear as a row of their own: overlapping
    // rows would count one finding twice on one screen.
    const cards = [
      card('data-flow/gateway', 'data-flow', 'production', violation(), {
        findingCounts: { conformance: 1 },
      }),
      card('infosec/audit', 'infosec', 'production'),
    ]
    const summary = summarise(payload(cards), [], 'production')
    expect(summary.teams.map((row) => row.team.id)).toEqual(['platform', 'infosec'])
    expect(summary.teamsTotal).toBe(2)
    expect(summary.teams[0]!.findingsAllEnvironments).toBe(1)
  })

  it('sorts worst-first, so the Team to look at leads', () => {
    const cards = [
      card('data-flow/ok', 'data-flow', 'production'),
      card('infosec/bad', 'infosec', 'production', violation(), {
        findingCounts: { conformance: 1 },
      }),
    ]
    const summary = summarise(payload(cards), [], 'production')
    expect(summary.teams.map((row) => row.team.id)).toEqual(['infosec', 'platform'])
    expect(teamStanding(summary.teams[0]!)).toBe('violation')
    expect(teamStanding(summary.teams[1]!)).toBe('none')
  })

  it('falls back to the root when the tree has no children', () => {
    const flat: TeamNode = { id: 'solo', name: 'Solo' }
    const summary = summarise(
      { environments: ['production'], teams: flat, cards: [], ungoverned: { served: 0, foreign: 0 }, settings },
      [],
      'production',
    )
    expect(summary.teams.map((row) => row.team.id)).toEqual(['solo'])
  })
})

describe('summarise: Rollouts', () => {
  it('draws the verdicts waiting on a person and counts the steady ones', () => {
    // `hold` is the steady state: nothing there needs doing, so it is a
    // number rather than a row (ADR-0029 §5).
    const rollouts = [
      rollout('a', 'hold'),
      rollout('b', 'advance'),
      rollout('c', 'blocked'),
      rollout('d', 'abort'),
    ]
    const summary = summarise(payload([]), rollouts, 'production')
    expect(summary.rollouts.map((r) => r.id)).toEqual(['d', 'c', 'b'])
    expect(summary.rolloutsWaiting).toBe(3)
    expect(summary.rolloutsSteady).toBe(1)
  })

  it('bounds the list and counts the overflow', () => {
    const rollouts = Array.from({ length: ROLLOUTS_SHOWN + 2 }, (_, i) =>
      rollout(`r${i}`, 'blocked'),
    )
    const summary = summarise(payload([]), rollouts, 'production')
    expect(summary.rollouts).toHaveLength(ROLLOUTS_SHOWN)
    expect(summary.rolloutsWaiting).toBe(ROLLOUTS_SHOWN + 2)
  })

  it('reads an absent Rollouts payload as none running, never as an error', () => {
    // The demo snapshot may carry no Rollouts at all, and Home renders
    // beside that rather than withholding the rest of the page.
    const summary = summarise(payload([]), [], 'production')
    expect(summary.rollouts).toEqual([])
    expect(summary.rolloutsWaiting).toBe(0)
    expect(summary.rolloutsSteady).toBe(0)
  })

  it('treats every non-hold verdict as waiting', () => {
    expect(rolloutWaiting(rollout('a', 'hold'))).toBe(false)
    for (const decision of ['advance', 'blocked', 'abort'] as RolloutDecision[]) {
      expect(rolloutWaiting(rollout('a', decision))).toBe(true)
    }
  })
})

describe('summarise: the standing tiles name their Tiers', () => {
  const noisy = card('data-flow/gateway', 'data-flow', 'production', violation('conformance'), {
    findingCounts: { conformance: 2 },
  })
  const warned = card('data-flow/edge', 'data-flow', 'production', advisory('conformance'), {
    findingCounts: { conformance: 1 },
  })
  const quiet = card('infosec/audit', 'infosec', 'production')
  const neutral = card('data-flow/payments', 'data-flow', 'production', {
    delivery: { state: 'unknown', worstSeverity: 'none' },
    expectation: { state: 'unknown', worstSeverity: 'none' },
    conformance: { state: 'unknown', worstSeverity: 'none' },
  })

  it('a kind carrying findings names exactly the Tiers with them, worst first', () => {
    const summary = summarise(payload([quiet, warned, noisy, neutral]), [], 'production')
    expect(summary.standingTiers.conformance.shown.map((c) => c.tier)).toEqual([
      'data-flow/gateway',
      'data-flow/edge',
    ])
    expect(summary.standingTiers.conformance.more).toBe(0)
  })

  it('a clean kind names the Tiers its ratio counted', () => {
    // Delivery is clean on every verdict-bearing card, so the tile names
    // them all; the neutral card joins no kind's names, as it joins no
    // kind's denominator.
    const summary = summarise(payload([quiet, warned, noisy, neutral]), [], 'production')
    expect(summary.standingTiers.delivery.shown.map((c) => c.tier)).toEqual([
      'data-flow/gateway',
      'data-flow/edge',
      'infosec/audit',
    ])
  })

  it('bounds the names and counts the rest (ADR-0056 §5)', () => {
    const many = Array.from({ length: STANDING_TIERS_NAMED + 2 }, (_, i) =>
      card(`data-flow/t${i}`, 'data-flow', 'production', violation('delivery'), {
        findingCounts: { delivery: 1 },
      }),
    )
    const summary = summarise(payload(many), [], 'production')
    expect(summary.standingTiers.delivery.shown).toHaveLength(STANDING_TIERS_NAMED)
    expect(summary.standingTiers.delivery.more).toBe(2)
  })

  it('names the neutral Tiers behind the neutral count', () => {
    const summary = summarise(payload([quiet, neutral]), [], 'production')
    expect(summary.neutralTiers.shown.map((c) => c.tier)).toEqual(['data-flow/payments'])
    expect(summary.standing.neutral).toBe(summary.neutralTiers.shown.length)
  })
})

describe('tierDetail', () => {
  const asOf = '2026-08-18T12:00:00Z'
  function lane(signal: SignalRow['signal'], over: Partial<SignalRow> = {}): SignalRow {
    return {
      signal,
      lane: 'present',
      volume: {
        known: true,
        asOf,
        in: 100,
        out: 100,
        reduction: 0,
        refused: 0,
        sendFailed: 0,
        enqueueFailed: 0,
      },
      freshness: { known: true, asOf, ageSeconds: 30 },
      shape: { known: true, asOf, required: 2, missing: 0 },
      ...over,
    }
  }

  it('leads with the worst finding band, preferring the face label the card carries', () => {
    const face = card('data-flow/gateway', 'data-flow', 'production', {
      expectation: { state: 'finding', worstSeverity: 'advisory' },
      conformance: {
        state: 'finding',
        worstSeverity: 'violation',
        worstFinding: 'checkout misses the C1 floor in production',
      },
    })
    expect(tierDetail(face)[0]).toEqual({ lead: 'checkout misses the C1 floor in production' })
  })

  it('falls back to the kind name when the face carries no label', () => {
    const face = card('data-flow/edge', 'data-flow', 'production', advisory('conformance'))
    expect(tierDetail(face)).toEqual([{ lead: 'Conformance: finding' }])
  })

  it('reads the lane facts off the face, in lane order', () => {
    const face = card('data-flow/gateway', 'data-flow', 'production', violation('conformance'), {
      signals: [
        lane('logs', { freshness: { known: true, asOf, silent: true } }),
        lane('metrics', {
          volume: {
            known: true,
            asOf,
            in: 1000,
            out: 900,
            reduction: 0,
            refused: 100,
            sendFailed: 0,
            enqueueFailed: 0,
          },
          shape: { known: true, asOf, required: 2, missing: 1 },
        }),
        { signal: 'traces', lane: 'not_applicable' },
      ],
    })
    // Each lane fact keeps its lane's name apart from the words around
    // it, so the row can colour the name and nothing else (ADR-0047 §5).
    expect(tierDetail(face)).toEqual([
      { lead: 'Conformance: finding' },
      { lead: '', signal: 'logs', trail: ' silent' },
      { lead: '1 of 2 ', signal: 'metrics', trail: ' missing' },
      { lead: '100 ', signal: 'metrics', trail: ' refused' },
      { lead: 'no ', signal: 'traces', trail: ' lane on this Tier' },
    ])
  })

  it('reads back as the line the row draws', () => {
    const face = card('data-flow/gateway', 'data-flow', 'production', {}, {
      signals: [
        lane('logs', { freshness: { known: true, asOf, silent: true } }),
        { signal: 'traces', lane: 'not_applicable' },
      ],
    })
    expect(
      tierDetail(face)
        .map((segment) => `${segment.lead}${segment.signal ?? ''}${segment.trail ?? ''}`)
        .join(' · '),
    ).toBe('logs silent · no traces lane on this Tier')
  })

  it('is empty for a healthy card with quiet lanes', () => {
    const face = card('data-flow/ok', 'data-flow', 'production', {}, {
      signals: [lane('logs'), lane('metrics'), lane('traces')],
    })
    expect(tierDetail(face)).toEqual([])
  })

  it('survives a wired lane that has no readings yet', () => {
    // A never_seen Tier's rows carry a lane and nothing else: no reading
    // is not a fact to report, so the line stays empty rather than lying.
    const face = card('data-flow/new', 'data-flow', 'production', {}, {
      signals: [{ signal: 'logs', lane: 'present' }],
    })
    expect(tierDetail(face)).toEqual([])
  })
})

describe('rolloutPosition', () => {
  const split = { members: 0, to: 0, from: 0, other: 0, unknown: 0 }
  function cohorts(n: number): RolloutProgress['cohorts'] {
    return Array.from({ length: n }, (_, index) => ({
      index,
      cohort: `cohort-${index}`,
      soak: '24h',
      state: 'entered' as const,
      widens: 0,
      served: split,
      foreign: split,
    }))
  }
  const evidence = {
    membersSeen: 3,
    runningTo: 1,
    runningFrom: 2,
    runningOther: 0,
    unknown: 0,
    soaked: '36h',
    minSoak: '24h',
  }

  it('carries the stage, the running split, and the halted member by name', () => {
    const progressing = rollout('gateway-canary', 'blocked', {
      stage: 1,
      cohorts: cohorts(3),
      evidence,
      halts: [
        { collector: 'gw-1', path: 'served', condition: 'failed', reason: 'apply failed' },
      ],
    })
    expect(rolloutPosition(progressing)).toEqual([
      'stage 2 of 3',
      '1 of 3 on the new version',
      'gw-1 apply failed',
    ])
  })

  it('words the halt conditions as the ledger does, spacing out unmapped ones', () => {
    const dark = rollout('trial', 'abort', {
      halts: [{ collector: 'gws-0', path: 'foreign', condition: 'went_dark', reason: 'dark' }],
    })
    expect(rolloutPosition(dark)).toEqual(['gws-0 went dark'])
    const odd = rollout('trial', 'abort', {
      halts: [{ collector: 'gws-1', path: 'foreign', condition: 'ran_hot', reason: 'hot' }],
    })
    expect(rolloutPosition(odd)).toEqual(['gws-1 ran hot'])
  })

  it('counts halts beyond the first rather than naming them all (ADR-0056 §5)', () => {
    const many = rollout('canary', 'blocked', {
      halts: [
        { collector: 'gw-1', path: 'served', condition: 'failed', reason: 'a' },
        { collector: 'gw-2', path: 'served', condition: 'failed', reason: 'b' },
        { collector: 'gw-3', path: 'served', condition: 'went_dark', reason: 'c' },
      ],
    })
    expect(rolloutPosition(many)).toEqual(['gw-1 apply failed', 'and 2 more halted'])
  })

  it('stays empty when the payload carries no position to report', () => {
    expect(rolloutPosition(rollout('bare', 'advance'))).toEqual([])
  })
})

describe('summarise: the ungoverned population', () => {
  it('carries both referents through, and their total', () => {
    // Served and foreign are distinct referents (ADR-0030, ADR-0031) and
    // stay distinct; the total is for the door's label, not a merge.
    const summary = summarise(payload([], { served: 2, foreign: 5 }), [], 'production')
    expect(summary.ungoverned).toEqual({ served: 2, foreign: 5 })
    expect(ungovernedTotal(summary.ungoverned)).toBe(7)
  })

  it('keeps the ungoverned out of every standing ratio', () => {
    // They appear in no compliance denominator (ADR-0031): a large
    // ungoverned population must not move a single ratio on this page.
    const cards = [card('data-flow/gateway', 'data-flow', 'production')]
    const without = summarise(payload(cards), [], 'production')
    const with_ = summarise(payload(cards, { served: 40, foreign: 60 }), [], 'production')
    expect(with_.standing.kinds).toEqual(without.standing.kinds)
    expect(with_.standing.tiersTotal).toBe(without.standing.tiersTotal)
  })
})
