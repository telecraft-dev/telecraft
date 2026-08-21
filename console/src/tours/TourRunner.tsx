import * as Dialog from '@radix-ui/react-dialog'
import { useRouterState, useSearch } from '@tanstack/react-router'
import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
} from 'react'
import { demoMode } from '../api/demo'
import { Button } from '../ui/Button'
import { CARD_HEIGHT, CARD_WIDTH, placeCard, type Placed } from './position'
import { WELCOME_TOUR, isBareLanding, stepBody } from './registry'
import { findAnchor, useAnchorBox } from './useAnchor'
import { useTour } from './useTour'

/**
 * The one thing that renders a Tour (ADR-0051 §1). Every Tour in the
 * registry is drawn by this component, from the ADR-0048 primitives and
 * the dialog jump-to-object already uses, so a new Tour is a file of prose
 * rather than a screen.
 *
 * Two placements, one card. A Step with an anchor points at it and takes
 * no pointer events, so the product underneath stays live and a reader may
 * ignore the Tour entirely — that is §2's "narrates, never drives" in the
 * markup. A Step without one is centred over a scrim in a real modal
 * dialog, which is the welcome (§7), and is also where an anchored Step
 * lands when its element is not on screen (§4).
 */

/** How much room the spotlight leaves around the thing it lights. */
const SPOTLIGHT_PAD = 6

export function TourRunner() {
  const { tour, index, step, seen, start, go, end } = useTour()
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const search = useSearch({ strict: false })

  const box = useAnchorBox(step?.anchor)
  const viewport = useViewport()
  const card = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState({ width: CARD_WIDTH, height: CARD_HEIGHT })

  // The welcome opens itself once per reader, and only on their own bare
  // arrival (§7). The ref makes that once per load as well: `seen` answers
  // "yes" until /api/v1/me resolves, and without it a reader who ends the
  // Tour would be offered it again the moment the URL lost its params.
  const offered = useRef(false)
  useEffect(() => {
    if (offered.current || tour !== undefined) return
    if (!isBareLanding(pathname, search as Record<string, unknown>)) return
    if (seen(WELCOME_TOUR)) return
    offered.current = true
    start(WELCOME_TOUR)
  }, [tour, pathname, search, seen, start])

  // A Step is addressable (§3), so a link to one that is read on another
  // Workspace takes the reader there: a cited Step that opened wherever
  // the link was clicked from would point at nothing, which §4 survives
  // but nobody meant.
  //
  // Once per Step, and never again, which is the whole of the difference
  // between narrating and driving (§2). A reader who walks off to another
  // Workspace mid-Step is not dragged back; their Step simply loses its
  // anchor and centres itself, and Next carries them on from there.
  const delivered = useRef<string>('')
  useEffect(() => {
    if (tour === undefined || step === undefined) return
    const here = `${tour.id}/${step.id}`
    if (delivered.current === here) return
    delivered.current = here
    if (step.to !== undefined && step.to !== pathname) go(index)
  }, [tour, step, pathname, index, go])

  // A Step that points at something below the fold scrolls it into view.
  // The Tour is allowed to move the page to the evidence; it is not
  // allowed to touch the evidence (§2).
  useEffect(() => {
    const element = findAnchor(step?.anchor)
    if (element === null) return
    const rect = element.getBoundingClientRect()
    if (rect.top >= 0 && rect.bottom <= window.innerHeight) return
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    element.scrollIntoView({ block: 'center', behavior: reduced ? 'auto' : 'smooth' })
  }, [step, tour])

  // The card is measured rather than assumed, because its height is the
  // prose's. `placeCard` gets the real size on the frame after the first.
  useLayoutEffect(() => {
    const element = card.current
    if (element === null) return
    const rect = element.getBoundingClientRect()
    setSize((prev) =>
      prev.width === rect.width && prev.height === rect.height
        ? prev
        : { width: rect.width, height: rect.height },
    )
  }, [step, box, viewport])

  // Focus follows the Step, which is what makes a Tour readable without a
  // mouse and audible to a screen reader. The centred Step is a Radix
  // dialog and does its own.
  useEffect(() => {
    if (box === null) return
    card.current?.focus()
  }, [step, box])

  // Escape leaves from anywhere, not only from the card: a reader who
  // clicked into the product mid-Tour still has the Tour on screen, and
  // reaching for Escape is what they will do.
  useEffect(() => {
    if (tour === undefined || box === null) return
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') end()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [tour, box, end])

  if (tour === undefined || step === undefined) return null

  const placed = placeCard(box, size, viewport)
  const last = index === tour.steps.length - 1

  // One card, two wrappers: the title arrives as an element because a
  // centred Step is a Radix dialog and owes it a `Dialog.Title`.
  const cardBody = (title: ReactNode) => (
    <TourCardBody
      title={title}
      progress={`Step ${index + 1} of ${tour.steps.length}`}
      text={stepBody(step, demoMode)}
      onBack={() => go(index - 1)}
      onNext={() => (last ? end() : go(index + 1))}
      onEnd={end}
      first={index === 0}
      last={last}
    />
  )

  const keys = (event: ReactKeyboardEvent) => {
    if (event.key === 'ArrowRight' && !last) go(index + 1)
    if (event.key === 'ArrowLeft' && index > 0) go(index - 1)
  }

  if (box === null) {
    return (
      <Dialog.Root open onOpenChange={(open) => !open && end()}>
        <Dialog.Portal>
          <Dialog.Overlay className="tour-scrim" />
          <Dialog.Content
            className="tour-card tour-centred"
            data-testid="tour-card"
            data-placement="centre"
            data-step={step.id}
            aria-describedby={undefined}
            onKeyDown={keys}
          >
            {cardBody(<Dialog.Title className="tour-title">{step.title}</Dialog.Title>)}
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    )
  }

  return (
    <>
      {/* Light, never a lid: the ring is drawn with a shadow that reaches
          the edges of the viewport, and the whole overlay is transparent to
          the pointer, so every control it dims still works (§2). */}
      <div
        className="tour-spotlight"
        data-testid="tour-spotlight"
        style={{
          top: box.top - SPOTLIGHT_PAD,
          left: box.left - SPOTLIGHT_PAD,
          width: box.width + SPOTLIGHT_PAD * 2,
          height: box.height + SPOTLIGHT_PAD * 2,
        }}
      />
      <div
        ref={card}
        className="tour-card"
        role="dialog"
        aria-label={`${tour.title}: ${step.title}`}
        data-testid="tour-card"
        data-placement={placed.placement}
        data-step={step.id}
        style={cardStyle(placed)}
        tabIndex={-1}
        onKeyDown={keys}
      >
        {cardBody(<h2 className="tour-title">{step.title}</h2>)}
      </div>
    </>
  )
}

function cardStyle(placed: Placed): CSSProperties {
  return { top: placed.top, left: placed.left, width: CARD_WIDTH }
}

/**
 * The card itself, identical in both placements. The title arrives as an
 * element because a centred Step is a Radix dialog and owes it a
 * `Dialog.Title`, while an anchored one is an ordinary heading.
 */
function TourCardBody({
  title,
  progress,
  text,
  onBack,
  onNext,
  onEnd,
  first,
  last,
}: {
  title: ReactNode
  progress: string
  text: string
  onBack: () => void
  onNext: () => void
  onEnd: () => void
  first: boolean
  last: boolean
}) {
  return (
    <>
      <p className="tour-progress" data-testid="tour-progress">
        {progress}
      </p>
      {title}
      <p className="tour-body">{text}</p>
      <footer className="tour-actions">
        <Button tone="quiet" onClick={onEnd} data-testid="tour-end">
          {last ? 'Close' : 'Skip the tour'}
        </Button>
        <span className="tour-actions-gap" />
        <Button onClick={onBack} disabled={first} data-testid="tour-back">
          Back
        </Button>
        <Button tone="primary" onClick={onNext} data-testid="tour-next">
          {last ? 'Done' : 'Next'}
        </Button>
      </footer>
    </>
  )
}

/** The viewport, which decides which side of an anchor the card fits on. */
function useViewport() {
  const [viewport, setViewport] = useState(() => ({
    width: window.innerWidth,
    height: window.innerHeight,
  }))
  useEffect(() => {
    const onResize = () => setViewport({ width: window.innerWidth, height: window.innerHeight })
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])
  return viewport
}
