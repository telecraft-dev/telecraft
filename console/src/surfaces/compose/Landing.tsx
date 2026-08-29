import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { api } from '../../api/client'
import type { ActivationsPayload, BlueprintDoc, Me, TeamNode } from '../../api/types'
import { catalogueReading } from '../../chrome/ambient'
import { effectivePalette, type TeamPalette } from '../../governance/effective'
import { formatObjectRef } from '../../objectref'
import { buttonClass } from '../../ui/Button'
import { Chip } from '../../ui/Chip'
import { Mark } from '../../ui/Mark'
import { count } from '../../ui/text'
import { laneOrder } from './draft'
import { allowListStanding, claimRows, grantsInForce, offerReports } from './standing'

/**
 * The Compose landing (ADR-0064): the reader's Blueprints with their
 * governance-relevant facts, before any of them is open. Discovery stays
 * in the Blueprints view (ADR-0061 §3), reached through the browse door:
 * the landing answers "my Blueprints and their standing", the browse view
 * "which Blueprint fits", and neither grows the other's controls.
 *
 * The table's one verdict-shaped column is the Allow-list standing,
 * because the Allow-list is the one save gate (ADR-0022 §3) and its
 * judgement is derivable through governance/effective.ts, the module the
 * Effective palette view already reads. Everything else a verdict would
 * need (floors, lifecycle, requirement coverage) is the engine's per open
 * draft, so the landing does not claim it.
 *
 * The rail carries the two readings checked before composing, both
 * already served: the reader's team's effective palette, and the
 * Catalogue designation with the impact report behind any version on
 * offer (ADR-0062's derive-never-judge posture). Every reading is a door;
 * a section whose data is absent is absent.
 */
export function Landing({ docs, me }: { docs: BlueprintDoc[]; me: Me | undefined }) {
  const estate = useQuery({ queryKey: ['estate'], queryFn: api.estate })
  const governance = useQuery({ queryKey: ['governance'], queryFn: api.governance })
  const versions = useQuery({ queryKey: ['catalogue-versions'], queryFn: api.catalogueVersions })
  const active = versions.data?.active
  const entries = useQuery({
    queryKey: ['catalogue-entries', active],
    queryFn: () => api.catalogueEntries(active as string),
    enabled: active !== undefined,
  })
  const shared = useQuery({ queryKey: ['catalogue'], queryFn: api.catalogue })
  const activations = useQuery({ queryKey: ['activations'], queryFn: api.activations })
  const navigate = useNavigate()

  // One palette per owning team, shared by the table rows and the rail.
  const palettes = new Map<string, TeamPalette | undefined>()
  const paletteFor = (team: string): TeamPalette | undefined => {
    if (
      estate.data === undefined ||
      entries.data === undefined ||
      governance.data === undefined
    ) {
      return undefined
    }
    if (!palettes.has(team)) {
      palettes.set(
        team,
        effectivePalette({
          tree: estate.data.teams,
          team,
          entries: entries.data,
          governance: governance.data,
        }),
      )
    }
    return palettes.get(team)
  }

  const standingCell = (doc: BlueprintDoc) => {
    const palette = paletteFor(doc.team)
    if (palette === undefined || shared.data === undefined || entries.data === undefined) {
      return null
    }
    const standing = allowListStanding({
      doc,
      palette,
      shared: shared.data,
      entries: entries.data,
    })
    if (!standing.blocked) {
      return (
        <Chip tone="ok" data-testid={`landing-standing-${doc.id}`}>
          clear
        </Chip>
      )
    }
    return (
      <>
        <Chip tone="advisory" data-testid={`landing-standing-${doc.id}`}>
          save disabled
        </Chip>
        {standing.facts.map((fact) => (
          <p key={fact} className="landing-fact">
            {fact}
          </p>
        ))}
      </>
    )
  }

  const usedByCell = (doc: BlueprintDoc) => {
    if (doc.tier === undefined) {
      return <span className="item-meta">not used by a Tier</span>
    }
    const card = estate.data?.cards.find((c) => c.tier === doc.tier)
    if (card === undefined) {
      return <span className="mono">{doc.tier}</span>
    }
    return (
      <>
        <Link
          to="/estate"
          search={(prev) => ({
            lens: prev.lens,
            tour: prev.tour,
            step: prev.step,
            view: 'shelf' as const,
            scope: 'estate' as const,
            object: formatObjectRef({ kind: 'tier', id: card.tier }),
          })}
          className="count-door"
          data-testid={`landing-used-by-${doc.id}`}
        >
          {card.name}
        </Link>
        <span className="item-meta"> · {card.environment}</span>
      </>
    )
  }

  const claims = claimRows(docs)

  return (
    <section className="compose-landing" data-tour="compose" data-testid="compose-landing">
      <header className="compose-landing-header">
        <h1>Compose</h1>
        <Link
          to="/compose"
          search={(prev) => ({ ...prev, browse: true })}
          className={buttonClass('quiet')}
          data-testid="browse-blueprints"
        >
          Browse Blueprints
        </Link>
      </header>
      <div className="compose-landing-body">
        <div className="compose-landing-main">
          <div className="landing-section-head">
            <h2>Blueprints</h2>
            <p className="section-summary">{count(docs.length, 'Blueprint')}.</p>
          </div>
          {docs.length === 0 ? (
            <p className="section-summary">No Blueprints on this estate.</p>
          ) : (
            <table className="landing-table" data-testid="landing-table">
              <thead>
                <tr>
                  <th>Blueprint</th>
                  <th>Team</th>
                  <th>Lanes</th>
                  <th>Satisfies</th>
                  <th>Used by</th>
                  <th>Allow-list</th>
                </tr>
              </thead>
              <tbody>
                {docs.map((bp) => (
                  <tr key={bp.id} data-testid={`landing-row-${bp.id}`}>
                    <td className="landing-name-cell">
                      {/* The same door the cards offered: the object param
                          opens the workspace over this Blueprint. */}
                      <button
                        type="button"
                        className="landing-name"
                        data-testid={`blueprint-${bp.id}`}
                        onClick={() =>
                          void navigate({
                            to: '/compose',
                            search: (prev) => ({
                              ...prev,
                              object: formatObjectRef({ kind: 'blueprint', id: bp.id }),
                            }),
                          })
                        }
                      >
                        {bp.name}
                      </button>{' '}
                      <span className="item-meta">v{bp.version}</span>
                    </td>
                    <td>{bp.team}</td>
                    <td className="mono landing-lanes">
                      {laneOrder(bp).map((signal, i) => (
                        <span key={signal}>
                          {i > 0 && ' · '}
                          <span className="lane-name" data-signal={signal}>
                            {signal}
                          </span>
                        </span>
                      ))}
                    </td>
                    <td>
                      {bp.satisfies.length === 0 ? (
                        <span className="item-meta">no claims</span>
                      ) : (
                        bp.satisfies.map((claim) => (
                          <Chip key={claim} mono className="landing-satisfies">
                            {claim}
                          </Chip>
                        ))
                      )}
                    </td>
                    <td data-testid={`landing-used-by-cell-${bp.id}`}>{usedByCell(bp)}</td>
                    <td data-testid={`landing-standing-cell-${bp.id}`}>{standingCell(bp)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          <div className="landing-section-head">
            <h2>Requirements</h2>
          </div>
          {claims.length === 0 ? (
            <p className="section-summary">No Blueprint claims a Requirement.</p>
          ) : (
            <ul className="landing-claims" data-testid="landing-requirements">
              {claims.map(({ claim, blueprint }) => (
                <li
                  key={`${claim} ${blueprint.id}`}
                  className="landing-claim"
                  data-testid={`landing-claim-${blueprint.id}-${claim}`}
                >
                  <span className="mono">{claim}</span>
                  <span className="item-meta">claimed by {blueprint.name}</span>
                  {/* The claim chip's door (REQ-031): the verdict is the
                      engine's, on the Requirement-first surface. */}
                  <Link
                    to="/compose"
                    search={(prev) => ({
                      ...prev,
                      object: formatObjectRef({ kind: 'blueprint', id: blueprint.id }),
                      surface: 'requirements' as const,
                    })}
                    className="count-door landing-claim-door"
                    data-testid={`landing-claim-verdict-${blueprint.id}-${claim}`}
                  >
                    → verdict
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </div>

        <aside className="compose-rail" data-testid="compose-rail">
          <PaletteReading me={me} active={active} paletteFor={paletteFor} teams={estate.data?.teams} />
          <CatalogueOffer
            activations={activations.data}
          />
        </aside>
      </div>
    </section>
  )
}

/** A team's display name from the tree; the id where the tree lacks it. */
function teamNameOf(tree: TeamNode | undefined, id: string): string {
  if (tree === undefined) return id
  if (tree.id === id) return tree.name
  for (const child of tree.teams ?? []) {
    const found = teamNameOf(child, id)
    if (found !== id) return found
  }
  return id
}

function PaletteReading({
  me,
  active,
  paletteFor,
  teams,
}: {
  me: Me | undefined
  active: string | undefined
  paletteFor: (team: string) => TeamPalette | undefined
  teams: TeamNode | undefined
}) {
  if (me === undefined || active === undefined) return null
  const palette = paletteFor(me.team)
  if (palette === undefined) return null
  const allowed = palette.rows.filter((row) => row.allowed).length
  const grants = grantsInForce(palette)
  return (
    <section className="rail-section" data-testid="landing-palette">
      <h2>Effective palette</h2>
      <p className="rail-reading" data-testid="landing-palette-summary">
        {teamNameOf(teams, me.team)}: {allowed} of {palette.rows.length} Catalogue entries
        allowed · judged against {active} (active)
      </p>
      {grants.map((grant) => (
        <p key={grant.id} className="rail-reading" data-testid={`landing-grant-${grant.id}`}>
          Grant <span className="mono">{grant.id}</span> allows{' '}
          <span className="mono">{grant.adds.join(', ')}</span>.
        </p>
      ))}
      <Link
        to="/catalogue"
        search={(prev) => ({
          lens: prev.lens,
          tour: prev.tour,
          step: prev.step,
          view: 'palette' as const,
        })}
        className="count-door"
        data-testid="landing-open-palette"
      >
        Open the Effective palette
      </Link>
    </section>
  )
}

function CatalogueOffer({ activations }: { activations: ActivationsPayload | undefined }) {
  if (activations === undefined) return null
  const reading = catalogueReading(activations)
  if (reading === undefined || reading.active === '') return null
  const reports = offerReports(activations)
  return (
    <section className="rail-section" data-testid="landing-catalogue">
      <h2>Catalogue</h2>
      <p className="rail-reading" data-testid="landing-catalogue-active">
        Active: <span className="mono">{reading.active}</span>
        {reading.onOffer.length > 0 && (
          <>
            {' · '}
            <span className="mono">{reading.onOffer.join(', ')}</span> on offer
          </>
        )}
      </p>
      {reports.flatMap((report) =>
        report.lines.map((line) => (
          <p
            key={`${report.version} ${line}`}
            className="rail-advisory"
            data-testid="landing-offer-line"
          >
            <Mark name="advisory" /> {line}
          </p>
        )),
      )}
      <Link
        to="/catalogue"
        search={(prev) => ({
          lens: prev.lens,
          tour: prev.tour,
          step: prev.step,
          view: 'activation' as const,
        })}
        className="count-door"
        data-testid="landing-open-activation"
      >
        Open Activation
      </Link>
    </section>
  )
}
