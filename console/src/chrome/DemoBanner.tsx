import { useQuery } from '@tanstack/react-query'
import { demoMeta } from '../api/demo'

/**
 * The demo's own statement of what it is (issue #50): a build-time
 * snapshot of one estate, not a live instance. It stands where the
 * sign-out control does on an instance, because in the demo there is no
 * session to end — and it names the estate and the commit the snapshot was
 * taken at, so what is on screen is traceable to a reviewable tree.
 *
 * The repository is shown as text, never as a link: the bundle references
 * no external host at all (ADR-0045 §5), and that rule does not bend for a
 * convenience.
 */
export function DemoBanner() {
  const meta = useQuery({ queryKey: ['demo-meta'], queryFn: demoMeta, staleTime: Infinity })

  return (
    <span className="demo-banner" data-testid="demo-banner">
      <strong className="demo-badge">Read-only demo</strong>
      {meta.data && (
        <span className="demo-provenance" data-testid="demo-provenance">
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
