import { Link, useSearch } from '@tanstack/react-router'
import type { CatalogueView } from '../../router'
import { Browse } from './Browse'
import { GovernanceView } from './Governance'
import { PaletteView } from './Palette'

// The Catalogue & Governance Workspace (ADR-0042 §1): browse-and-request,
// thin. Three view-switchers over one model — browse the retained
// catalogues (ADR-0020 §9), a team's effective palette with total
// provenance (ADR-0021), and the governance editor whose Allow-list and
// Grant edits exit as PRs via the forge adapter (ADR-0042 §6). Switching
// preserves selection, filters, and lens (rule 3.1).

const VIEWS: { view: CatalogueView; label: string; testid: string }[] = [
  { view: 'browse', label: 'Browse', testid: 'view-browse' },
  { view: 'palette', label: 'Effective palette', testid: 'view-palette' },
  { view: 'governance', label: 'Governance', testid: 'view-governance' },
]

export function Catalogue() {
  const search = useSearch({ from: '/catalogue' })
  const view: CatalogueView = search.view ?? 'browse'

  return (
    <div className="catalogue-layout">
      <div className="catalogue-main">
        <header className="catalogue-header">
          <h1>Catalogue &amp; Governance</h1>
          <nav className="view-switcher" aria-label="Catalogue views">
            {VIEWS.map(({ view: v, label, testid }) => (
              <Link
                key={v}
                from="/catalogue"
                to="/catalogue"
                search={(prev) => ({ ...prev, view: v })}
                className={v === view ? 'scope-link active' : 'scope-link'}
                data-testid={testid}
              >
                {label}
              </Link>
            ))}
          </nav>
        </header>
        {view === 'browse' && <Browse />}
        {view === 'palette' && <PaletteView />}
        {view === 'governance' && <GovernanceView />}
      </div>
    </div>
  )
}
