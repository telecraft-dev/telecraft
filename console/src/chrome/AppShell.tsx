import { Link, Outlet } from '@tanstack/react-router'
import { JumpToObject } from './JumpToObject'
import { LensControl } from './LensControl'
import { ProfileMenu } from './ProfileMenu'
import { WORKSPACES } from './workspaces'
import { TourControl } from '../tours/TourControl'
import { TourRunner } from '../tours/TourRunner'

// Navigation is activity-first: the five Workspaces are the only top-level
// entries (ADR-0042 §1, amended by ADR-0056 §1). Switching Workspaces keeps
// the lens (one control, in the context strip, ADR-0058) and a running Tour
// (ADR-0051 §3, which rides with it), and drops the object selection, which
// belongs to the Workspace that read it. The list itself lives in
// ./workspaces, because the deploy pre-renders an entry document per
// Workspace URL and the two must not drift.

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
        {/* Search leads the cluster, then the Tour control, then the
            profile button (issue #182). The lens left the bar for the
            context strip (ADR-0058): the chrome keeps what is about getting
            around and about the reader, and evaluation context sits with
            the surfaces it contextualises. */}
        <div className="chrome-controls">
          <JumpToObject />
          <TourControl />
          <ProfileMenu />
        </div>
      </header>
      <main className="workspace-body">
        {/* The context strip (ADR-0058): the lens, and in time the other
            surface-level context controls, on one line above every
            Workspace. The surface below it scrolls; the strip does not. */}
        <div className="context-strip">
          <LensControl />
        </div>
        <div className="workspace-surface">
          <Outlet />
        </div>
      </main>
      {/* Above every surface and beneath none of them: the runner draws
          whatever Tour the URL names, and nothing when it names none. */}
      <TourRunner />
    </div>
  )
}
