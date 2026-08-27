import { useQuery } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { api } from '../../api/client'
import type { CollectorRow, EstatePayload, TeamNode } from '../../api/types'
import { formatAge } from '../../estate/readings'
import { formatObjectRef } from '../../objectref'
import { Chip } from '../../ui/Chip'

// The flat filter-first estate list (ADR-0042 §1): the InfoSec workflow
// and the only home of per-collector detail (rule 3.4): collector counts
// elsewhere are doors that land here pre-filtered. Filters are explicit
// and URL-addressable; the lens does not touch the rows at all (ADR-0059
// §3): the Environment filter is this surface's tool. Clicking a governed
// row summons the Tier's card panel in place (rule 3.2).
//
// Ungoverned collectors appear here too (ADR-0031 §2): concern, never
// failure: no Tier, no team, an explicit onboard affordance instead. The
// claim flow is herd-first (ADR-0042 §6): ungoverned rows multi-select
// into a herd, and the flow operates on the selection. Served and foreign
// rows select alike: collectors running the Unmatched artefact
// (ADR-0030) enter the same flow.

function flattenTeams(root: TeamNode): TeamNode[] {
  const out: TeamNode[] = []
  const walk = (team: TeamNode) => {
    out.push(team)
    for (const child of team.teams ?? []) walk(child)
  }
  walk(root)
  return out
}

/** The herd search param, split: selected ungoverned collector ids. */
export function herdIds(herd: string | undefined): string[] {
  return (herd ?? '').split(',').filter(Boolean)
}

// Age in the cell, the exact instant on the title (ADR-0040 §6): the
// reader scanning the column wants staleness, and the InfoSec reader who
// wants the instant hovers for it.
function LastSeen({ lastSeen }: { lastSeen: string | undefined }) {
  if (lastSeen === undefined) return <>never</>
  const takenAt = new Date(lastSeen)
  if (Number.isNaN(takenAt.getTime())) return <>{lastSeen}</>
  return <span title={lastSeen}>{formatAge((Date.now() - takenAt.getTime()) / 1000)} ago</span>
}

export function FlatList({
  payload,
  selectedTier,
}: {
  payload: EstatePayload
  selectedTier?: string
}) {
  const collectors = useQuery({ queryKey: ['collectors'], queryFn: api.collectors })
  const search = useSearch({ strict: false })
  const navigate = useNavigate()

  if (collectors.isPending) return <p className="surface-status">Loading collectors…</p>
  if (collectors.isError) return <p className="surface-status">Collectors failed to load.</p>

  const tierFilter = search.tier
  const teamFilter = search.team
  const envFilter = search.env
  const ungovernedOnly = search.ungoverned === true
  const herd = herdIds(search.herd)

  const rows = collectors.data.filter(
    (row) =>
      (!tierFilter || row.tier === tierFilter) &&
      (!teamFilter || row.team === teamFilter) &&
      (!envFilter || row.environment === envFilter) &&
      (!ungovernedOnly || row.ungoverned !== undefined),
  )

  const setFilter = (key: 'tier' | 'team' | 'env', value: string) =>
    void navigate({
      from: '/estate',
      to: '/estate',
      search: (prev) => ({ ...prev, [key]: value === '' ? undefined : value }),
    })

  // Herd selection lives in the URL (ADR-0042 §3.5): a half-built claim
  // can be cited and resumed. Toggling never touches the governed rows.
  const toggleHerd = (id: string) => {
    const next = herd.includes(id) ? herd.filter((h) => h !== id) : [...herd, id]
    void navigate({
      from: '/estate',
      to: '/estate',
      search: (prev) => ({ ...prev, herd: next.length > 0 ? next.join(',') : undefined }),
    })
  }

  const selectRow = (row: CollectorRow) => {
    if (row.ungoverned !== undefined) {
      toggleHerd(row.id)
      return
    }
    void navigate({
      from: '/estate',
      to: '/estate',
      search: (prev) => ({
        ...prev,
        object: formatObjectRef({ kind: 'tier', id: row.tier ?? '' }),
      }),
    })
  }

  return (
    <section className="flat-list" data-testid="flat-list">
      <div className="flat-filters">
        <label>
          Tier
          <select
            data-testid="filter-tier"
            value={tierFilter ?? ''}
            onChange={(event) => setFilter('tier', event.target.value)}
          >
            <option value="">all</option>
            {payload.cards.map((card) => (
              <option key={card.tier} value={card.tier}>
                {card.tier}
              </option>
            ))}
          </select>
        </label>
        <label>
          Team
          <select
            data-testid="filter-team"
            value={teamFilter ?? ''}
            onChange={(event) => setFilter('team', event.target.value)}
          >
            <option value="">all</option>
            {flattenTeams(payload.teams).map((team) => (
              <option key={team.id} value={team.id}>
                {team.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Environment
          <select
            data-testid="filter-env"
            value={envFilter ?? ''}
            onChange={(event) => setFilter('env', event.target.value)}
          >
            <option value="">all</option>
            {payload.environments.map((env) => (
              <option key={env} value={env}>
                {env}
              </option>
            ))}
          </select>
        </label>
        <label className="filter-toggle">
          <input
            type="checkbox"
            data-testid="filter-ungoverned"
            checked={ungovernedOnly}
            onChange={(event) =>
              void navigate({
                from: '/estate',
                to: '/estate',
                search: (prev) => ({
                  ...prev,
                  ungoverned: event.target.checked ? true : undefined,
                }),
              })
            }
          />
          Ungoverned only
        </label>
      </div>
      <table className="catalogue-table collector-table" data-testid="collector-table">
        <thead>
          <tr>
            <th className="herd-column" aria-label="Claim selection" />
            <th>Collector</th>
            <th>Tier</th>
            <th>Team</th>
            <th>Environment</th>
            <th>State</th>
            <th>Version</th>
            <th>Last seen</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr
              key={row.id}
              data-testid={`collector-${row.id}`}
              // No lens styling on rows (ADR-0059 §3): the Environment
              // filter above is the list's explicit tool, and dimming rows
              // a visible filter did not remove reads as a second filter.
              className={[
                row.tier !== undefined && row.tier === selectedTier ? 'selected' : '',
                row.ungoverned !== undefined ? 'ungoverned-row' : '',
                herd.includes(row.id) ? 'in-herd' : '',
              ]
                .filter(Boolean)
                .join(' ')}
              onClick={() => selectRow(row)}
            >
              <td className="herd-column">
                {row.ungoverned !== undefined && (
                  <input
                    type="checkbox"
                    data-testid={`herd-${row.id}`}
                    aria-label={`Select ${row.id} for the claim`}
                    checked={herd.includes(row.id)}
                    onClick={(event) => event.stopPropagation()}
                    onChange={() => toggleHerd(row.id)}
                  />
                )}
              </td>
              <td>{row.id}</td>
              <td>
                {row.tier ?? (
                  <Chip
                    tone="ungoverned"
                    className={`ungoverned-chip ungoverned-${row.ungoverned}`}
                    data-testid={`ungoverned-${row.id}`}
                  >
                    ungoverned · {row.ungoverned === 'served' ? 'served the Unmatched artefact' : 'foreign'}
                  </Chip>
                )}
              </td>
              <td>{row.team ?? 'none'}</td>
              <td>{row.environment}</td>
              <td>{row.state.replace('_', ' ')}</td>
              <td>{row.version}</td>
              <td>
                <LastSeen lastSeen={row.lastSeen} />
              </td>
            </tr>
          ))}
          {rows.length === 0 && (
            <tr>
              <td colSpan={8} className="section-summary">
                No collectors match the filters.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </section>
  )
}
