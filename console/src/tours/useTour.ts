import { useNavigate, useRouterState, useSearch } from '@tanstack/react-router'
import { useCallback } from 'react'
import { usePresentation } from '../presentation/usePresentation'
import { stepIndex, tourById } from './registry'
import type { Step, Tour } from './types'

/**
 * The running Tour, which is a reading of the URL and nothing else
 * (ADR-0051 §3). `tour` and `step` are root search params, so a Step is
 * citable in a pull-request comment exactly like a traced Path or a
 * filtered list, and the browser's back button walks the Tour backwards
 * because every Step was a navigation.
 *
 * Nothing here is component state. A Tour that kept its position in a
 * `useState` would be the one console state that cannot be linked, which
 * ADR-0042 §3.5 exists to prevent.
 */
export interface RunningTour {
  /** The Tour named by the URL, if it names one that exists. */
  tour: Tour | undefined
  /** The Step within it, clamped; zero when no Tour runs. */
  index: number
  step: Step | undefined
  /** Whether this reader has been shown a given Tour before. */
  seen: (id: string) => boolean
  /** Opens a Tour at its first Step, and records that it has been offered. */
  start: (id: string) => void
  /** Moves to another Step of the running Tour, by index. */
  go: (index: number) => void
  /** Leaves the Tour, dropping it from the URL. */
  end: () => void
}

export function useTour(): RunningTour {
  const search = useSearch({ strict: false })
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const navigate = useNavigate()
  const { store, me } = usePresentation()

  const tour = tourById(search.tour)
  const index = tour ? stepIndex(tour, search.step) : 0

  /**
   * One navigation per Step, with one rule for what it carries.
   *
   * Staying on the same Workspace keeps everything the reader had and lays
   * the Step's own params over it. Crossing to another Workspace narrows
   * to the lens, the Tour, and what the Step asked for — which is what
   * jump-to-object already does, because one Workspace's params mean
   * nothing in another.
   */
  const goTo = useCallback(
    (target: Tour, next: number) => {
      const step = target.steps[next]
      if (step === undefined) return
      const params = { tour: target.id, step: next + 1, ...(step.search ?? {}) }
      const crossing = step.to !== undefined && step.to !== pathname
      if (crossing) {
        void navigate({
          to: step.to,
          search: (prev: Record<string, unknown>) => ({ lens: prev.lens, ...params }),
        } as never)
        return
      }
      void navigate({
        search: (prev: Record<string, unknown>) => ({ ...prev, ...params }),
      } as never)
    },
    [navigate, pathname],
  )

  const seen = useCallback(
    (id: string) => {
      // Until /api/v1/me answers, the store is a throwaway anonymous one
      // (see usePresentation). Reading "not seen" from it would offer the
      // welcome to somebody who has already had it, so an unresolved user
      // counts as seen and the offer waits a beat.
      if (me === undefined) return true
      return store.load().toursSeen[id] === true
    },
    [store, me],
  )

  const start = useCallback(
    (id: string) => {
      const target = tourById(id)
      if (target === undefined) return
      // Recorded on opening rather than on finishing: a reader who closes
      // the tab halfway through has still been offered it, and being
      // offered it again on every visit is the annoyance §7 avoids.
      if (me !== undefined) {
        store.save({ toursSeen: { ...store.load().toursSeen, [id]: true } })
      }
      goTo(target, 0)
    },
    [goTo, store, me],
  )

  const go = useCallback(
    (next: number) => {
      if (tour === undefined) return
      goTo(tour, next)
    },
    [goTo, tour],
  )

  const end = useCallback(() => {
    void navigate({
      search: (prev: Record<string, unknown>) => ({ ...prev, tour: undefined, step: undefined }),
    } as never)
  }, [navigate])

  return { tour, index, step: tour?.steps[index], seen, start, go, end }
}
