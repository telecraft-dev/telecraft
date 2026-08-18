import { useQuery } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { api } from '../../api/client'
import { formatObjectRef, parseObjectRef } from '../../objectref'

/**
 * The Catalogue & Governance Workspace: browse-and-request, thin, not a
 * workspace of surfaces (ADR-0042 §1). The scaffold browses governed
 * Components at their pinned versions (ADR-0020, ADR-0026).
 */
export function Catalogue() {
  const catalogue = useQuery({ queryKey: ['catalogue'], queryFn: api.catalogue })
  const search = useSearch({ strict: false })
  const navigate = useNavigate()

  if (catalogue.isPending) return <p className="surface-status">Loading the catalogue…</p>
  if (catalogue.isError) return <p className="surface-status">The catalogue failed to load.</p>

  const selected = parseObjectRef(search.object)

  return (
    <div className="catalogue-layout">
      <h1>Catalogue &amp; Governance</h1>
      <table className="catalogue-table" data-testid="catalogue-table">
        <thead>
          <tr>
            <th>Component</th>
            <th>Version</th>
            <th>Class</th>
            <th>Type</th>
            <th>Owning team</th>
          </tr>
        </thead>
        <tbody>
          {catalogue.data.map((component) => (
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
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
