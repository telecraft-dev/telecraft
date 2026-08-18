import { useQuery } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { api } from '../../api/client'
import type { EstatePayload, TeamNode } from '../../api/types'
import { useLens } from '../../chrome/LensControl'
import { formatObjectRef } from '../../objectref'

// The flat filter-first estate list (ADR-0042 §1): the InfoSec workflow
// and the only home of per-collector detail (rule 3.4) — collector counts
// elsewhere are doors that land here pre-filtered. Filters are explicit
// and URL-addressable; the lens only emphasises its Environment's rows,
// it never filters (§4). Clicking a row summons the Tier's card panel in
// place (rule 3.2).

function flattenTeams(root: TeamNode): TeamNode[] {
  const out: TeamNode[] = []
  const walk = (team: TeamNode) => {
    out.push(team)
    for (const child of team.teams ?? []) walk(child)
  }
  walk(root)
  return out
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
  const lens = useLens()

  if (collectors.isPending) return <p className="surface-status">Loading collectors…</p>
  if (collectors.isError) return <p className="surface-status">Collectors failed to load.</p>

  const tierFilter = search.tier
  const teamFilter = search.team
  const envFilter = search.env

  const rows = collectors.data.filter(
    (row) =>
      (!tierFilter || row.tier === tierFilter) &&
      (!teamFilter || row.team === teamFilter) &&
      (!envFilter || row.environment === envFilter),
  )

  const setFilter = (key: 'tier' | 'team' | 'env', value: string) =>
    void navigate({
      to: '/estate',
      search: (prev) => ({ ...prev, [key]: value === '' ? undefined : value }),
    })

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
      </div>
      <table className="catalogue-table collector-table" data-testid="collector-table">
        <thead>
          <tr>
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
              className={[
                row.environment === lens ? 'lens-leading' : 'lens-muted',
                row.tier === selectedTier ? 'selected' : '',
              ]
                .filter(Boolean)
                .join(' ')}
              onClick={() =>
                void navigate({
                  to: '/estate',
                  search: (prev) => ({
                    ...prev,
                    object: formatObjectRef({ kind: 'tier', id: row.tier }),
                  }),
                })
              }
            >
              <td>{row.id}</td>
              <td>{row.tier}</td>
              <td>{row.team}</td>
              <td>{row.environment}</td>
              <td>{row.state.replace('_', ' ')}</td>
              <td>{row.version}</td>
              <td>{row.lastSeen ?? 'never'}</td>
            </tr>
          ))}
          {rows.length === 0 && (
            <tr>
              <td colSpan={7} className="section-summary">
                No collectors match the filters.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </section>
  )
}
