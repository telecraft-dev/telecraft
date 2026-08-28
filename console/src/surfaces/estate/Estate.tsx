import { useQuery } from '@tanstack/react-query'
import { Link, useSearch } from '@tanstack/react-router'
import { api } from '../../api/client'
import type { EstateView } from '../../router'
import { parseObjectRef } from '../../objectref'
import { buttonClass } from '../../ui/Button'
import { AddTierPanel } from './AddTier'
import { CardPanel } from './card'
import { ClaimPanel } from './Claim'
import { FlatList, herdIds } from './FlatList'
import { RollUp } from './RollUp'
import { Shelf } from './Shelf'

// The Estate Workspace (ADR-0042 §1): the shelf lands; the tree-table
// roll-up and the flat filter-first list are view-switchers over the same
// model: switching preserves selection, filters, and lens (rule 3.1). The
// ADR-0041 card panel is one universal component summoned in place beside
// whichever view holds the selection: inspection never navigates (rule 3.2).

const VIEWS: { view: EstateView; label: string; testid: string }[] = [
  { view: 'shelf', label: 'Shelf', testid: 'view-shelf' },
  { view: 'rollup', label: 'Roll-up', testid: 'view-rollup' },
  { view: 'list', label: 'Flat list', testid: 'view-list' },
]

export function Estate() {
  const estate = useQuery({ queryKey: ['estate'], queryFn: api.estate })
  const search = useSearch({ from: '/estate' })

  if (estate.isPending) return <p className="surface-status">Loading the estate…</p>
  if (estate.isError) return <p className="surface-status">The estate payload failed to load.</p>

  const payload = estate.data
  const view: EstateView = search.view ?? 'shelf'
  const selected = parseObjectRef(search.object)
  const selectedCard =
    selected?.kind === 'tier'
      ? payload.cards.find((card) => card.tier === selected.id)
      : undefined
  // The claim flow's herd (ADR-0042 §6): a non-empty selection summons the
  // claim panel in place of the card panel. View-switching preserves it
  // (rule 3.1), since the selection lives in the URL.
  const herd = herdIds(search.herd)

  return (
    <div className="estate-layout">
      <div className="estate-main">
        <header className="estate-header">
          <h1>Estate</h1>
          <nav className="view-switcher" aria-label="Estate views">
            {VIEWS.map(({ view: v, label, testid }) => (
              <Link
                key={v}
                from="/estate"
                to="/estate"
                search={(prev) => ({ ...prev, view: v })}
                className={v === view ? 'scope-link active' : 'scope-link'}
                data-testid={testid}
              >
                {label}
              </Link>
            ))}
          </nav>
          {/* The shelf's scope shares the title row with the views
              (ADR-0063 §1): one row of switchers, the cards higher. It
              still scopes only the shelf (ADR-0042 §2), so the other
              views do not show it. */}
          {view === 'shelf' && (
            <nav className="shelf-scope" aria-label="Shelf scope">
              <Link
                from="/estate"
                to="/estate"
                search={(prev) => ({ ...prev, scope: 'team' as const })}
                className={(search.scope ?? 'team') === 'team' ? 'scope-link active' : 'scope-link'}
                data-testid="scope-team"
              >
                My team
              </Link>
              <Link
                from="/estate"
                to="/estate"
                search={(prev) => ({ ...prev, scope: 'estate' as const })}
                className={search.scope === 'estate' ? 'scope-link active' : 'scope-link'}
                data-testid="scope-estate"
              >
                Whole estate
              </Link>
            </nav>
          )}
          {/* The governance-first door into the Add-a-Tier flow (ADR-0060
              §1); the claim flow's draft branch is the other door. */}
          <Link
            from="/estate"
            to="/estate"
            search={(prev) => ({ ...prev, add: true })}
            className={buttonClass('secondary', 'add-tier-door')}
            data-testid="add-tier"
          >
            Add a Tier
          </Link>
        </header>
        {view === 'shelf' && <Shelf payload={payload} selectedTier={selectedCard?.tier} />}
        {view === 'rollup' && <RollUp payload={payload} />}
        {view === 'list' && <FlatList payload={payload} selectedTier={selectedCard?.tier} />}
      </div>
      {/* One panel at a time: the Add-a-Tier flow first (ADR-0060 §1),
          then the claim flow's herd, then the card. */}
      {search.add ? (
        <AddTierPanel payload={payload} />
      ) : herd.length > 0 ? (
        <ClaimPanel payload={payload} herd={herd} />
      ) : (
        selectedCard && <CardPanel card={selectedCard} />
      )}
    </div>
  )
}
