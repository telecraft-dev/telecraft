// Typings for the fixture claim module (claims.mjs), so the Vitest suite
// exercises it against the same shapes the console consumes. The response
// contracts live in src/api/types.ts: this file only types the module
// boundary.

import type {
  ClaimContext,
  ClaimOutcome,
  ClaimPreview,
  ClaimPreviewRequest,
  ClaimRequest,
  UngovernedSummary,
} from '../src/api/types'

export const INSTANCE_KEYS: Set<string>

export function ungovernedSummary(estate: unknown): UngovernedSummary

export function claimContextProblems(estate: unknown, claim: ClaimContext): string[]

export function previewClaim(estate: unknown, body: ClaimPreviewRequest): ClaimPreview

export function submitClaim(estate: unknown, body: ClaimRequest): ClaimOutcome
