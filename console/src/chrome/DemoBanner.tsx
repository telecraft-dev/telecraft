import { useQuery } from '@tanstack/react-query'
import { demoMeta } from '../api/demo'
import { Chip } from '../ui/Chip'

/**
 * The demo's own statement of what it is (issue #50): a build-time
 * snapshot of one estate, not a live instance. It lives in the profile
 * menu, where the session controls live on an instance, because in the
 * demo there is no session to end (issue #182); the menu's trigger carries
 * the read-only wording, so the statement is still one glance away. It
 * names the estate and the commit the snapshot was taken at, so what is on
 * screen is traceable to a reviewable tree.
 *
 * The repository is shown as text, never as a link: the bundle references
 * no external host at all (ADR-0045 §5), and that rule does not bend for a
 * convenience.
 */
export function DemoBanner() {
  const meta = useQuery({ queryKey: ['demo-meta'], queryFn: demoMeta, staleTime: Infinity })

  return (
    <span className="demo-banner" data-testid="demo-banner">
      <Chip className="demo-badge">Read-only demo</Chip>
      {meta.data && (
        <span
          className="demo-provenance"
          data-testid="demo-provenance"
          title={`${meta.data.repository ?? 'the demo estate'} at ${meta.data.commit}, evaluated ${formatInstant(meta.data.evaluatedAt)}`}
        >
          {meta.data.repository ?? 'the demo estate'} at{' '}
          <code>{meta.data.commit.slice(0, 7)}</code>, evaluated{' '}
          <time dateTime={meta.data.evaluatedAt}>{formatInstant(meta.data.evaluatedAt)}</time>
        </span>
      )}
    </span>
  )
}

/** Renders an instant in the reader's locale, ISO date first so it sorts. */
function formatInstant(iso: string): string {
  const at = new Date(iso)
  if (Number.isNaN(at.getTime())) return iso
  return at.toISOString().replace('T', ' ').slice(0, 16) + 'Z'
}
