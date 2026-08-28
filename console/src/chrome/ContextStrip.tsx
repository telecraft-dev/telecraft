import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { api } from '../api/client'
import { catalogueReading, elsewhereReadings, standingReading } from './ambient'
import { LensControl, useLens } from './LensControl'
import { ungovernedTotal } from '../home/summary'
import { chipClass } from '../ui/Chip'
import { Mark } from '../ui/Mark'
import { count } from '../ui/text'

// The context strip (ADR-0058): the lens, the surface-level context
// controls as they earn a place, and the ambient standing readings beside
// them (ADR-0062). Three readings and no more: the estate's finding
// standing under the lens, the Catalogue designation, and the ungoverned
// population, with a quiet edge summary for the other Environments. Each
// reading is a door to the surface that owns it, carrying its filter the
// way Home's doors do (ADR-0056 §4), and none of them is judged here: the
// derivation lives in ./ambient.ts, on top of the modules the surfaces
// themselves read.
//
// The queries are the ones the Workspaces already make. The lens select
// reads the estate payload on every surface as it is, and TanStack Query
// serves both from one fetch; the activations read is the strip's only
// addition, shared with the Activation view the same way.

export function ContextStrip() {
  const lens = useLens()
  const estate = useQuery({ queryKey: ['estate'], queryFn: api.estate })
  const activations = useQuery({ queryKey: ['activations'], queryFn: api.activations })

  const standing = estate.data ? standingReading(estate.data, lens) : undefined
  const catalogue = activations.data ? catalogueReading(activations.data) : undefined
  const ungoverned = estate.data ? ungovernedTotal(estate.data.ungoverned) : 0
  const elsewhere = estate.data ? elsewhereReadings(estate.data, lens) : []

  return (
    <div className="context-strip">
      <LensControl />
      {standing && (
        <span className={`strip-item severity-${standing.worst}`} data-testid="strip-standing">
          {(standing.worst === 'violation' || standing.worst === 'advisory') && (
            <Mark name={standing.worst} />
          )}
          <Link
            to="/"
            search={(prev) => ({ lens: prev.lens, tour: prev.tour, step: prev.step })}
            className="count-door"
          >
            {standing.findings === 0 ? 'clear' : count(standing.findings, 'finding')}
            {standing.exempt > 0 && ` · ${standing.exempt} exempt`}
          </Link>
        </span>
      )}
      {catalogue && catalogue.active !== '' && (
        <span className="strip-item" data-testid="strip-catalogue">
          <span className="strip-label">Catalogue</span>
          <Link
            to="/catalogue"
            search={(prev) => ({
              lens: prev.lens,
              tour: prev.tour,
              step: prev.step,
              view: 'browse' as const,
            })}
            className="count-door strip-version"
          >
            {catalogue.active}
          </Link>
          {catalogue.onOffer.length > 0 && (
            <>
              <span className="strip-label">·</span>
              <Link
                to="/catalogue"
                search={(prev) => ({
                  lens: prev.lens,
                  tour: prev.tour,
                  step: prev.step,
                  view: 'activation' as const,
                })}
                className="count-door"
                data-testid="strip-catalogue-offer"
              >
                {catalogue.onOffer.length === 1
                  ? `${catalogue.onOffer[0]} on offer`
                  : `${catalogue.onOffer.length} versions on offer`}
              </Link>
            </>
          )}
        </span>
      )}
      {ungoverned > 0 && (
        <span className="strip-item">
          <Link
            to="/estate"
            search={(prev) => ({
              lens: prev.lens,
              tour: prev.tour,
              step: prev.step,
              view: 'list' as const,
              ungoverned: true,
            })}
            className={chipClass('ungoverned')}
            data-testid="strip-ungoverned"
          >
            {count(ungoverned, 'ungoverned collector')}
          </Link>
        </span>
      )}
      {elsewhere.length > 0 && (
        <span className="strip-item strip-elsewhere" data-testid="strip-elsewhere">
          {elsewhere
            .map(
              (reading) =>
                `${reading.environment}: ${
                  reading.findings === 0 ? 'clear' : count(reading.findings, 'finding')
                }`,
            )
            .join(' · ')}
        </span>
      )}
    </div>
  )
}
