import { Link, Outlet } from '@tanstack/react-router'
import { JumpToObject } from './JumpToObject'
import { LensControl } from './LensControl'
import { ProfileMenu } from './ProfileMenu'
import { WORKSPACES } from './workspaces'
import { TourControl } from '../tours/TourControl'
import { TourRunner } from '../tours/TourRunner'

// Navigation is activity-first: the five Workspaces are the only top-level
// entries (ADR-0042 §1, amended by ADR-0056 §1). Switching Workspaces keeps
// the lens (one global chrome control) and a running Tour (ADR-0051 §3,
// which rides with it), and drops the object selection, which belongs to
// the Workspace that read it. The list itself lives in ./workspaces, because
// the deploy pre-renders an entry document per Workspace URL and the two
// must not drift.

export function AppShell() {
  return (
    <div className="shell">
      <header className="chrome">
        <span className="brand">Telecraft</span>
        <nav className="workspaces" aria-label="Workspaces">
          {WORKSPACES.map((ws) => (
            <Link
              key={ws.to}
              to={ws.to}
              search={(prev) => ({ lens: prev.lens, tour: prev.tour, step: prev.step })}
              data-testid={ws.testid}
              className="workspace-link"
              // Home is `/`, which prefix-matches every other Workspace, so
              // it alone is matched exactly. Without this the landing entry
              // reads as pressed on every surface (the ADR-0048 hazard: the
              // router stamps `active` on any Link whose route matches).
              activeOptions={{ exact: ws.to === '/' }}
              activeProps={{ className: 'workspace-link active', 'aria-current': 'page' }}
            >
              {ws.label}
            </Link>
          ))}
        </nav>
        {/* Search leads the cluster, then the compact controls, then the
            profile button (issue #182). The lens keeps its word at every
            width: it changes what every number on the page means, where the
            theme only changes how the page is drawn, so the theme rides in
            the profile menu and the lens does not (issue #183). */}
        <div className="chrome-controls">
          <JumpToObject />
          <LensControl />
          <TourControl />
          <ProfileMenu />
        </div>
      </header>
      <main className="workspace-body">
        <Outlet />
      </main>
      {/* Above every surface and beneath none of them: the runner draws
          whatever Tour the URL names, and nothing when it names none. */}
      <TourRunner />
    </div>
  )
}
