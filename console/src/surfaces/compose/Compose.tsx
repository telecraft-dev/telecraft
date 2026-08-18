import { useQuery } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { api } from '../../api/client'
import { canActOn } from '../../auth/authz'
import { formatObjectRef, parseObjectRef } from '../../objectref'

/**
 * The Compose Workspace: Blueprint authoring (ADR-0042 §1, ADR-0043).
 * The scaffold lists Blueprints and shows the selected Blueprint's
 * per-signal lanes in renderer order; the composer surfaces build on this
 * shell against the same URL-addressable selection. A Blueprint-shaped
 * who-acts chip lands here at the offending lane (ADR-0042 §3.3), carried
 * in the `lane` search param.
 *
 * Whether a Blueprint is yours to author is the ownership tree's answer,
 * not the surface's (ADR-0019 §2): authoring is offered exactly when the
 * owning team is in the signed-in user's editableTeams, and the composer
 * surfaces (ADR-0043) gate their controls on the same answer.
 */
export function Compose() {
  const blueprints = useQuery({ queryKey: ['blueprints'], queryFn: api.blueprints })
  const me = useQuery({ queryKey: ['me'], queryFn: api.me })
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
          {me.data &&
            (canActOn(me.data, chosen.team) ? (
              <p className="authoring editable" data-testid="compose-authoring">
                Yours to author: {chosen.team} is in your remit.
              </p>
            ) : (
              <p className="authoring readonly" data-testid="compose-authoring">
                Read-only: owned by {chosen.team}. Changes route through its owners.
              </p>
            ))}
          {Object.entries(chosen.pipelines).map(([signal, lane]) => (
            <div
              key={signal}
              className={signal === search.lane ? 'signal-lane offending' : 'signal-lane'}
              data-testid={`lane-${signal}`}
            >
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
