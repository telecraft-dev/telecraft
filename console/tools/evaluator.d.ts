// Typings for the fixture evaluator (evaluator.mjs), so the Vitest suite
// exercises it against the same shapes the console consumes. The response
// contracts live in src/api/types.ts — this file only types the module
// boundary. `entries` is the active catalogue's entry list, the same data
// /api/v1/catalogue/entries serves.

import type {
  BlueprintDoc,
  CatalogueEntry,
  ClaimContext,
  ComposeVerdict,
  Proposal,
} from '../src/api/types'

export function validate(
  estate: unknown,
  entries: CatalogueEntry[],
  draft: BlueprintDoc,
  environment: string,
): ComposeVerdict

export function propose(
  estate: unknown,
  entries: CatalogueEntry[],
  draft: BlueprintDoc,
  environment: string,
  claim?: ClaimContext,
): Proposal
