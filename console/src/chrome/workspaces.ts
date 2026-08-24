/**
 * The five Workspaces, and the only top-level entries navigation has
 * (ADR-0042 §1, amended by ADR-0056 §1). They live here rather than inside
 * the shell because two things need the same list: the chrome that renders
 * them, and the deploy that pre-renders an entry document for each of their
 * URLs so a static host answers them 200 rather than falling back to its
 * not-found page (ADR-0042 §3.5). A Workspace added in one place and not
 * the other is exactly the drift `tests/site.test.ts` refuses.
 *
 * Home leads, and is the one entry named for a place rather than an
 * activity: the activity it serves is choosing which activity (ADR-0056
 * §1). It needs no pre-rendered document of its own, because `index.html`
 * is already what `/` resolves to; `tools/assemble-site.mjs` says so where
 * it skips it.
 */
export const WORKSPACES = [
  { to: '/', label: 'Home', testid: 'nav-home' },
  { to: '/estate', label: 'Estate', testid: 'nav-estate' },
  { to: '/topology', label: 'Topology', testid: 'nav-topology' },
  { to: '/compose', label: 'Compose', testid: 'nav-compose' },
  { to: '/catalogue', label: 'Catalogue & Governance', testid: 'nav-catalogue' },
] as const
