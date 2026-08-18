import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { useState } from 'react'
import { api } from '../../api/client'
import { BAND_ORDER, type CardFace, type EstatePayload, type TeamNode } from '../../api/types'
import { useLens } from '../../chrome/LensControl'
import { cardStanding, orderCards, sectionAllHealthy, totalFindings } from '../../estate/order'
import { formatObjectRef, parseObjectRef } from '../../objectref'
import { usePresentation } from '../../presentation/usePresentation'
import { CardFaceView, CardPanel } from './card'

interface Section {
  team: TeamNode
  cards: CardFace[]
}

/** Depth-first sections: one per team that owns cards, tree order. */
function sections(root: TeamNode, cards: CardFace[]): Section[] {
  const out: Section[] = []
  const walk = (team: TeamNode) => {
    const owned = cards.filter((card) => card.team === team.id)
    if (owned.length > 0) out.push({ team, cards: owned })
    for (const child of team.teams ?? []) walk(child)
  }
  walk(root)
  return out
}

/** The subtree rooted at a team id, or undefined when absent. */
function subtree(root: TeamNode, id: string): TeamNode | undefined {
  if (root.id === id) return root
  for (const child of root.teams ?? []) {
    const found = subtree(child, id)
    if (found) return found
  }
  return undefined
}

/**
 * The Estate landing surface (ADR-0042 §2): team-subtree sections crossed
 * with aligned Environment rows, cards ordered worst-severity-first from
 * face summary fields alone. Scope rests on the signed-in user's team
 * subtree; one click widens to the estate.
 */
export function Shelf() {
  const estate = useQuery({ queryKey: ['estate'], queryFn: api.estate })
  const search = useSearch({ strict: false })
  const { me, store } = usePresentation()
  const lens = useLens()

  if (estate.isPending) return <p className="surface-status">Loading the estate…</p>
  if (estate.isError) return <p className="surface-status">The estate payload failed to load.</p>

  const payload = estate.data
  const selected = parseObjectRef(search.object)
  const scope = search.scope ?? 'team'

  // A team selection scopes the shelf to that subtree; otherwise the
  // resting scope is the signed-in user's team subtree (ADR-0042 §2).
  const scopeTeam =
    selected?.kind === 'team'
      ? selected.id
      : scope === 'team'
        ? (me?.team ?? payload.teams.id)
        : payload.teams.id
  const root = subtree(payload.teams, scopeTeam) ?? payload.teams
  const visible = sections(root, payload.cards)

  // Environment rows: production leads by default; the lens leads when it
  // names another Environment — emphasis, never a filter (ADR-0042 §4).
  const environments = [lens, ...payload.environments.filter((env) => env !== lens)]

  const selectedCard =
    selected?.kind === 'tier'
      ? payload.cards.find((card) => card.tier === selected.id)
      : undefined

  return (
    <div className="shelf-layout">
      <section className="shelf" data-testid="shelf">
        <header className="shelf-header">
          <h1>Estate</h1>
          <div className="shelf-scope">
            <Link
              to="/estate"
              search={(prev) => ({ lens: prev.lens, scope: 'team' as const })}
              className={scope === 'team' ? 'scope-link active' : 'scope-link'}
              data-testid="scope-team"
            >
              My team
            </Link>
            <Link
              to="/estate"
              search={(prev) => ({ lens: prev.lens, scope: 'estate' as const })}
              className={scope === 'estate' ? 'scope-link active' : 'scope-link'}
              data-testid="scope-estate"
            >
              Whole estate
            </Link>
          </div>
        </header>
        {visible.map(({ team, cards }) => (
          <ShelfSection
            key={team.id}
            team={team}
            cards={cards}
            environments={environments}
            store={store}
            selectedTier={selectedCard?.tier}
          />
        ))}
      </section>
      {selectedCard && <CardPanel card={selectedCard} />}
    </div>
  )
}

function ShelfSection({
  team,
  cards,
  environments,
  store,
  selectedTier,
}: {
  team: TeamNode
  cards: CardFace[]
  environments: EstatePayload['environments']
  store: ReturnType<typeof usePresentation>['store']
  selectedTier?: string
}) {
  const allHealthy = sectionAllHealthy(cards)
  // All-healthy sections collapse to a summary line (ADR-0042 §2); the
  // user's explicit toggle overrides the default and persists per user.
  const [override, setOverride] = useState<boolean | undefined>(
    store.load().collapsedSections[team.id],
  )
  const collapsed = override ?? allHealthy
  const navigate = useNavigate()

  const toggle = () => {
    const next = !collapsed
    setOverride(next)
    store.save({ collapsedSections: { ...store.load().collapsedSections, [team.id]: next } })
  }

  return (
    <section className="shelf-section" data-testid={`section-${team.id}`}>
      <header className="section-header">
        <h2>{team.name}</h2>
        <button type="button" onClick={toggle} data-testid={`section-toggle-${team.id}`}>
          {collapsed ? 'Expand' : 'Collapse'}
        </button>
      </header>
      {collapsed ? (
        <p className="section-summary" data-testid={`section-summary-${team.id}`}>
          {cards.length} {cards.length === 1 ? 'Tier' : 'Tiers'},{' '}
          {allHealthy ? 'all healthy' : `${cards.reduce((n, c) => n + totalFindings(c), 0)} findings`}
        </p>
      ) : (
        environments.map((env) => {
          const row = orderCards(cards.filter((card) => card.environment === env))
          if (row.length === 0) return null
          return (
            <div key={env} className="environment-row" data-environment={env}>
              <h3 className="environment-label">{env}</h3>
              <div className="card-row">
                {row.map((card) => (
                  <CardFaceView
                    key={card.tier}
                    card={card}
                    standing={cardStanding(card)}
                    bands={BAND_ORDER}
                    selected={card.tier === selectedTier}
                    onSelect={() =>
                      void navigate({
                        to: '/estate',
                        search: (prev) => ({
                          ...prev,
                          object: formatObjectRef({ kind: 'tier', id: card.tier }),
                        }),
                      })
                    }
                  />
                ))}
              </div>
            </div>
          )
        })
      )}
    </section>
  )
}
