import { Button } from '../ui/Button'
import { Icon } from '../ui/Icon'
import { WELCOME_TOUR } from './registry'
import { useTour } from './useTour'

/**
 * Where the welcome Tour is offered a second time (ADR-0051 §7). It opens
 * itself once, on a reader's own bare arrival; after that it is here, and
 * it stays here, because the reader who most needs it again is the one who
 * skipped it the first time.
 *
 * One control for one Tour, deliberately. A Tour written later is offered
 * where it is relevant (beside the claim panel, beside the composer's
 * save gate), not from a menu of every Tour the console owns, which is a
 * museum rather than an affordance.
 *
 * Icon only since the chrome compaction (issue #182): of the compact
 * controls this is the one allowed to lose its word, because it changes
 * nothing about what the page shows. The button stays the whole carrier of
 * meaning, so it keeps its accessible name and says the same words in its
 * tooltip.
 */
export function TourControl() {
  const { tour, start } = useTour()

  return (
    <Button
      data-testid="tour-trigger"
      aria-label="Take the tour of the console"
      title="Take the tour of the console"
      // Restarting mid-Tour is what the control means while one is
      // running, and it is the same gesture: go back to the first Step.
      onClick={() => start(WELCOME_TOUR)}
      className={tour === undefined ? 'tour-trigger' : 'tour-trigger running'}
    >
      <Icon name="tour" />
    </Button>
  )
}
