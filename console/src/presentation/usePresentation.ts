import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { api } from '../api/client'
import { PresentationStore } from './store'

/**
 * The signed-in user's presentation store (ADR-0042 §7), backed by
 * localStorage and keyed per user. Until /api/v1/me answers, reads fall
 * back to a throwaway anonymous store so nothing blocks first paint.
 */
export function usePresentation() {
  const me = useQuery({ queryKey: ['me'], queryFn: api.me })
  const user = me.data?.id
  const store = useMemo(
    () => new PresentationStore(window.localStorage, user ?? 'anonymous'),
    [user],
  )
  return { store, me: me.data }
}
