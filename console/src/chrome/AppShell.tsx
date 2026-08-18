import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, Outlet } from '@tanstack/react-router'
import { api } from '../api/client'
import { demoMode } from '../api/demo'
import { DemoBanner } from './DemoBanner'
import { JumpToObject } from './JumpToObject'
import { LensControl } from './LensControl'

// Navigation is activity-first: the four Workspaces are the only top-level
// entries (ADR-0042 §1). Switching Workspaces keeps the lens (one global
// chrome control) and drops the object selection, which belongs to the
// Workspace that read it.
const WORKSPACES = [
  { to: '/estate', label: 'Estate', testid: 'nav-estate' },
  { to: '/topology', label: 'Topology', testid: 'nav-topology' },
  { to: '/compose', label: 'Compose', testid: 'nav-compose' },
  { to: '/catalogue', label: 'Catalogue & Governance', testid: 'nav-catalogue' },
] as const

export function AppShell() {
  const me = useQuery({ queryKey: ['me'], queryFn: api.me })
  const queryClient = useQueryClient()
  const signOut = useMutation({
    mutationFn: api.logout,
    // Dropping the me query flips the auth gate back to the sign-in
    // surface; everything else is stale with it.
    onSuccess: () => queryClient.resetQueries(),
  })

  return (
    <div className="shell">
      <header className="chrome">
        <span className="brand">Telecraft</span>
        <nav className="workspaces" aria-label="Workspaces">
          {WORKSPACES.map((ws) => (
            <Link
              key={ws.to}
              to={ws.to}
              search={(prev) => ({ lens: prev.lens })}
              data-testid={ws.testid}
              className="workspace-link"
              activeProps={{ className: 'workspace-link active', 'aria-current': 'page' }}
            >
              {ws.label}
            </Link>
          ))}
        </nav>
        <div className="chrome-controls">
          <LensControl />
          <JumpToObject />
          {me.data && (
            <span className="chrome-user" data-testid="chrome-user">
              {me.data.name}
            </span>
          )}
          {/* The demo has no session to end, so it says what it is
              instead (issue #50). */}
          {demoMode ? (
            <DemoBanner />
          ) : (
            <button
              type="button"
              className="sign-out"
              data-testid="sign-out"
              onClick={() => signOut.mutate()}
            >
              Sign out
            </button>
          )}
        </div>
      </header>
      <main className="workspace-body">
        <Outlet />
      </main>
    </div>
  )
}
