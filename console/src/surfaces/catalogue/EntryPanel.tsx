import { Link, useNavigate } from '@tanstack/react-router'
import type { CatalogueComponent, CatalogueEntry } from '../../api/types'
import { formatCatalogueKey } from '../../api/types'
import { formatObjectRef } from '../../objectref'

/**
 * One Catalogue entry, summoned in place (ADR-0042 §3.2): identity with
 * the resolving alias, per-signal stability, and the upstream deprecation
 * notices — ready-made remediation text (ADR-0020). The governed
 * Components instantiating this type link back, and the palette door
 * answers "may my team use it?".
 */
export function EntryPanel({
  entry,
  version,
  active,
  governed,
}: {
  entry: CatalogueEntry
  version: string
  active: boolean
  governed: CatalogueComponent[]
}) {
  const navigate = useNavigate()
  const key = formatCatalogueKey(entry)
  const instances = governed.filter(
    (component) => component.class === entry.class && component.type === entry.type,
  )
  const deprecations = Object.entries(entry.deprecation ?? {})

  return (
    <aside className="card-panel" data-testid="entry-panel">
      <header className="panel-head">
        <h2 data-testid="entry-panel-title">{entry.displayName ?? entry.type}</h2>
        <button
          type="button"
          data-testid="entry-panel-close"
          onClick={() =>
            void navigate({ to: '.', search: (prev) => ({ ...prev, object: undefined }) })
          }
        >
          Close
        </button>
      </header>
      <dl className="panel-facts">
        <dt>Key</dt>
        <dd className="mono">{key}</dd>
        {entry.deprecatedType && (
          <>
            <dt>Was</dt>
            <dd className="mono">{entry.deprecatedType}</dd>
          </>
        )}
        <dt>Source</dt>
        <dd>{entry.source}</dd>
        <dt>Catalogue</dt>
        <dd>
          {version}
          {active ? ' (active)' : ''}
        </dd>
      </dl>
      {entry.description && <p className="entry-description">{entry.description}</p>}
      <section>
        <h3>Per-signal stability</h3>
        <ul className="signal-chips">
          {Object.keys(entry.stability)
            .sort()
            .map((signal) => (
              <li key={signal} className={`stability-chip stability-${entry.stability[signal]}`}>
                {signal}: {entry.stability[signal]}
              </li>
            ))}
        </ul>
        {deprecations.map(([signal, notice]) => (
          <div
            key={signal}
            className="entry-deprecation"
            data-testid={`deprecation-${signal}`}
          >
            <strong>
              {signal} deprecated {notice.date}
            </strong>
            <p>{notice.migration}</p>
          </div>
        ))}
      </section>
      <section data-testid="entry-instances">
        <h3>Governed Components of this type</h3>
        {instances.length === 0 ? (
          <p className="section-summary">None in the estate.</p>
        ) : (
          <ul className="entry-instance-list">
            {instances.map((component) => (
              <li key={component.id}>
                <Link
                  from="/catalogue"
                  to="/catalogue"
                  search={(prev) => ({
                    ...prev,
                    object: formatObjectRef({ kind: 'component', id: component.id }),
                  })}
                  className="who-acts"
                  data-testid={`entry-instance-${component.id}`}
                >
                  {component.id}@{component.version}
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>
      <Link
        from="/catalogue"
        to="/catalogue"
        search={(prev) => ({ ...prev, view: 'palette' as const })}
        className="who-acts"
        data-testid="entry-see-palette"
      >
        Is this allowed? See the effective palette
      </Link>
    </aside>
  )
}
