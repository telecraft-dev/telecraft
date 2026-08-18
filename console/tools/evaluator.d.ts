// Typings for the fixture evaluator (evaluator.mjs), so the Vitest suite
// exercises it against the same shapes the console consumes. The response
// contracts live in src/api/types.ts — this file only types the module
// boundary.

import type { BlueprintDoc, ComposeVerdict, Proposal } from '../src/api/types'

export function validate(estate: unknown, draft: BlueprintDoc, environment: string): ComposeVerdict

export function propose(estate: unknown, draft: BlueprintDoc, environment: string): Proposal
