// Typings for the fixture Tier module (tiers.mjs), so the Vitest suite
// exercises it against the same shapes the console consumes. The response
// contracts live in src/api/types.ts: this file only types the module
// boundary.

import type { SetupGuidance, TierProposalRequest } from '../src/api/types'

export function tierProblems(estate: unknown, request: TierProposalRequest): string[]

export function setupGuidance(
  estate: unknown,
  activeVersion: string,
  tier: string,
): SetupGuidance | undefined
