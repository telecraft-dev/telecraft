/**
 * The four Workspaces, and the only top-level entries navigation has
 * (ADR-0042 §1). They live here rather than inside the shell because two
 * things need the same list: the chrome that renders them, and the deploy
 * that pre-renders an entry document for each of their URLs so a static
 * host answers them 200 rather than falling back to its not-found page
 * (ADR-0042 §3.5). A Workspace added in one place and not the other is
 * exactly the drift `tests/site.test.ts` refuses.
 */
export const WORKSPACES = [
  { to: '/estate', label: 'Estate', testid: 'nav-estate' },
  { to: '/topology', label: 'Topology', testid: 'nav-topology' },
  { to: '/compose', label: 'Compose', testid: 'nav-compose' },
  { to: '/catalogue', label: 'Catalogue & Governance', testid: 'nav-catalogue' },
] as const
