import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { api } from '../../api/client'
import { formatObjectRef } from '../../objectref'
import { buttonClass } from '../../ui/Button'
import { Chip } from '../../ui/Chip'
import { count } from '../../ui/text'
import {
  endorsementFor,
  environmentOptions,
  matchesFilters,
  serviceClassOptions,
  substrateLabel,
  substrateOptions,
} from './browse'

/**
 * The Blueprints browse view (ADR-0061 §3): every Blueprint the reader may
 * see, with its declared `fits` facets, its endorsed mark where one holds,
 * what it satisfies, and its current version. The filters answer the
 * discovery scenario directly and live in the URL like the flat list's
 * (ADR-0042 §3.5); the shared `env` param is the Environment filter.
 *
 * Presentation only (ADR-0061 §5): a Blueprint outside the reader's Team
 * is still listed, and enforcement stays at the enforcement points. The
 * endorsements query loading reads as no Endorsements yet, so the list
 * never blocks on the marks.
 */
export function Blueprints() {
  const blueprints = useQuery({ queryKey: ['blueprints'], queryFn: api.blueprints })
  const endorsements = useQuery({ queryKey: ['endorsements'], queryFn: api.endorsements })
  const search = useSearch({ strict: false })
  const navigate = useNavigate()

  if (blueprints.isPending) return <p className="surface-status">Loading Blueprints…</p>
  if (blueprints.isError) return <p className="surface-status">Blueprints failed to load.</p>

  const docs = blueprints.data
  const held = endorsements.data ?? []
  const rows = docs.filter((bp) =>
    matchesFilters(
      bp,
      {
        substrate: search.substrate,
        environment: search.env,
        serviceClass: search.serviceClass,
        endorsedOnly: search.endorsed,
      },
      held,
    ),
  )

  const setFilter = (key: 'substrate' | 'env' | 'serviceClass', value: string) =>
    void navigate({
      from: '/compose',
      to: '/compose',
      search: (prev) => ({ ...prev, [key]: value === '' ? undefined : value }),
    })

  return (
    <section className="blueprint-browse" data-testid="blueprint-browse">
      <h1>Blueprints</h1>
      <div className="browse-filters">
        <label>
          Substrate
          <select
            data-testid="filter-substrate"
            value={search.substrate ?? ''}
            onChange={(event) => setFilter('substrate', event.target.value)}
          >
            <option value="">all</option>
            {substrateOptions(docs).map((value) => (
              <option key={value} value={value}>
                {substrateLabel(value)}
              </option>
            ))}
          </select>
        </label>
        <label>
          Environment
          <select
            data-testid="filter-environment"
            value={search.env ?? ''}
            onChange={(event) => setFilter('env', event.target.value)}
          >
            <option value="">all</option>
            {environmentOptions(docs).map((value) => (
              <option key={value} value={value}>
                {value}
              </option>
            ))}
          </select>
        </label>
        <label>
          Service Class
          <select
            data-testid="filter-service-class"
            value={search.serviceClass ?? ''}
            onChange={(event) => setFilter('serviceClass', event.target.value)}
          >
            <option value="">all</option>
            {serviceClassOptions(docs).map((value) => (
              <option key={value} value={value}>
                {value}
              </option>
            ))}
          </select>
        </label>
        <label className="filter-toggle">
          <input
            type="checkbox"
            data-testid="filter-endorsed"
            checked={search.endorsed === true}
            onChange={(event) =>
              void navigate({
                from: '/compose',
                to: '/compose',
                search: (prev) => ({
                  ...prev,
                  endorsed: event.target.checked ? true : undefined,
                }),
              })
            }
          />
          Endorsed only
        </label>
      </div>

      {rows.length === 0 ? (
        <p className="section-summary" data-testid="browse-empty">
          No Blueprints match these filters.
        </p>
      ) : (
        <ul className="browse-entries">
          {rows.map((bp) => {
            const mark = endorsementFor(bp, held)
            return (
              <li
                key={bp.id}
                className="browse-entry"
                data-testid={`blueprint-entry-${bp.id}`}
              >
                <div className="browse-entry-head">
                  <h2>
                    {bp.name}{' '}
                    <span className="item-meta">
                      v{bp.version} · {bp.team}
                    </span>
                  </h2>
                  {mark && (
                    <Chip tone={mark.current ? 'ok' : 'neutral'} data-testid={`endorsed-${bp.id}`}>
                      {mark.current ? 'endorsed' : `endorsed at v${mark.version}`}
                    </Chip>
                  )}
                </div>
                {/* Only the declared facets draw: an absent facet is
                    undeclared, and drawing "any" for it would claim a fit
                    the owner never made (ADR-0061 §1). */}
                <dl className="browse-fits">
                  {bp.fits?.substrates && (
                    <div>
                      <dt>Substrates</dt>
                      <dd>{bp.fits.substrates.map(substrateLabel).join(', ')}</dd>
                    </div>
                  )}
                  {bp.fits?.environments && (
                    <div>
                      <dt>Environments</dt>
                      <dd>{bp.fits.environments.join(', ')}</dd>
                    </div>
                  )}
                  {bp.fits?.serviceClasses && (
                    <div>
                      <dt>Service Classes</dt>
                      <dd>{bp.fits.serviceClasses.join(', ')}</dd>
                    </div>
                  )}
                </dl>
                {bp.satisfies.length > 0 && (
                  <p className="browse-satisfies">
                    Satisfies {count(bp.satisfies.length, 'Requirement')}
                  </p>
                )}
                <div className="browse-doors">
                  <Link
                    to="/compose"
                    search={(prev) => ({
                      ...prev,
                      browse: undefined,
                      object: formatObjectRef({ kind: 'blueprint', id: bp.id }),
                    })}
                    className={buttonClass('secondary')}
                    data-testid={`open-composer-${bp.id}`}
                  >
                    Open in the composer
                  </Link>
                  {/* The Tier-first flow's door (ADR-0060 §1): a fresh
                      search object, because the pre-fill crosses routes. */}
                  <Link
                    to="/estate"
                    search={{ add: true, blueprint: `${bp.id}@${bp.version}` }}
                    className={buttonClass('secondary')}
                    data-testid={`add-tier-${bp.id}`}
                  >
                    Add a Tier using this Blueprint
                  </Link>
                </div>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
