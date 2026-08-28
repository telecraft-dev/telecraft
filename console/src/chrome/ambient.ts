import {
  BAND_ORDER,
  type ActivationsPayload,
  type Environment,
  type EstatePayload,
  type Severity,
} from '../api/types'
import { totalFindings } from '../estate/order'
import { rollupTree } from '../estate/rollup'
import { teamStanding } from '../home/summary'

// The context strip's ambient readings (ADR-0062, extending ADR-0058 §3).
// The strip derives and never judges, which is Home's posture (ADR-0056
// §2): every number here comes through the module that already owns the
// judgement. `estate/rollup.ts` supplies the root row's worst and waived
// counts, `estate/order.ts` a card's finding total, and the activations
// payload its own designation. A strip-only verdict could disagree with
// the surface its door opens, and a reader would have no way to tell
// which of the two was right.

/** The estate's finding standing under one Environment. */
export interface StandingReading {
  worst: Severity
  /** Findings on cards judged under the Environment. */
  findings: number
  /** Waived findings on the same cards: exemptions never hide (ADR-0017). */
  exempt: number
}

export function standingReading(payload: EstatePayload, lens: Environment): StandingReading {
  const [root] = rollupTree(payload.teams, payload.cards, lens)
  const inLens = payload.cards.filter((card) => card.environment === lens)
  return {
    worst: root ? teamStanding(root) : 'none',
    findings: inLens.reduce((n, card) => n + totalFindings(card), 0),
    exempt: root ? BAND_ORDER.reduce((n, kind) => n + root.kinds[kind].waived, 0) : 0,
  }
}

/** The Catalogue designation: what authoring judges against, and what is on offer. */
export interface CatalogueReading {
  /** The active version; empty when the estate has designated none. */
  active: string
  /** Versions on offer, each with a computed impact report behind it. */
  onOffer: string[]
}

export function catalogueReading(activations: ActivationsPayload): CatalogueReading | undefined {
  const substrate = activations.substrates.find((s) => s.kind === 'catalogue')
  if (!substrate) return undefined
  return {
    active: substrate.active,
    onOffer: substrate.candidates.map((candidate) => candidate.version),
  }
}

/** One non-lens Environment's finding count, for the quiet edge summary. */
export interface ElsewhereReading {
  environment: Environment
  findings: number
}

export function elsewhereReadings(payload: EstatePayload, lens: Environment): ElsewhereReading[] {
  return payload.environments
    .filter((environment) => environment !== lens)
    .map((environment) => ({
      environment,
      findings: payload.cards
        .filter((card) => card.environment === environment)
        .reduce((n, card) => n + totalFindings(card), 0),
    }))
}
