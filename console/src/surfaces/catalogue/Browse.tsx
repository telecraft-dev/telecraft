import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { api } from '../../api/client'
import type { StabilityLevel } from '../../api/types'
import { STABILITY_LEVELS, formatCatalogueKey } from '../../api/types'
import { formatObjectRef, parseObjectRef } from '../../objectref'
import { EntryPanel } from './EntryPanel'

/**
 * Catalogue browsing by version (ADR-0020 §9: installed catalogues are
 * retained, never replaced — the picker consults the version asked for),
 * filtered by stability and signal. Selecting an entry summons its panel
 * in place; the governed Components table below shows the configured
 * instances of those types, each deep-linking to its Catalogue entry.
 */
export function Browse() {
  const versions = useQuery({ queryKey: ['catalogue-versions'], queryFn: api.catalogueVersions })
  const governed = useQuery({ queryKey: ['catalogue'], queryFn: api.catalogue })
  const search = useSearch({ from: '/catalogue' })
  const navigate = useNavigate()

  const version = search.version ?? versions.data?.active
  const entries = useQuery({
    queryKey: ['catalogue-entries', version],
    queryFn: () => api.catalogueEntries(version as string),
    enabled: version !== undefined,
  })

  if (versions.isPending || governed.isPending || entries.isPending) {
    return <p className="surface-status">Loading the catalogue…</p>
  }
  if (versions.isError || governed.isError || entries.isError) {
    return <p className="surface-status">The catalogue failed to load.</p>
  }

  const stability = search.stability
  const signal = search.signal
  const signals = [...new Set(entries.data.flatMap((entry) => Object.keys(entry.stability)))].sort()

  // Together the filters name the signal at the level; alone, each filters
  // on any signal.
  const visible = entries.data.filter((entry) => {
    if (signal && stability) return entry.stability[signal] === stability
    if (signal) return signal in entry.stability
    if (stability) return Object.values(entry.stability).includes(stability as StabilityLevel)
    return true
  })

  const selected = parseObjectRef(search.object)
  const selectedEntry =
    selected?.kind === 'entry'
      ? entries.data.find((entry) => formatCatalogueKey(entry) === selected.id)
      : undefined

  const setParam = (key: 'version' | 'stability' | 'signal', value: string) =>
    void navigate({
      from: '/catalogue',
      to: '/catalogue',
      search: (prev) => ({ ...prev, [key]: value === '' ? undefined : value }),
    })

  return (
    <div className="browse-layout">
      <div className="browse-main">
        <div className="catalogue-filters">
          <label>
            Catalogue
            <select
              data-testid="version-select"
              value={version}
              onChange={(event) => setParam('version', event.target.value)}
            >
              {versions.data.versions.map((v) => (
                <option key={v.version} value={v.version}>
                  {v.version}
                  {v.active ? ' (active)' : ''} · {v.components} entries
                </option>
              ))}
            </select>
          </label>
          <label>
            Stability
            <select
              data-testid="filter-stability"
              value={stability ?? ''}
              onChange={(event) => setParam('stability', event.target.value)}
            >
              <option value="">any</option>
              {STABILITY_LEVELS.map((level) => (
                <option key={level} value={level}>
                  {level}
                </option>
              ))}
            </select>
          </label>
          <label>
            Signal
            <select
              data-testid="filter-signal"
              value={signal ?? ''}
              onChange={(event) => setParam('signal', event.target.value)}
            >
              <option value="">any</option>
              {signals.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </label>
        </div>
        <table className="catalogue-table" data-testid="entries-table">
          <thead>
            <tr>
              <th>Class</th>
              <th>Type</th>
              <th>Name</th>
              <th>Per-signal stability</th>
              <th>Source</th>
            </tr>
          </thead>
          <tbody>
            {visible.map((entry) => {
              const key = formatCatalogueKey(entry)
              return (
                <tr
                  key={key}
                  data-testid={`entry-${key}`}
                  className={selectedEntry === entry ? 'selected' : undefined}
                  onClick={() =>
                    void navigate({
                      from: '/catalogue',
                      to: '/catalogue',
                      search: (prev) => ({
                        ...prev,
                        object: formatObjectRef({ kind: 'entry', id: key }),
                      }),
                    })
                  }
                >
                  <td>{entry.class}</td>
                  <td className="mono">
                    {entry.type}
                    {entry.deprecatedType && (
                      <span className="item-meta"> (was {entry.deprecatedType})</span>
                    )}
                  </td>
                  <td>{entry.displayName}</td>
                  <td>
                    <ul className="signal-chips">
                      {Object.keys(entry.stability)
                        .sort()
                        .map((s) => (
                          <li key={s} className={`stability-chip stability-${entry.stability[s]}`}>
                            {s}: {entry.stability[s]}
                          </li>
                        ))}
                    </ul>
                  </td>
                  <td>{entry.source}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
        <h2 className="catalogue-subhead">Governed Components</h2>
        <p className="section-summary">
          Configured instances of Catalogue types, at their pinned versions (ADR-0026).
        </p>
        <table className="catalogue-table" data-testid="catalogue-table">
          <thead>
            <tr>
              <th>Component</th>
              <th>Version</th>
              <th>Class</th>
              <th>Type</th>
              <th>Owning team</th>
              <th>Catalogue entry</th>
            </tr>
          </thead>
          <tbody>
            {governed.data.map((component) => (
              <tr
                key={component.id}
                data-testid={`component-${component.id}`}
                className={
                  selected?.kind === 'component' && selected.id === component.id
                    ? 'selected'
                    : undefined
                }
                onClick={() =>
                  void navigate({
                    from: '/catalogue',
                    to: '/catalogue',
                    search: (prev) => ({
                      ...prev,
                      object: formatObjectRef({ kind: 'component', id: component.id }),
                    }),
                  })
                }
              >
                <td>{component.name}</td>
                <td>{component.version}</td>
                <td>{component.class}</td>
                <td>{component.type}</td>
                <td>{component.team}</td>
                <td>
                  <Link
                    from="/catalogue"
                    to="/catalogue"
                    search={(prev) => ({
                      ...prev,
                      object: formatObjectRef({
                        kind: 'entry',
                        id: `${component.class}/${component.type}`,
                      }),
                    })}
                    className="who-acts"
                    data-testid={`component-entry-${component.id}`}
                    onClick={(event) => event.stopPropagation()}
                  >
                    {component.class}/{component.type}
                  </Link>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {selectedEntry && version !== undefined && (
        <EntryPanel
          entry={selectedEntry}
          version={version}
          active={version === versions.data.active}
          governed={governed.data}
        />
      )}
    </div>
  )
}
