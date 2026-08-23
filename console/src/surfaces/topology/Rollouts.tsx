import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { useState } from 'react'
import { api } from '../../api/client'
import type {
  Provenance,
  RolloutCohortProgress,
  RolloutDecision,
  RolloutPathProgress,
  RolloutProgress,
} from '../../api/types'
import { formatObjectRef, parseObjectRef } from '../../objectref'
import { Chip, chipClass, type ChipTone } from '../../ui/Chip'
import { Mark } from '../../ui/Mark'
import { Panel } from '../../ui/Panel'
import { CardPanel, WhyButton } from '../estate/card'
import { TopologyViewSwitcher } from './switcher'

// The rollout ledger (ADR-0029): an active Rollout rendered cohort by
// cohort across both delivery paths: membership from the pure function,
// delivery status against the rollout artefacts from commit stamps.
// Foreign (GitOps) members are advisory, blocking nothing: lag, never
// failure (§7). Halt and abort states carry provenance and deep-link to
// the Rollout panel; every state here lives in the URL (ADR-0042 §3.5).

const DECISION_LABEL: Record<RolloutDecision, string> = {
  hold: 'holding',
  blocked: 'halted (advance withheld)',
  advance: 'advance proposed',
  abort: 'abort proposed',
}

/* The verdict chip's tone. The words above are what carry the decision;
   the tone only reinforces them (ADR-0041 §2), and holding is a state with
   no severity at all rather than a quiet failure. */
const DECISION_TONE: Record<RolloutDecision, ChipTone> = {
  hold: 'neutral',
  blocked: 'violation',
  advance: 'ok',
  abort: 'violation',
}

/** One path's running split, `to` leading: the advance evidence number. */
function PathSplit({ split }: { split: RolloutPathProgress }) {
  if (split.members === 0) return <span className="path-split">none</span>
  const rest = [
    ['from', split.from],
    ['other', split.other],
    ['unknown', split.unknown],
  ].filter(([, count]) => (count as number) > 0)
  return (
    <span className="path-split">
      {split.to} of {split.members} on to
      {rest.map(([label, count]) => (
        <span key={label as string} className={`path-split-part running-${label}`}>
          {' '}
          · {count} {label}
        </span>
      ))}
    </span>
  )
}

function CohortRow({
  rollout,
  cohort,
}: {
  rollout: RolloutProgress
  cohort: RolloutCohortProgress
}) {
  const members = cohort.served.members + cohort.foreign.members
  return (
    <tr
      className={`cohort-row cohort-${cohort.state}`}
      data-testid={`cohort-${rollout.id}-${cohort.index}`}
    >
      <td className="cohort-stage">
        <span className={`cohort-state cohort-state-${cohort.state}`}>
          {cohort.index + 1}
          {cohort.state === 'active' && ' · active'}
          {cohort.state === 'pending' && ' · pending'}
        </span>
      </td>
      <td className="cohort-spec">
        <code>{cohort.cohort}</code>
        {cohort.widens > 0 && cohort.index > 0 && (
          <span className="cohort-widens"> widens by {cohort.widens}</span>
        )}
      </td>
      <td>{cohort.soak}</td>
      <td>
        {/* A collector count is a door to the flat list, pre-filtered
            (ADR-0042 §3.4): per-collector detail lives there only. */}
        <Link
          to="/estate"
          search={(prev) => ({ ...prev, view: 'list' as const, tier: rollout.tier })}
          className="count-door"
          data-testid={`cohort-members-${rollout.id}-${cohort.index}`}
        >
          {members} member{members === 1 ? '' : 's'}
        </Link>
      </td>
      <td data-testid={`cohort-served-${rollout.id}-${cohort.index}`}>
        {cohort.state === 'pending' ? (
          <span className="cohort-preview">{cohort.served.members} previewed</span>
        ) : (
          <PathSplit split={cohort.served} />
        )}
      </td>
      <td data-testid={`cohort-foreign-${rollout.id}-${cohort.index}`}>
        {cohort.state === 'pending' ? (
          <span className="cohort-preview">{cohort.foreign.members} previewed</span>
        ) : (
          <PathSplit split={cohort.foreign} />
        )}
        {cohort.foreign.members > 0 && (
          <Chip
            tone="ungoverned"
            className="advisory-chip"
            data-testid={`advisory-${rollout.id}-${cohort.index}`}
          >
            advisory
          </Chip>
        )}
      </td>
    </tr>
  )
}

function RolloutSection({
  rollout,
  selected,
}: {
  rollout: RolloutProgress
  selected: boolean
}) {
  return (
    <section
      className={`rollout-section${selected ? ' selected' : ''}`}
      data-testid={`rollout-${rollout.id}`}
    >
      <header className="rollout-head">
        <h2>
          <Link
            from="/topology"
            to="/topology"
            search={(prev) => ({
              ...prev,
              object: formatObjectRef({ kind: 'rollout', id: rollout.id }),
            })}
            className="rollout-name"
            data-testid={`rollout-select-${rollout.id}`}
          >
            {rollout.name}
          </Link>
        </h2>
        <span className="rollout-bindings">
          <code>{rollout.from}</code> → <code>{rollout.to}</code>
        </span>
        {/* The halt/abort state deep-links to the Rollout panel, where its
            provenance lives: inspect stays, action travels (ADR-0042 §3). */}
        <Link
          from="/topology"
          to="/topology"
          search={(prev) => ({
            ...prev,
            object: formatObjectRef({ kind: 'rollout', id: rollout.id }),
          })}
          className={chipClass(DECISION_TONE[rollout.decision], {
            extra: `decision-chip decision-${rollout.decision}`,
          })}
          data-testid={`rollout-decision-${rollout.id}`}
        >
          {DECISION_LABEL[rollout.decision]}
        </Link>
      </header>
      <p className="rollout-target">
        Stages{' '}
        <Link
          from="/topology"
          to="/topology"
          search={(prev) => ({
            ...prev,
            object: formatObjectRef({ kind: 'tier', id: rollout.tier }),
          })}
          className="count-door"
          data-testid={`rollout-tier-${rollout.id}`}
        >
          {rollout.tier}
        </Link>{' '}
        ({rollout.environment}), stage {rollout.stage + 1} of {rollout.cohorts.length}:{' '}
        {rollout.evidence.runningTo} of {rollout.evidence.membersSeen} running the to artefact
      </p>
      <table className="catalogue-table rollout-table">
        <thead>
          <tr>
            <th>Stage</th>
            <th>Cohort</th>
            <th>Min soak</th>
            <th>Members</th>
            <th>Served</th>
            <th>Foreign (lag, never failure)</th>
          </tr>
        </thead>
        <tbody>
          {rollout.cohorts.map((cohort) => (
            <CohortRow key={cohort.index} rollout={rollout} cohort={cohort} />
          ))}
        </tbody>
      </table>
      {rollout.halts.length > 0 && (
        <ul className="rollout-halts">
          {rollout.halts.map((halt) => (
            <li key={halt.collector}>
              <Link
                from="/topology"
                to="/topology"
                search={(prev) => ({
                  ...prev,
                  object: formatObjectRef({ kind: 'rollout', id: rollout.id }),
                })}
                className={chipClass('violation', { extra: 'halt-chip' })}
                data-testid={`rollout-halt-${rollout.id}-${halt.collector}`}
              >
                <Mark name="violation" /> {halt.collector} {halt.condition}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

/**
 * The Rollout panel: the authored facts, the evaluation's verdict with its
 * evidence, and every halted member with provenance, summoned in place
 * beside the ledger, like the Tier card (ADR-0042 §3.2).
 */
function RolloutPanel({ rollout }: { rollout: RolloutProgress }) {
  const navigate = useNavigate()
  const [openWhy, setOpenWhy] = useState<string | undefined>()

  const why = (key: string) => {
    const entry: Provenance | undefined = rollout.provenance.find((p) => p.key === key)
    if (!entry) return null
    return (
      <WhyButton
        provenance={entry}
        open={openWhy === key}
        onToggle={() => setOpenWhy(openWhy === key ? undefined : key)}
      />
    )
  }

  return (
    <Panel
      name="rollout"
      testId="rollout-panel"
      className="rollout-panel"
      title={rollout.name}
      titleTestId="rollout-panel-title"
      closeTestId="rollout-panel-close"
      onClose={() =>
        void navigate({
          to: '.',
          search: (prev) => ({ ...prev, object: undefined }),
        })
      }
    >
      <dl className="panel-facts">
        <dt>Rollout</dt>
        <dd>{rollout.id}</dd>
        <dt>Owner</dt>
        <dd>{rollout.owner}</dd>
        <dt>Tier</dt>
        <dd>
          <Link
            from="/topology"
            to="/topology"
            search={(prev) => ({
              ...prev,
              object: formatObjectRef({ kind: 'tier', id: rollout.tier }),
            })}
            className="count-door"
            data-testid="rollout-panel-tier"
          >
            {rollout.tier}
          </Link>{' '}
          ({rollout.environment})
        </dd>
        <dt>From</dt>
        <dd>
          <code>{rollout.from}</code> {why('bindings')}
        </dd>
        <dt>To</dt>
        <dd>
          <code>{rollout.to}</code>
        </dd>
        <dt>Stage</dt>
        <dd>
          {rollout.stage + 1} of {rollout.cohorts.length}, soaked {rollout.evidence.soaked} of{' '}
          {rollout.evidence.minSoak} minimum {why('stage')}
        </dd>
      </dl>
      <section className="rollout-verdict" data-testid="rollout-verdict">
        <span
          className={chipClass(DECISION_TONE[rollout.decision], {
            extra: `decision-chip decision-${rollout.decision}`,
          })}
          data-testid="rollout-panel-decision"
        >
          {DECISION_LABEL[rollout.decision]}
        </span>
        <p className="rollout-reason">{rollout.reason}</p>
        <p className="rollout-evidence">
          {rollout.evidence.membersSeen} cohort members seen: {rollout.evidence.runningTo} on to,{' '}
          {rollout.evidence.runningFrom} on from, {rollout.evidence.runningOther} on another
          config, {rollout.evidence.unknown} unknown.
        </p>
      </section>
      <section className="panel-findings" data-testid="rollout-panel-halts">
        <h3>Halted members</h3>
        {rollout.halts.length === 0 ? (
          <p className="section-summary">Nothing halted.</p>
        ) : (
          <ul className="findings-list">
            {rollout.halts.map((halt) => (
              <li
                key={halt.collector}
                className="finding severity-violation"
                data-testid={`halt-${rollout.id}-${halt.collector}`}
              >
                <p className="finding-head">
                  <Mark name="violation" />
                  <span className="finding-kind">{halt.condition}</span>
                  {halt.path === 'foreign' && (
                    <Chip tone="ungoverned" className="advisory-chip">
                      foreign
                    </Chip>
                  )}
                </p>
                <p className="finding-summary">
                  {halt.collector}: {halt.reason}
                </p>
              </li>
            ))}
          </ul>
        )}
      </section>
    </Panel>
  )
}

export function Rollouts() {
  const rollouts = useQuery({ queryKey: ['rollouts'], queryFn: api.rollouts })
  const estate = useQuery({ queryKey: ['estate'], queryFn: api.estate })
  const search = useSearch({ strict: false })

  if (rollouts.isPending) return <p className="surface-status">Loading the rollouts…</p>
  if (rollouts.isError)
    return <p className="surface-status">The rollout payload failed to load.</p>

  const selected = parseObjectRef(search.object)
  const selectedRollout =
    selected?.kind === 'rollout'
      ? rollouts.data.find((rollout) => rollout.id === selected.id)
      : undefined
  // The universal Tier card is summoned in place here too: a Tier appears
  // on the ledger, so its selection keeps its panel (ADR-0042 §3.2).
  const selectedCard =
    selected?.kind === 'tier'
      ? estate.data?.cards.find((card) => card.tier === selected.id)
      : undefined

  return (
    <div className="topology-layout">
      <div className="topology-main">
        <header className="topology-header">
          <h1>Topology</h1>
          <TopologyViewSwitcher active="rollout" />
        </header>
        <div className="rollout-ledger" data-testid="rollout-ledger">
          {rollouts.data.length === 0 ? (
            <p className="section-summary">
              No Rollout is active: every Tier is single-bound, the flat rebind.
            </p>
          ) : (
            rollouts.data.map((rollout) => (
              <RolloutSection
                key={rollout.id}
                rollout={rollout}
                selected={rollout.id === selectedRollout?.id}
              />
            ))
          )}
        </div>
      </div>
      {selectedRollout ? (
        <RolloutPanel rollout={selectedRollout} />
      ) : (
        selectedCard && <CardPanel card={selectedCard} />
      )}
    </div>
  )
}
