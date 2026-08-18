import { useQuery } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { api } from '../../api/client'
import { formatObjectRef, parseObjectRef } from '../../objectref'

/**
 * The Compose Workspace: Blueprint authoring (ADR-0042 §1, ADR-0043).
 * The scaffold lists Blueprints and shows the selected Blueprint's
 * per-signal lanes in renderer order; the composer surfaces build on this
 * shell against the same URL-addressable selection.
 */
export function Compose() {
  const blueprints = useQuery({ queryKey: ['blueprints'], queryFn: api.blueprints })
  const search = useSearch({ strict: false })
  const navigate = useNavigate()

  if (blueprints.isPending) return <p className="surface-status">Loading Blueprints…</p>
  if (blueprints.isError) return <p className="surface-status">Blueprints failed to load.</p>

  const selected = parseObjectRef(search.object)
  const chosen =
    selected?.kind === 'blueprint'
      ? blueprints.data.find((bp) => bp.id === selected.id)
      : undefined

  return (
    <div className="compose-layout">
      <section className="compose-list">
        <h1>Compose</h1>
        <ul>
          {blueprints.data.map((bp) => (
            <li key={bp.id}>
              <button
                type="button"
                data-testid={`blueprint-${bp.id}`}
                className={bp.id === chosen?.id ? 'list-item active' : 'list-item'}
                onClick={() =>
                  void navigate({
                    to: '/compose',
                    search: (prev) => ({
                      ...prev,
                      object: formatObjectRef({ kind: 'blueprint', id: bp.id }),
                    }),
                  })
                }
              >
                <span className="item-name">{bp.name}</span>
                <span className="item-meta">
                  v{bp.version} · {bp.team}
                </span>
              </button>
            </li>
          ))}
        </ul>
      </section>
      {chosen && (
        <section className="compose-detail" data-testid="compose-detail">
          <h2>
            {chosen.name} <span className="item-meta">v{chosen.version}</span>
          </h2>
          {Object.entries(chosen.pipelines).map(([signal, lane]) => (
            <div key={signal} className="signal-lane">
              <h3>{signal}</h3>
              <ol className="lane-components">
                {lane.map((component, i) => (
                  <li key={`${component}-${i}`}>{component}</li>
                ))}
              </ol>
            </div>
          ))}
        </section>
      )}
    </div>
  )
}
