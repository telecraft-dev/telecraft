import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Fragment } from 'react'
import { api } from '../../api/client'
import { BAND_ORDER, type BandName, type CardFace, type RolloutDecision } from '../../api/types'
import { useLens } from '../../chrome/LensControl'
import { cardStanding, totalFindings } from '../../estate/order'
import type { KindRollup, TeamRollup } from '../../estate/rollup'
import {
  KIND_LABEL,
  rolloutPosition,
  summarise,
  teamStanding,
  tierDetail,
  ungovernedTotal,
  type HomeSummary,
  type NamedTiers,
} from '../../home/summary'
import { formatObjectRef } from '../../objectref'
import { Chip } from '../../ui/Chip'
import { Mark } from '../../ui/Mark'
import { count } from '../../ui/text'

// The Home Workspace (ADR-0056): the console's landing, and the one entry
// named for a place, because the activity it serves is choosing which
// activity. Its question is "where do I look first?", and every answer it
// gives is a door out of it (§4): a Tier opens Estate with its card
// selected, a Team opens the shelf scoped to it, the ungoverned count opens
// the flat list already filtered, a Rollout opens the ledger.
//
// The copy carries none of that reasoning. This surface is an instrument
// rather than a dashboard (docs/branding/identity.md): it labels its
// readings and stops. Why a ratio is not blended belongs in ADR-0017 and in
// this comment, never on screen, where it reads as a page defending itself.
//
// Nothing here is judged here (§2). The numbers come from `home/summary.ts`,
// which reads `estate/rollup.ts`, `estate/order.ts` and
// `estate/readings.ts`: the same modules the tree-table, the shelf and the
// card matrix read, so Home cannot disagree with the surface it points at.
// No number on this page is blended (§3), and every bounded list says what
// it left out (§5).

/** The tone a Rollout's verdict earns; the word beside it carries the meaning. */
const DECISION_TONE: Record<RolloutDecision, 'violation' | 'advisory' | 'neutral'> = {
  abort: 'violation',
  blocked: 'violation',
  advance: 'advisory',
  hold: 'neutral',
}

/** A Tier name as a door: Estate's shelf with that card selected (§4). */
function TierDoor({ card }: { card: CardFace }) {
  return (
    <Link
      to="/estate"
      search={(prev) => ({
        ...prev,
        view: 'shelf' as const,
        scope: 'estate' as const,
        object: formatObjectRef({ kind: 'tier', id: card.tier }),
      })}
      className="count-door"
    >
      {card.name}
    </Link>
  )
}

/**
 * The Tier names behind one tile's number, each a door, the overflow
 * counted rather than dropped. The trailing words are the tile's own
 * (`no result yet`, the exempt count), passed by the tile that has them.
 */
function TileTiers({
  tiers,
  testId,
  trailing,
}: {
  tiers: NamedTiers
  testId: string
  trailing?: string
}) {
  if (tiers.shown.length === 0 && trailing === undefined) return null
  return (
    <span className="standing-kind-tiers" data-testid={testId}>
      {tiers.shown.map((card, index) => (
        <Fragment key={card.tier}>
          {index > 0 && ' · '}
          <TierDoor card={card} />
        </Fragment>
      ))}
      {tiers.more > 0 && (
        <>
          {' · '}
          <Link
            to="/estate"
            search={(prev) => ({ ...prev, view: 'shelf' as const, scope: 'estate' as const })}
            className="count-door"
          >
            and {tiers.more} more
          </Link>
        </>
      )}
      {trailing !== undefined && (
        <>
          {tiers.shown.length > 0 && ' · '}
          {trailing}
        </>
      )}
    </span>
  )
}

/**
 * One finding kind at estate grain: ratio, worst, and the waived count
 * alongside (ADR-0017). This is the tree-table's own cell, at the root row,
 * and it stays ratio-plus-worst here for the reason it is there: an
 * exemption-heavy 100% must not be able to hide. Beneath the number, the
 * Tiers it counts, each a door.
 */
function StandingKind({
  kind,
  rollup,
  tiers,
}: {
  kind: BandName
  rollup: KindRollup
  tiers: NamedTiers
}) {
  const badge =
    rollup.worst === 'violation' ? 'violation' : rollup.worst === 'advisory' ? 'advisory' : null
  return (
    <div className={`standing-kind severity-${rollup.worst}`} data-testid={`standing-${kind}`}>
      <span className="standing-kind-label">{KIND_LABEL[kind]}</span>
      {rollup.counted === 0 ? (
        <span className="rollup-empty">no verdicts</span>
      ) : (
        <span className="standing-kind-ratio">
          <span className="rollup-ratio">
            {rollup.passing}/{rollup.counted}
          </span>
          {badge && <Mark name={badge} />}
        </span>
      )}
      <TileTiers
        tiers={tiers}
        testId={`standing-${kind}-tiers`}
        trailing={rollup.waived > 0 ? `${rollup.waived} exempt` : undefined}
      />
    </div>
  )
}

/** A Team subtree row: worst-first, and a door to its shelf section. */
function TeamRow({ row }: { row: TeamRollup }) {
  const worst = teamStanding(row)
  const badge = worst === 'violation' ? 'violation' : worst === 'advisory' ? 'advisory' : null
  return (
    <li className="home-team" data-testid={`home-team-${row.team.id}`}>
      <Link
        to="/estate"
        search={(prev) => ({
          ...prev,
          view: 'shelf' as const,
          scope: 'estate' as const,
          object: formatObjectRef({ kind: 'team', id: row.team.id }),
        })}
        className="count-door"
      >
        {row.team.name}
      </Link>
      <span className={`home-team-standing severity-${worst}`}>
        {badge ? <Mark name={badge} /> : <span className="rollup-empty">clear</span>}
      </span>
      <span className="home-team-counts">
        {row.tiersInEnvironment} of {count(row.tiersTotal, 'Tier')} here ·{' '}
        <span data-testid={`home-team-all-${row.team.id}`}>
          {count(row.findingsAllEnvironments, 'finding')} in all Environments
        </span>
        {row.waivedAllEnvironments > 0 && ` · ${row.waivedAllEnvironments} exempt`}
      </span>
    </li>
  )
}

function Summary({ summary }: { summary: HomeSummary }) {
  const { standing } = summary
  const ungoverned = ungovernedTotal(summary.ungoverned)
  const undrawn = summary.attentionInLens - summary.worstTiers.length
  const lens = summary.lens

  return (
    <div className="home">
      {/* Estate standing: the root row of the tree-table's own roll-up,
          judged under the lens, one tile per finding kind, with the Tiers
          behind each number named beneath it (ADR-0056 §3, §4). */}
      <section className="home-block" data-testid="home-standing" data-tour="home-standing">
        <div className="standing-kinds">
          {BAND_ORDER.map((kind) => (
            <StandingKind
              key={kind}
              kind={kind}
              rollup={standing.kinds[kind]}
              tiers={summary.standingTiers[kind]}
            />
          ))}
          <div className="standing-kind" data-testid="standing-neutral">
            <span className="standing-kind-label">Neutral</span>
            <span className="standing-kind-ratio">{standing.neutral}</span>
            <TileTiers
              tiers={summary.neutralTiers}
              testId="standing-neutral-tiers"
              trailing="no result yet"
            />
          </div>
        </div>
      </section>

      {/* The two bounded lists share a row where the window affords it,
          in the same reading order they stack in. */}
      <div className="home-columns">
        {/* Where to look first: the shelf's own worst-first order, bounded,
            and saying what it did not draw (ADR-0056 §5). Each row's second
            line is the card face's own facts (§2): never the drawer's. */}
        <section className="home-block" data-testid="home-worst">
          <div className="section-header">
            <h2>Tiers with findings</h2>
            <p className="section-summary">
              {summary.attentionInLens === 0
                ? `Nothing in ${lens} has a finding.`
                : `${count(summary.attentionInLens, 'Tier')} in ${lens}, worst first.`}
            </p>
          </div>
          {summary.worstTiers.length > 0 && (
            <ul className="home-tiers">
              {summary.worstTiers.map((card) => {
                const standingName = cardStanding(card)
                const detail = tierDetail(card)
                return (
                  <li className="home-tier" data-testid={`home-tier-${card.tier}`} key={card.tier}>
                    <TierDoor card={card} />
                    <Chip tone={standingName === 'violation' ? 'violation' : 'advisory'}>
                      {standingName}
                    </Chip>
                    {card.serviceClass && <Chip mono>{card.serviceClass}</Chip>}
                    <span className="home-tier-counts">
                      {count(totalFindings(card), 'finding')}, {card.team}
                    </span>
                    {detail.length > 0 && (
                      <span
                        className="home-tier-detail"
                        data-testid={`home-tier-detail-${card.tier}`}
                      >
                        {detail.join(' · ')}
                      </span>
                    )}
                  </li>
                )
              })}
            </ul>
          )}
          <p className="section-summary">
            {undrawn > 0 && (
              <span data-testid="home-worst-undrawn">
                Showing {summary.worstTiers.length} of {summary.attentionInLens}.{' '}
              </span>
            )}
            {summary.attentionElsewhere > 0 && (
              <span data-testid="home-worst-elsewhere">
                {summary.attentionElsewhere} more in other Environments.{' '}
              </span>
            )}
            <Link
              to="/estate"
              search={(prev) => ({ ...prev, view: 'shelf' as const, scope: 'estate' as const })}
              className="count-door"
              data-testid="home-to-shelf"
            >
              See all Tiers
            </Link>
          </p>
        </section>

        {/* Teams: the reason this surface exists, per ADR-0056's context. */}
        <section className="home-block" data-testid="home-teams">
          <div className="section-header">
            <h2>Teams</h2>
            <p className="section-summary">
              Totals include every team below. Worst first.
              {summary.teamsTotal > summary.teams.length &&
                ` Showing ${summary.teams.length} of ${summary.teamsTotal}.`}
            </p>
          </div>
          <ul className="home-teams">
            {summary.teams.map((row) => (
              <TeamRow key={row.team.id} row={row} />
            ))}
          </ul>
          <p className="section-summary">
            <Link
              to="/estate"
              search={(prev) => ({ ...prev, view: 'rollup' as const })}
              className="count-door"
              data-testid="home-to-rollup"
            >
              See every team
            </Link>
          </p>
        </section>
      </div>

      <div className="home-columns">
        {/* Ungoverned: a concern carrying the onboard CTA, never a failure,
            and in no compliance denominator (ADR-0031). One row: the chip,
            the reading, and the door. */}
        <section className="home-block" data-testid="home-ungoverned">
          <div className="section-header">
            <h2>Ungoverned collectors</h2>
          </div>
          {ungoverned === 0 ? (
            <p className="section-summary">Every collector matches a Tier.</p>
          ) : (
            <div className="home-ungoverned-row">
              <Chip tone="ungoverned" data-testid="home-ungoverned-count">
                {count(ungoverned, 'collector')}
              </Chip>
              <span className="home-ungoverned-note">
                don't match any Tier. {summary.ungoverned.served} served,{' '}
                {summary.ungoverned.foreign} foreign.
              </span>
              <Link
                to="/estate"
                search={(prev) => ({
                  ...prev,
                  view: 'list' as const,
                  ungoverned: true,
                })}
                className="count-door"
                data-testid="home-to-ungoverned"
              >
                Claim them
              </Link>
            </div>
          )}
        </section>

        {/* Rollouts whose verdict wants a person; `hold` is counted, not
            drawn, because nothing there needs doing (ADR-0029 §5). The
            position line is the ledger's own numbers, in its words. */}
        <section className="home-block" data-testid="home-rollouts">
          <div className="section-header">
            <h2>Rollouts</h2>
          </div>
          {summary.rolloutsWaiting === 0 ? (
            <p className="section-summary" data-testid="home-rollouts-quiet">
              {summary.rolloutsSteady === 0
                ? 'No Rollouts are running.'
                : `${summary.rolloutsSteady} running, nothing to decide.`}
            </p>
          ) : (
            <>
              <ul className="home-rollouts">
                {summary.rollouts.map((rollout) => {
                  const position = rolloutPosition(rollout)
                  return (
                    <li
                      className="home-rollout"
                      data-testid={`home-rollout-${rollout.id}`}
                      key={rollout.id}
                    >
                      <Link
                        to="/topology"
                        search={(prev) => ({
                          ...prev,
                          view: 'rollout' as const,
                          object: formatObjectRef({ kind: 'rollout', id: rollout.id }),
                        })}
                        className="count-door"
                      >
                        {rollout.name}
                      </Link>
                      <Chip tone={DECISION_TONE[rollout.decision]}>{rollout.decision}</Chip>
                      {position.length > 0 && (
                        <span
                          className="home-rollout-position"
                          data-testid={`home-rollout-position-${rollout.id}`}
                        >
                          {position.join(' · ')}
                        </span>
                      )}
                      <span className="home-rollout-reason">{rollout.reason}</span>
                    </li>
                  )
                })}
              </ul>
              <p className="section-summary">
                {summary.rolloutsWaiting > summary.rollouts.length &&
                  `${summary.rolloutsWaiting - summary.rollouts.length} more waiting. `}
                {summary.rolloutsSteady > 0 && `${summary.rolloutsSteady} running. `}
                <Link
                  to="/topology"
                  search={(prev) => ({ ...prev, view: 'rollout' as const })}
                  className="count-door"
                  data-testid="home-to-rollouts"
                >
                  See all Rollouts
                </Link>
              </p>
            </>
          )}
        </section>
      </div>
    </div>
  )
}

export function Home() {
  const estate = useQuery({ queryKey: ['estate'], queryFn: api.estate })
  // Home is the first surface reading two endpoints (ADR-0056). Both are
  // cached for the Workspaces it is a door into, so the departure is warm.
  const rollouts = useQuery({ queryKey: ['rollouts'], queryFn: api.rollouts })
  const lens = useLens()

  if (estate.isPending) return <p className="surface-status">Loading the estate…</p>
  if (estate.isError) return <p className="surface-status">The estate payload failed to load.</p>

  // A Rollouts payload that has not arrived is not a reason to withhold the
  // rest: the section says so, and everything else on the page still reads.
  const summary = summarise(estate.data, rollouts.data ?? [], lens)

  return (
    <div className="estate-main" data-testid="home">
      <div className="home-page">
        {/* The title row carries the estate sentence: the lens-judged count
            with its all-Environments companion beside it, so the lens hides
            nothing (ADR-0056 §6). */}
        <header className="estate-header">
          <h1>Home</h1>
          <p className="section-summary home-lede">
            In <strong data-testid="home-lens">{lens}</strong>, across{' '}
            {summary.standing.tiersInEnvironment} of{' '}
            {count(summary.standing.tiersTotal, 'Tier')} ·{' '}
            <span data-testid="home-all-environments">
              {count(summary.standing.findingsAllEnvironments, 'finding')},{' '}
              {summary.standing.waivedAllEnvironments} exempt in all Environments.
            </span>
          </p>
        </header>
        <Summary summary={summary} />
      </div>
    </div>
  )
}
