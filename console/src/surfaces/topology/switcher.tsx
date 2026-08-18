import { Link } from '@tanstack/react-router'
import type { TopologyView } from '../../router'

// The Topology view-switchers (ADR-0042 §1): the flow canvas and the
// rollout ledger are complementary representations of one model.
// Switching preserves selection and lens (rule 3.1) and stays
// URL-addressable (rule 3.5).

const VIEWS: { view: TopologyView; label: string; testid: string }[] = [
  { view: 'flow', label: 'Flow canvas', testid: 'view-flow' },
  { view: 'rollout', label: 'Rollouts', testid: 'view-rollout' },
]

export function TopologyViewSwitcher({ active }: { active: TopologyView }) {
  return (
    <nav className="view-switcher" aria-label="Topology views">
      {VIEWS.map(({ view, label, testid }) => (
        <Link
          key={view}
          from="/topology"
          to="/topology"
          search={(prev) => ({
            ...prev,
            view,
            // Leaving the ledger drops a Rollout selection with it: the
            // flow canvas has nowhere to summon that panel. A Tier or
            // Service selection survives the switch (rule 3.1).
            object:
              view === 'flow' && prev.object?.startsWith('rollout:') ? undefined : prev.object,
          })}
          className={view === active ? 'scope-link active' : 'scope-link'}
          data-testid={testid}
        >
          {label}
        </Link>
      ))}
    </nav>
  )
}
