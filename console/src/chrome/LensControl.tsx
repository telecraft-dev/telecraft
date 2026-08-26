import { useQuery } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { api } from '../api/client'
import { usePresentation } from '../presentation/usePresentation'

export const DEFAULT_LENS = 'production'

/**
 * Resolves the environment lens (ADR-0042 §4): an explicit lens in the URL
 * beats the persisted preference; a bare URL falls back to the preference,
 * and failing that to `production` (ADR-0033).
 */
export function useLens(): string {
  const search = useSearch({ strict: false })
  const { store } = usePresentation()
  return search.lens ?? store.load().lens ?? DEFAULT_LENS
}

/**
 * The one lens control, in the context strip above every Workspace
 * (ADR-0058): emphasis and evaluation context, never a hard filter
 * (ADR-0042 §4). Choosing a lens writes it to the URL (so the state stays
 * citable) and persists it as the per-user preference.
 */
export function LensControl() {
  const estate = useQuery({ queryKey: ['estate'], queryFn: api.estate })
  const lens = useLens()
  const { store } = usePresentation()
  const navigate = useNavigate()

  const environments = estate.data?.environments ?? [DEFAULT_LENS]
  return (
    <label className="lens-control" data-tour="lens">
      <span>Lens</span>
      <select
        data-testid="lens-control"
        value={lens}
        onChange={(event) => {
          const next = event.target.value
          store.save({ lens: next })
          void navigate({
            to: '.',
            search: (prev) => ({ ...prev, lens: next }),
          })
        }}
      >
        {environments.map((env) => (
          <option key={env} value={env}>
            {env}
          </option>
        ))}
      </select>
    </label>
  )
}
