import { Link, useNavigate } from '@tanstack/react-router'
import type { CatalogueComponent, CatalogueEntry } from '../../api/types'
import { formatCatalogueKey } from '../../api/types'
import { formatObjectRef } from '../../objectref'
import { buttonClass } from '../../ui/Button'
import { Panel } from '../../ui/Panel'
import { chipClass } from '../../ui/Chip'
import { formatDate } from '../../ui/text'
import { STABILITY_TITLE } from './stability'

/**
 * One Catalogue entry, summoned in place (ADR-0042 §3.2): identity with
 * the resolving alias, per-signal stability, and the upstream deprecation
 * notices, ready-made remediation text (ADR-0020). The governed
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
    <Panel
      name="entry"
      testId="entry-panel"
      title={entry.displayName ?? entry.type}
      titleTestId="entry-panel-title"
      closeTestId="entry-panel-close"
      onClose={() =>
        void navigate({ to: '.', search: (prev) => ({ ...prev, object: undefined }) })
      }
    >
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
        <h3>Stability by signal</h3>
        <ul className="signal-chips">
          {Object.entries(entry.stability)
            .sort(([a], [b]) => a.localeCompare(b))
            .map(([signal, level]) => (
              <li
                key={signal}
                className={chipClass('neutral', {
                  mono: true,
                  extra: `stability-chip stability-${level}`,
                })}
                data-signal={signal}
                title={STABILITY_TITLE[level]}
              >
                {signal}: {level}
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
              <span className="lane-name" data-signal={signal}>
                {signal}
              </span>{' '}
              deprecated {formatDate(notice.date)}
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
                {/* Each governed Component links back to its own row in the
                    browse table. Inspection, not action: a bare anchor,
                    dressed by `base.css`. */}
                <Link
                  from="/catalogue"
                  to="/catalogue"
                  search={(prev) => ({
                    ...prev,
                    object: formatObjectRef({ kind: 'component', id: component.id }),
                  })}
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
        className={buttonClass('secondary', 'who-acts')}
        data-testid="entry-see-palette"
      >
        {/* The reader's question, not the surface's name (interface-text
            rule): the door answers "may my team use it?", and "effective
            palette" is the docs' word for the answer, not the reader's. */}
        See what your team may use
      </Link>
    </Panel>
  )
}
