import { describe, expect, it } from 'vitest'
import {
  PRESENTATION_KEYS,
  PresentationStore,
  type StorageLike,
} from '../src/presentation/store'

// The presentation store is the console's only non-git state (ADR-0042
// §7): per user, presentation only, fully loseable. These tests hold that
// shape.

function memoryStorage(): StorageLike & { data: Map<string, string> } {
  const data = new Map<string, string>()
  return {
    data,
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => void data.set(key, value),
  }
}

describe('PresentationStore', () => {
  it('round-trips preferences', () => {
    const store = new PresentationStore(memoryStorage(), 'user-a')
    store.save({ lens: 'staging', collapsedSections: { infosec: true } })
    expect(store.load()).toEqual({
      lens: 'staging',
      collapsedSections: { infosec: true },
      arrangement: {},
      toursSeen: {},
    })
  })

  it('persists per user: two users never share preferences', () => {
    const storage = memoryStorage()
    const alpha = new PresentationStore(storage, 'user-a')
    const beta = new PresentationStore(storage, 'user-b')
    alpha.save({ lens: 'staging' })
    expect(beta.load().lens).toBeUndefined()
    expect(alpha.load().lens).toBe('staging')
  })

  it('reads corrupt state as defaults: the store is fully loseable', () => {
    const storage = memoryStorage()
    storage.setItem('telecraft.console.presentation.v1.user-a', '{not json')
    const store = new PresentationStore(storage, 'user-a')
    expect(store.load()).toEqual({ collapsedSections: {}, arrangement: {}, toursSeen: {} })
  })

  it('remembers which Tours a reader has been offered (ADR-0051 §6)', () => {
    const storage = memoryStorage()
    const store = new PresentationStore(storage, 'user-a')
    store.save({ toursSeen: { welcome: true } })
    expect(store.load().toursSeen).toEqual({ welcome: true })

    // And it is loseable like everything else here: a nonsense value
    // reads as "seen nothing", which offers the welcome again rather than
    // throwing on the way to first paint.
    storage.setItem(
      'telecraft.console.presentation.v1.user-a',
      JSON.stringify({ collapsedSections: {}, arrangement: {}, toursSeen: 7 }),
    )
    expect(store.load().toursSeen).toEqual({})
  })

  it('holds presentation keys only, never domain data', () => {
    const storage = memoryStorage()
    const store = new PresentationStore(storage, 'user-a')
    store.save({
      lens: 'production',
      collapsedSections: { 'data-flow': false },
      arrangement: { topology: { 'data-flow/gateway': 24 } },
      toursSeen: { welcome: true },
    })
    const persisted = JSON.parse([...storage.data.values()][0]!) as Record<string, unknown>
    for (const key of Object.keys(persisted)) {
      expect(PRESENTATION_KEYS).toContain(key)
    }
  })
})
