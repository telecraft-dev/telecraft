import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { Button } from './Button'

/**
 * The side panel, and the drag handle that sizes it.
 *
 * ADR-0041 makes the panel one universal component summoned in place beside
 * whatever was read; ADR-0042 §3.2 makes inspection never navigate. Three
 * surfaces had grown their own version of it (the card panel, the claim
 * panel and the compose flyout) at three fixed widths (320px, 380px,
 * 380px), none of which is right for everyone: a Tier's findings and a
 * rendered YAML document want very different amounts of room, and so do a
 * 13-inch laptop and a 32-inch display.
 *
 * So the width is the reader's, not ours. Drag the handle, or focus it and
 * use the arrow keys; double-click resets it. The chosen width is a device
 * preference and follows the theme's rule (ADR-0047 §2): it lives in
 * `localStorage`, not the URL, because it says nothing about what is on
 * screen and a shared link should not carry it.
 */

const MIN_WIDTH = 280
const MAX_WIDTH = 960
const KEYBOARD_STEP = 24

const storageKeyFor = (name: string) => `telecraft.panel.${name}`

function loadWidth(name: string, fallback: number): number {
  try {
    const stored = Number(localStorage.getItem(storageKeyFor(name)))
    return Number.isFinite(stored) && stored > 0 ? clamp(stored) : fallback
  } catch {
    return fallback
  }
}

function saveWidth(name: string, width: number): void {
  try {
    localStorage.setItem(storageKeyFor(name), String(width))
  } catch {
    // Not remembering the width is survivable; the panel still opens.
  }
}

const clamp = (width: number) =>
  Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, Math.round(width)))

/** What the viewport can spare, leaving the surface beside it usable. */
const available = () => Math.max(MIN_WIDTH, window.innerWidth - 240)

/**
 * The width of one named panel, remembered across sessions.
 *
 * The stored *preference* and the *applied* width are deliberately two
 * things. A viewport narrower than the preference applies less, but never
 * writes less back: a reader who parks a wide panel, narrows the window,
 * and widens it again gets their width returned. Persisting the bounded
 * value instead would quietly lose it the first time they resized a window.
 */
export function useResizableWidth(name: string, initial: number) {
  const [preferred, setPreferred] = useState(() => loadWidth(name, initial))
  const [room, setRoom] = useState(() => available())

  useEffect(() => {
    const onWindowResize = () => setRoom(available())
    window.addEventListener('resize', onWindowResize)
    return () => window.removeEventListener('resize', onWindowResize)
  }, [])

  const set = useCallback(
    (next: number) => {
      const bounded = clamp(Math.min(next, available()))
      setPreferred(bounded)
      saveWidth(name, bounded)
    },
    [name],
  )

  const reset = useCallback(() => set(initial), [set, initial])

  return { width: Math.min(preferred, room), set, reset }
}

/**
 * The handle itself: a separator, which is what it is to a screen reader,
 * with the arrow keys doing what dragging does. Pointer capture keeps the
 * drag alive over the canvas and over an iframe.
 */
export function ResizeHandle({
  width,
  onResize,
  onReset,
  label,
  testId,
}: {
  width: number
  onResize: (width: number) => void
  onReset: () => void
  label: string
  testId?: string
}) {
  const dragging = useRef<{ startX: number; startWidth: number } | null>(null)

  return (
    <div
      className="panel-resize"
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      aria-valuenow={width}
      aria-valuemin={MIN_WIDTH}
      aria-valuemax={MAX_WIDTH}
      tabIndex={0}
      data-testid={testId}
      onDoubleClick={onReset}
      onPointerDown={(event) => {
        // The panel grows leftwards, so a drag towards the left is wider.
        dragging.current = { startX: event.clientX, startWidth: width }
        event.currentTarget.setPointerCapture(event.pointerId)
      }}
      onPointerMove={(event) => {
        const drag = dragging.current
        if (!drag) return
        onResize(drag.startWidth + (drag.startX - event.clientX))
      }}
      onPointerUp={(event) => {
        dragging.current = null
        event.currentTarget.releasePointerCapture(event.pointerId)
      }}
      onKeyDown={(event) => {
        if (event.key === 'ArrowLeft') onResize(width + KEYBOARD_STEP)
        else if (event.key === 'ArrowRight') onResize(width - KEYBOARD_STEP)
        else if (event.key === 'Home') onReset()
        else return
        event.preventDefault()
      }}
    />
  )
}

/**
 * A panel with a head, a close control and a resize handle. `name` is the
 * key its width is remembered under, so two kinds of panel keep two widths
 * and the same kind keeps one wherever it is summoned.
 */
export function Panel({
  name,
  title,
  titleTestId,
  onClose,
  closeTestId,
  initialWidth = 340,
  className,
  testId,
  children,
}: {
  name: string
  title: ReactNode
  titleTestId?: string
  onClose: () => void
  closeTestId?: string
  initialWidth?: number
  className?: string
  testId?: string
  children: ReactNode
}) {
  const { width, set, reset } = useResizableWidth(name, initialWidth)

  return (
    <aside
      className={className ? `panel ${className}` : 'panel'}
      style={{ width: `${width}px` }}
      data-testid={testId}
    >
      <ResizeHandle
        width={width}
        onResize={set}
        onReset={reset}
        label={`Resize the ${name} panel`}
        testId={`${testId ?? name}-resize`}
      />
      <div className="panel-body">
        <header className="panel-head">
          <h2 data-testid={titleTestId}>{title}</h2>
          <Button tone="quiet" data-testid={closeTestId} onClick={onClose}>
            Close
          </Button>
        </header>
        {children}
      </div>
    </aside>
  )
}
