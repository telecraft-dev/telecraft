import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { useState } from 'react'
import { api } from '../../api/client'
import type { TeamNode } from '../../api/types'
import { formatCatalogueKey } from '../../api/types'
import type { PaletteOrigin } from '../../api/types'
import type { PaletteRow } from '../../governance/effective'
import { effectivePalette } from '../../governance/effective'
import { Button, buttonClass } from '../../ui/Button'
import { chipClass } from '../../ui/Chip'

// The wire enum stays as it is; the chip shows the glossary casing.
const ORIGIN_LABEL: Record<PaletteOrigin, string> = {
  'default-allow': 'default allow',
  'allow-list': 'Allow-list',
  grant: 'Grant',
}

function flattenTeams(node: TeamNode, out: { id: string; name: string }[] = []) {
  out.push({ id: node.id, name: node.name })
  for (const child of node.teams ?? []) flattenTeams(child, out)
  return out
}

/**
 * A team's effective palette with total provenance (ADR-0021): every
 * catalogue entry, allowed or narrowed out, and "why is this allowed?"
 * resolving to the named Grant, the ancestor Allow-lists survived, or the
 * default posture. A non-allowed row's door is the Grant request, the
 * browse-and-request flow (ADR-0042 §1); the exit is a PR.
 */
export function PaletteView() {
  const me = useQuery({ queryKey: ['me'], queryFn: api.me })
  const estate = useQuery({ queryKey: ['estate'], queryFn: api.estate })
  const governance = useQuery({ queryKey: ['governance'], queryFn: api.governance })
  const versions = useQuery({ queryKey: ['catalogue-versions'], queryFn: api.catalogueVersions })
  const active = versions.data?.active
  const entries = useQuery({
    queryKey: ['catalogue-entries', active],
    queryFn: () => api.catalogueEntries(active as string),
    enabled: active !== undefined,
  })
  const search = useSearch({ from: '/catalogue' })
  const navigate = useNavigate()
  const [openWhy, setOpenWhy] = useState<string | undefined>()

  const pending =
    me.isPending || estate.isPending || governance.isPending || versions.isPending || entries.isPending
  const failed = me.isError || estate.isError || governance.isError || versions.isError || entries.isError
  if (pending) return <p className="surface-status">Judging the palette…</p>
  if (failed) return <p className="surface-status">The palette failed to load.</p>

  const team = search.team ?? me.data.team
  const palette = effectivePalette({
    tree: estate.data.teams,
    team,
    entries: entries.data,
    governance: governance.data,
  })
  if (!palette) {
    return <p className="surface-status">No team "{team}" in the team tree.</p>
  }
  const allowed = palette.rows.filter((row) => row.allowed).length

  const whyContent = (row: PaletteRow) => {
    if (!row.allowed) {
      return (
        <>
          <p className="why-claim">
            Narrowed out by the {row.narrowedBy} Allow-list
          </p>
          <p>
            A team&rsquo;s Allow-list can only remove entries from its parent&rsquo;s list. To
            add this one back, request a Grant for {team} or a team below it.
          </p>
        </>
      )
    }
    switch (row.origin) {
      case 'grant':
        return (
          <>
            <p className="why-claim">Admitted by Grant {row.grant?.id}</p>
            <p>
              Granted by {row.grantedBy} to {row.grant?.team}. Without the Grant, the
              Allow-lists would exclude it.
            </p>
            <p className="mono">{row.grant?.adds.join(', ')}</p>
          </>
        )
      case 'allow-list':
        return (
          <>
            <p className="why-claim">Allowed by every Allow-list declared above this team</p>
            <ul className="why-lines">
              {palette.declaredLists.map((listTeam) => (
                <li key={listTeam} className="mono">
                  {listTeam}
                </li>
              ))}
            </ul>
          </>
        )
      case 'default-allow':
        return (
          <>
            <p className="why-claim">
              No Allow-list is declared for this team or any team above it
            </p>
            <p>Everything in the active Catalogue is allowed.</p>
          </>
        )
      default:
        return null
    }
  }

  return (
    <div className="palette-view">
      <div className="catalogue-filters">
        <label>
          Team
          <select
            data-testid="palette-team"
            value={team}
            onChange={(event) =>
              void navigate({
                from: '/catalogue',
                to: '/catalogue',
                search: (prev) => ({ ...prev, team: event.target.value }),
              })
            }
          >
            {flattenTeams(estate.data.teams).map((node) => (
              <option key={node.id} value={node.id}>
                {node.name}
              </option>
            ))}
          </select>
        </label>
        <span className="section-summary" data-testid="palette-summary">
          {allowed} of {palette.rows.length} Catalogue entries allowed · judged against{' '}
          {active} (active)
        </span>
      </div>
      <table className="catalogue-table" data-testid="palette-table">
        <thead>
          <tr>
            <th>Component</th>
            <th>Key</th>
            <th>Origin</th>
            <th>Why?</th>
          </tr>
        </thead>
        <tbody>
          {palette.rows.map((row) => {
            const key = formatCatalogueKey(row.entry)
            return (
              <tr
                key={key}
                data-testid={`palette-${key}`}
                className={row.allowed ? undefined : 'excluded'}
              >
                <td>{row.entry.displayName ?? row.entry.type}</td>
                <td className="mono">{key}</td>
                <td>
                  <span
                    className={chipClass('neutral', {
                      mono: true,
                      extra: `origin-chip origin-${row.allowed ? row.origin : 'excluded'}`,
                    })}
                    data-testid={`origin-${key}`}
                  >
                    {row.allowed && row.origin ? ORIGIN_LABEL[row.origin] : 'not allowed'}
                  </span>
                </td>
                <td className="why-cell">
                  <span className="why">
                    <Button
                      tone="quiet"
                      className="why-button"
                      data-testid={`palette-why-${key}`}
                      aria-expanded={openWhy === key}
                      onClick={() => setOpenWhy(openWhy === key ? undefined : key)}
                    >
                      why?
                    </Button>
                    {openWhy === key && (
                      <div className="why-popover" data-testid="palette-popover">
                        {whyContent(row)}
                      </div>
                    )}
                  </span>
                  {!row.allowed && (
                    <Link
                      from="/catalogue"
                      to="/catalogue"
                      search={(prev) => ({
                        ...prev,
                        view: 'governance' as const,
                        request: key,
                        team,
                      })}
                      className={buttonClass('secondary', 'who-acts')}
                      data-testid={`request-grant-${key}`}
                    >
                      Request a Grant
                    </Link>
                  )}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
