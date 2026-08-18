// The console's only non-git state: the per-user presentation preference
// store (ADR-0042 §7). Presentation only, never model truth, fully
// loseable: losing it changes what leads, never what is asserted. The
// closed key list below is the whole schema; anything resembling domain
// data added here is a design regression requiring an ADR-0042 amendment.
// tests/presentation.test.ts holds this shape.

export interface Presentation {
  /** The environment lens preference; an explicit lens in a URL beats it. */
  lens?: string
  /** Per-section collapse overrides: true collapsed, false expanded, absent default. */
  collapsedSections: Record<string, boolean>
  /** Within-row canvas arrangement (ADR-0044 §3): canvas id → node id → x offset. */
  arrangement: Record<string, Record<string, number>>
}

export const PRESENTATION_KEYS = ['lens', 'collapsedSections', 'arrangement'] as const

const EMPTY: Presentation = { collapsedSections: {}, arrangement: {} }

/** The subset of the Web Storage API the store uses; tests supply a fake. */
export interface StorageLike {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

export class PresentationStore {
  private readonly key: string

  constructor(
    private readonly storage: StorageLike,
    user: string,
  ) {
    this.key = `telecraft.console.presentation.v1.${user}`
  }

  /** Loads the user's preferences; anything unreadable reads as defaults. */
  load(): Presentation {
    const raw = this.storage.getItem(this.key)
    if (raw === null) return structuredClone(EMPTY)
    try {
      const parsed: unknown = JSON.parse(raw)
      if (typeof parsed !== 'object' || parsed === null) return structuredClone(EMPTY)
      const record = parsed as Record<string, unknown>
      return {
        lens: typeof record.lens === 'string' ? record.lens : undefined,
        collapsedSections:
          typeof record.collapsedSections === 'object' &&
          record.collapsedSections !== null &&
          !Array.isArray(record.collapsedSections)
            ? (record.collapsedSections as Presentation['collapsedSections'])
            : {},
        arrangement:
          typeof record.arrangement === 'object' && record.arrangement !== null
            ? (record.arrangement as Presentation['arrangement'])
            : {},
      }
    } catch {
      return structuredClone(EMPTY)
    }
  }

  /** Merges a patch over the stored preferences and persists the result. */
  save(patch: Partial<Presentation>): Presentation {
    const next = { ...this.load(), ...patch }
    this.storage.setItem(this.key, JSON.stringify(next))
    return next
  }
}
