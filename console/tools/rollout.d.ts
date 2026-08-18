// Typings for the fixture rollout module (rollout.mjs), so the Vitest
// suite exercises it against the same shapes the console consumes. The
// response contract lives in src/api/types.ts — this file only types the
// module boundary.

import type { RolloutProgress, RolloutRunning } from '../src/api/types'

/** One authored cohort spec in its fixture shape (ADR-0029 §4). */
export interface FixtureCohortSpec {
  hosts?: { attribute: string; values: string[] }
  match?: Record<string, string>
  percent?: number
}

/** One authored Rollout in its fixture shape, plus the artefact facts the
 * running reading compares on. */
export interface FixtureRollout {
  name: string
  team: string
  owner: string
  tier: string
  from: string
  to: string
  stage: number
  hashAttributes?: string[]
  stages: { cohort: FixtureCohortSpec; soak?: string }[]
  stageStarted: string
  abortFraction?: number
  artefacts: {
    from: { hash: string; components: string[] }
    to: { hash: string; components: string[] }
  }
  provenance?: unknown[]
}

/** One collector's rollout reading in its fixture shape. */
export interface FixtureRolloutReading {
  path?: 'served' | 'foreign'
  remote?: { state: 'applied' | 'applying' | 'failed'; configHash?: string; error?: string }
  stamp?: { tier: string; commit: string; components?: string[] }
  silent?: boolean
}

export function bucket(keys: string[], attrs: Record<string, string>): number | undefined

export function member(
  rollout: Pick<FixtureRollout, 'stages' | 'hashAttributes'>,
  stage: number,
  attrs: Record<string, string>,
): boolean

export function runningArtefact(
  reading: FixtureRolloutReading | undefined,
  artefacts: FixtureRollout['artefacts'],
): RolloutRunning

export function cohortLabel(cohort: FixtureCohortSpec): string

export function evaluateRollout(
  estate: unknown,
  rollout: FixtureRollout,
  now: Date,
): RolloutProgress

export function rolloutProgress(estate: unknown, now?: Date): RolloutProgress[]
