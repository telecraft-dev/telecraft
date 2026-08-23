import { useEffect, useState } from 'react'
import type { Box } from './position'

/**
 * Finding, measuring and re-measuring the element a Step points at.
 *
 * An anchor is a `data-tour` attribute a surface declares (ADR-0051 §5).
 * It may not be there yet (the surface may still be fetching), it may
 * move when a panel opens beside it, and it may never appear at all. All
 * three are the same case here: measure what is there now, watch for it to
 * change, and answer `null` when there is nothing, which the runner
 * renders as a centred Step (§4).
 */

/**
 * Anchors are authored identifiers, so the selector they build is
 * constrained rather than escaped. Anything else resolves nowhere, which
 * degrades exactly like a missing element.
 */
export function anchorSelector(anchor: string): string | null {
  return /^[a-z][a-z0-9-]*$/.test(anchor) ? `[data-tour="${anchor}"]` : null
}

/** The element an anchor names, or null. The first match wins: the shelf
 * orders worst-first, so the leading card is the one worth pointing at. */
export function findAnchor(anchor: string | undefined): HTMLElement | null {
  if (anchor === undefined) return null
  const selector = anchorSelector(anchor)
  if (selector === null) return null
  return document.querySelector<HTMLElement>(selector)
}

function boxOf(element: HTMLElement | null): Box | null {
  if (element === null) return null
  const rect = element.getBoundingClientRect()
  // A zero-area element is present but not laid out: a collapsed section,
  // or a surface mid-render. Pointing at it would draw a spotlight around
  // nothing, so it counts as absent until it has a size.
  if (rect.width === 0 && rect.height === 0) return null
  return { top: rect.top, left: rect.left, width: rect.width, height: rect.height }
}

function same(a: Box | null, b: Box | null): boolean {
  if (a === null || b === null) return a === b
  return a.top === b.top && a.left === b.left && a.width === b.width && a.height === b.height
}

export function useAnchorBox(anchor: string | undefined): Box | null {
  const [box, setBox] = useState<Box | null>(null)

  useEffect(() => {
    if (anchor === undefined) {
      setBox(null)
      return
    }

    let frame = 0
    const measure = () => {
      const next = boxOf(findAnchor(anchor))
      // Compared by value, not by identity: the observer below fires on
      // the runner's own renders too, and setting an equal-but-new
      // rectangle would loop forever.
      setBox((prev) => (same(prev, next) ? prev : next))
    }
    const schedule = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(measure)
    }

    schedule()
    // The element may arrive later than the Step: a surface fetches, and
    // the anchor appears with its data.
    const observer = new MutationObserver(schedule)
    observer.observe(document.body, { childList: true, subtree: true })
    window.addEventListener('resize', schedule)
    // Capture, because the thing that scrolls is a surface inside the
    // shell far more often than the window.
    window.addEventListener('scroll', schedule, true)

    return () => {
      cancelAnimationFrame(frame)
      observer.disconnect()
      window.removeEventListener('resize', schedule)
      window.removeEventListener('scroll', schedule, true)
    }
  }, [anchor])

  return box
}
