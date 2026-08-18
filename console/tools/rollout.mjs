// The fixture rollout reading (ADR-0029): cohort membership as a pure
// function, the advisory running-artefact reading across both delivery
// paths, and the halt/advance evaluation — the same semantics as
// internal/rollout, run over the fixture estate. Membership is never
// stored (§4): this module computes it per request from the authored
// Rollout and the reported identifying attributes, exactly as the server
// computes it per connect. The Foreign population reads everything and
// blocks nothing (§7): the served path answers by acknowledged config
// hash; the foreign path by the telecraft.tier stamp readings —
// self-telemetry component identity under the Tier stamp (ADR-0039 §5) —
// and a member still on the *from* artefact is lag, never failure.

import { createHash } from 'node:crypto'

// Fractional membership is judged per 1/100th of a percent, mirroring
// internal/rollout (§4): integer percents are exact over the hash space.
const BUCKETS = 10000

/** The ADR-0029 §6 default: at or past this halted share, propose abort. */
const DEFAULT_ABORT_FRACTION = 0.1

// ---- Membership: the pure function (ADR-0029 §4) -------------------------

/**
 * Hashes the pinned identifying attributes to [0, BUCKETS), mirroring the
 * Go bucket function: the pinned set and its authored order are part of
 * the function's identity. A collector missing a pinned attribute has no
 * node-stable identity to hash and is deterministically outside.
 */
export function bucket(keys, attrs) {
  if (!keys || keys.length === 0) return undefined
  const h = createHash('sha256')
  for (const key of keys) {
    const value = attrs[key]
    if (value === undefined || value === '') return undefined
    h.update(key)
    h.update(Buffer.from([0]))
    h.update(value)
    h.update(Buffer.from([0]))
  }
  const sum = h.digest()
  return Number(sum.readBigUInt64BE(0) % BigInt(BUCKETS))
}

/** Equality over every asked pair — the Tier selector's semantics (ADR-0007). */
function satisfies(selector, attrs) {
  return Object.entries(selector).every(([key, value]) => attrs[key] === value)
}

/** Judges one stage's cohort spec: the three forms are mixable, membership their union. */
function specMember(cohort, hashAttributes, attrs) {
  if (cohort.hosts) {
    const value = attrs[cohort.hosts.attribute]
    if (value !== undefined && cohort.hosts.values.includes(value)) return true
  }
  if (cohort.match && Object.keys(cohort.match).length > 0 && satisfies(cohort.match, attrs)) {
    return true
  }
  if (cohort.percent > 0) {
    const b = bucket(hashAttributes, attrs)
    if (b !== undefined && b < cohort.percent * (BUCKETS / 100)) return true
  }
  return false
}

/**
 * Reports whether reported identifying attributes fall in the cohort of
 * stages 0..stage: the union, so advancing only ever widens and no
 * collector flaps backwards (§4). Identical inputs yield identical
 * membership anywhere — server, CI, and this preview alike.
 */
export function member(rollout, stage, attrs) {
  for (let i = 0; i <= stage && i < rollout.stages.length; i++) {
    if (specMember(rollout.stages[i].cohort, rollout.hashAttributes ?? [], attrs)) return true
  }
  return false
}

// ---- The advisory running reading (ADR-0029 §7) --------------------------

/**
 * Decides which artefact one member runs. The served path answers by
 * acknowledged config hash — only an APPLIED acknowledgement names what
 * runs; a FAILED reading's hash names what was refused, the collector has
 * self-reverted (ADR-0010). The foreign path answers by the stamp
 * readings: the component identities observed in self-telemetry under the
 * Tier's telecraft.tier stamp (ADR-0039 §5), compared against each
 * artefact's rendered component set. Identity both artefacts share
 * distinguishes nothing and reads unknown rather than guessed.
 */
export function runningArtefact(reading, artefacts) {
  if (reading?.remote?.state === 'applied' && reading.remote.configHash) {
    if (reading.remote.configHash === artefacts.to.hash) return 'to'
    if (reading.remote.configHash === artefacts.from.hash) return 'from'
    // An unrecognised hash is another artefact, but fresher stamp
    // readings may have arrived since — fall through.
  }
  const observed = reading?.stamp?.components
  if (!observed) return 'unknown'
  const seen = new Set(observed)
  const matchesTo = setEqual(seen, artefacts.to.components)
  const matchesFrom = setEqual(seen, artefacts.from.components)
  if (matchesTo && matchesFrom) return 'unknown'
  if (matchesTo) return 'to'
  if (matchesFrom) return 'from'
  return 'other'
}

function setEqual(seen, components) {
  return components.length === seen.size && components.every((id) => seen.has(id))
}

// ---- Halt conditions (ADR-0029 §6, explicitly extensible) ----------------

const CONDITIONS = [
  {
    // v1 signal (a): FAILED for the *to* artefact's hash — the apply
    // failed and the Supervisor has already self-reverted (ADR-0010). A
    // FAILED for any other hash is some other delivery's problem.
    name: 'failed',
    halt: (observation, artefacts) => {
      const remote = observation.reading?.remote
      if (remote?.state === 'failed' && remote.configHash === artefacts.to.hash) {
        return `reported FAILED for the to artefact: ${remote.error ?? '(no error detail reported)'}`
      }
      return undefined
    },
  },
  {
    // v1 signal (b): went dark after apply — took the new config, then
    // silent past the staleness horizon: the crash-loop signature that
    // never reports FAILED.
    name: 'went_dark',
    halt: (observation) => {
      if (observation.reading?.silent && observation.running === 'to') {
        return 'took the to artefact, then went silent past the staleness horizon'
      }
      return undefined
    },
  },
]

// ---- Soak arithmetic -----------------------------------------------------

const UNIT_MS = { h: 3_600_000, m: 60_000, s: 1_000 }

/** Parses an authored duration like `24h`, `90m` or `1h30m`. */
function parseSoak(soak) {
  let ms = 0
  for (const [, count, unit] of (soak ?? '').matchAll(/(\d+)([hms])/g)) {
    ms += Number(count) * UNIT_MS[unit]
  }
  return ms
}

function formatDuration(ms) {
  if (ms < UNIT_MS.h) return `${Math.round(ms / UNIT_MS.m)}m`
  return `${Math.round(ms / UNIT_MS.h)}h`
}

/** Renders one cohort spec for reading — the ledger's spec column. */
export function cohortLabel(cohort) {
  const parts = []
  if (cohort.hosts) parts.push(`${cohort.hosts.attribute} ∈ {${cohort.hosts.values.join(', ')}}`)
  if (cohort.match && Object.keys(cohort.match).length > 0) {
    parts.push(
      Object.entries(cohort.match)
        .map(([key, value]) => `${key}=${value}`)
        .join(', '),
    )
  }
  if (cohort.percent > 0) parts.push(`${cohort.percent}% of the population`)
  return parts.join(' + ')
}

// ---- The evaluation (ADR-0029 §5, §6) ------------------------------------

/**
 * The target Tier's population: the collectors satisfying its authored
 * selector, each with its rollout reading (`rolloutReadings`, keyed by
 * collector id — path, acknowledged hash, stamp reading, silence).
 */
function population(estate, rollout) {
  const selector = estate.selectors[rollout.tier] ?? {}
  return estate.collectors
    .filter((collector) => satisfies(selector, collector.attributes ?? {}))
    .map((collector) => ({
      collector,
      reading: estate.rolloutReadings?.[collector.id],
    }))
}

function emptySplit() {
  return { members: 0, to: 0, from: 0, other: 0, unknown: 0 }
}

/**
 * Evaluates one Rollout over the fixture estate: a pure function of its
 * inputs, mirroring internal/rollout Evaluate — the decision order is
 * abort, blocked, soak, no-evidence, advance. It proposes nothing itself:
 * hold and blocked are the passive states, a withheld proposal with no
 * control loop.
 */
export function evaluateRollout(estate, rollout, now) {
  const pop = population(estate, rollout)
  const minSoakMs = parseSoak(rollout.stages[rollout.stage]?.soak)
  const soakedMs = Math.max(0, now.getTime() - Date.parse(rollout.stageStarted))

  // Per-cohort progress: each stage's cumulative membership (stages
  // 0..index — entered cohorts accumulate, §4), split by delivery path.
  // Pending stages are the membership preview: information for the
  // reviewer, never the authoritative decision.
  const cohorts = rollout.stages.map((stage, index) => {
    const split = { served: emptySplit(), foreign: emptySplit() }
    for (const { collector, reading } of pop) {
      if (!member(rollout, index, collector.attributes ?? {})) continue
      const path = reading?.path === 'foreign' ? 'foreign' : 'served'
      split[path].members++
      if (index <= rollout.stage) {
        split[path][runningArtefact(reading, rollout.artefacts)]++
      }
    }
    return {
      index,
      cohort: cohortLabel(stage.cohort),
      soak: stage.soak ?? '0h',
      state: index === rollout.stage ? 'active' : index < rollout.stage ? 'entered' : 'pending',
      widens: 0,
      ...split,
    }
  })
  for (let i = 0; i < cohorts.length; i++) {
    const total = cohorts[i].served.members + cohorts[i].foreign.members
    const previous =
      i === 0 ? 0 : cohorts[i - 1].served.members + cohorts[i - 1].foreign.members
    cohorts[i].widens = total - previous
  }

  // The evidence, computed over the active cohort — collectors actually
  // running the *to* artefact on either path count toward the advance;
  // members still on *from* are lag, displayed, never blocking (§7).
  const evidence = {
    membersSeen: 0,
    runningTo: 0,
    runningFrom: 0,
    runningOther: 0,
    unknown: 0,
    soaked: formatDuration(soakedMs),
    minSoak: rollout.stages[rollout.stage]?.soak ?? '0h',
  }
  const halts = []
  for (const { collector, reading } of pop) {
    if (!member(rollout, rollout.stage, collector.attributes ?? {})) continue
    evidence.membersSeen++
    const running = runningArtefact(reading, rollout.artefacts)
    if (running === 'to') evidence.runningTo++
    else if (running === 'from') evidence.runningFrom++
    else if (running === 'other') evidence.runningOther++
    else evidence.unknown++
    for (const condition of CONDITIONS) {
      const reason = condition.halt({ reading, running }, rollout.artefacts)
      if (reason !== undefined) {
        halts.push({
          collector: collector.id,
          path: reading?.path === 'foreign' ? 'foreign' : 'served',
          condition: condition.name,
          reason,
        })
      }
    }
  }
  const haltedMembers = new Set(halts.map((halt) => halt.collector)).size
  const abortFraction = rollout.abortFraction ?? DEFAULT_ABORT_FRACTION

  let decision
  let reason
  if (evidence.membersSeen > 0 && haltedMembers / evidence.membersSeen >= abortFraction) {
    decision = 'abort'
    reason = `${haltedMembers} of ${evidence.membersSeen} cohort members halted, at or past the abort threshold — propose reverting the Tier to single-bound from (ADR-0029 §6)`
  } else if (haltedMembers > 0) {
    decision = 'blocked'
    reason = `${haltedMembers} cohort member(s) halted below the abort threshold — the advance is simply never proposed (ADR-0029 §6)`
  } else if (soakedMs < minSoakMs) {
    decision = 'hold'
    reason = `soaked ${evidence.soaked} of the stage's minimum ${evidence.minSoak}`
  } else if (evidence.runningTo === 0) {
    decision = 'hold'
    reason =
      'no cohort member observed running the to artefact yet — advance evidence is computed over collectors actually running it (ADR-0029 §7)'
  } else {
    decision = 'advance'
    reason = `exit criteria met: ${evidence.runningTo} of ${evidence.membersSeen} cohort members running the to artefact, ${haltedMembers} halted, soaked ${evidence.soaked} of the minimum ${evidence.minSoak}`
  }

  const card = estate.cards.find((c) => c.tier === rollout.tier)
  return {
    id: `${rollout.team}/${rollout.name}`,
    name: rollout.name,
    team: rollout.team,
    owner: rollout.owner,
    tier: rollout.tier,
    tierName: card?.name ?? rollout.tier,
    environment: card?.environment ?? 'production',
    from: rollout.from,
    to: rollout.to,
    stage: rollout.stage,
    decision,
    reason,
    evidence,
    cohorts,
    halts,
    provenance: rollout.provenance ?? [],
  }
}

/** GET /api/v1/rollouts: every active Rollout's cohort progress. */
export function rolloutProgress(estate, now = new Date()) {
  return (estate.rollouts ?? []).map((rollout) => evaluateRollout(estate, rollout, now))
}
