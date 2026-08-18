import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { api } from '../../api/client'
import type {
  BandName,
  BandState,
  CardFace,
  Finding,
  Provenance,
  Severity,
  SignalRow,
} from '../../api/types'
import { BAND_ORDER } from '../../api/types'
import type { CardStanding } from '../../estate/order'
import { totalFindings } from '../../estate/order'
import {
  errorReadings,
  formatChurn,
  formatFreshness,
  formatItems,
  formatReduction,
  formatShape,
  formatVolume,
  readingTitle,
} from '../../estate/readings'
import { deepLinkFor } from '../../objectref'

// The universal card, face and panel (ADR-0041, ADR-0042 §3.2): one
// component wherever a Tier appears, summoned in place — inspection never
// navigates. Glyphs map from band states; hue only reinforces them
// (mono-red rule, ADR-0041 §2).

const BAND_LABEL: Record<BandName, string> = {
  delivery: 'Delivery',
  expectation: 'Expectation',
  conformance: 'Conformance',
}

function glyph(state: BandState, severity: Severity): string {
  if (state === 'finding') return severity === 'violation' ? '✗' : '▲'
  if (state === 'ok') return '✓'
  return '◌'
}

function stateLabel(state: BandState): string {
  switch (state) {
    case 'ok':
      return 'ok'
    case 'finding':
      return 'finding'
    case 'not_applicable':
      return 'not applicable'
    case 'unknown':
      return 'unknown'
    case 'pending_settle':
      return 'pending settle'
    case 'stale_demoted':
      return 'stale, demoted'
  }
}

/**
 * One lane of the per-signal matrix (P4 variant D): volume with its
 * reduction, freshness, shape. Every cell carries the reading's own
 * as-of in its title, so an unknown reading reads as last-known-plus-age
 * rather than as a confident zero (ADR-0041 §2).
 */
function SignalMatrix({ tier, signals }: { tier: string; signals: SignalRow[] }) {
  if (signals.length === 0) {
    // A conforming payload with no lanes is a card with no matrix, not a
    // broken card: the console renders what the contract carries.
    return null
  }
  return (
    <table className="signal-matrix" data-testid={`matrix-${tier}`}>
      <tbody>
        {signals.map((row) => {
          const reduction = formatReduction(row.volume)
          const errors = errorReadings(row.volume)
          return (
            <tr key={row.signal} data-testid={`matrix-${tier}-${row.signal}`}>
              <th scope="row">{row.signal}</th>
              <td className="cell-volume" title={readingTitle(row.volume)}>
                {formatVolume(row.volume)}
                {reduction && <span className="cell-note">{reduction}</span>}
                {errors.length > 0 && (
                  <span className="cell-errors" data-testid={`errors-${tier}-${row.signal}`}>
                    {errors.map((error) => `${formatItems(error.items)} ${error.label}`).join(', ')}
                  </span>
                )}
              </td>
              <td className="cell-freshness" title={readingTitle(row.freshness)}>
                {formatFreshness(row.freshness)}
              </td>
              <td className="cell-shape" title={readingTitle(row.shape)}>
                {formatShape(row.shape)}
              </td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}

export function CardFaceView({
  card,
  standing,
  bands,
  selected,
  onSelect,
}: {
  card: CardFace
  standing: CardStanding
  bands: readonly BandName[]
  selected: boolean
  onSelect: () => void
}) {
  return (
    <div
      className={`card-face standing-${standing}${selected ? ' selected' : ''}`}
      data-testid={`card-${card.tier}`}
    >
      <button
        type="button"
        className="card-select"
        data-testid={`card-select-${card.tier}`}
        onClick={onSelect}
      >
        <header className="card-head">
          <span className="card-name">{card.name}</span>
          {card.serviceClass && <span className="card-class">{card.serviceClass}</span>}
        </header>
        <ul className="card-bands">
          {bands.map((band) => {
            const { state, worstSeverity } = card.bands[band]
            return (
              <li key={band} className={`band band-${state} severity-${worstSeverity}`}>
                <span className="band-glyph" aria-hidden="true">
                  {glyph(state, worstSeverity)}
                </span>
                <span className="band-name">{BAND_LABEL[band]}</span>
                <span className="band-state">{stateLabel(state)}</span>
              </li>
            )
          })}
        </ul>
        <SignalMatrix tier={card.tier} signals={card.signals ?? []} />
      </button>
      <footer className="card-foot">
        {/* A collector count is a door to the flat list, pre-filtered (ADR-0042 §3.4). */}
        <Link
          to="/estate"
          search={(prev) => ({ ...prev, view: 'list' as const, tier: card.tier })}
          className="count-door"
          data-testid={`card-collectors-${card.tier}`}
        >
          {card.population.matched} matched
        </Link>
        <span>{totalFindings(card)} findings</span>
      </footer>
    </div>
  )
}

function severityLabel(severity: Severity): string {
  return severity === 'none' ? 'neutral' : severity
}

/**
 * The "why?" affordance (ADR-0042 §5): a provenance popover — claim, the
 * implying config lines, the judged SHA — with an optional travel action
 * that traces the Service's Paths on the canvas via rule 3.3. Shared with
 * every panel that carries provenance (the Rollout panel among them).
 */
export function WhyButton({
  provenance,
  open,
  onToggle,
}: {
  provenance: Provenance
  open: boolean
  onToggle: () => void
}) {
  return (
    <span className="why">
      <button
        type="button"
        className="why-button"
        data-testid={`why-${provenance.key}`}
        aria-expanded={open}
        onClick={onToggle}
      >
        why?
      </button>
      {open && (
        <div className="why-popover" data-testid="why-popover">
          <p className="why-claim">{provenance.claim}</p>
          <ul className="why-lines">
            {provenance.lines.map((line) => (
              <li key={`${line.file}:${line.line}`}>
                <span className="why-file">
                  {line.file}:{line.line}
                </span>
                <code>{line.text}</code>
              </li>
            ))}
          </ul>
          <p className="why-sha">
            judged at <code>{provenance.sha}</code>
          </p>
          {provenance.trace && (
            <Link
              to="/topology"
              search={(prev) => ({
                lens: prev.lens,
                object: `service:${provenance.trace?.service}`,
              })}
              className="who-acts"
              data-testid={`why-trace-${provenance.key}`}
            >
              Trace {provenance.trace.service} on the canvas
            </Link>
          )}
        </div>
      )}
    </span>
  )
}

/** The who-acts chip: the routing target deep-links to the surface that can act (ADR-0042 §3.3). */
function WhoActsChip({ finding }: { finding: Finding }) {
  const link = deepLinkFor(finding.whoActs.target)
  return (
    <Link
      to={link.to}
      search={(prev) => ({
        lens: prev.lens,
        object: link.object,
        ...(finding.whoActs.lane ? { lane: finding.whoActs.lane } : {}),
      })}
      className="who-acts"
      data-testid={`who-acts-${finding.id}`}
    >
      {finding.whoActs.label}
    </Link>
  )
}

/**
 * The card panel: face facts plus the on-demand drawer (ADR-0041 §3) —
 * findings with who-acts chips and mandatory remediation, dampening state
 * visible (a waiver waives the count, never the diagnosis), and "why?"
 * provenance for every derived value.
 */
export function CardPanel({ card }: { card: CardFace }) {
  const navigate = useNavigate()
  const drawer = useQuery({
    queryKey: ['drawer', card.tier],
    queryFn: () => api.drawer(card.tier),
  })
  const [openWhy, setOpenWhy] = useState<string | undefined>()

  const provenanceFor = (key: string): Provenance | undefined =>
    drawer.data?.provenance.find((entry) => entry.key === key)

  const why = (key: string) => {
    const entry = provenanceFor(key)
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
    <aside className="card-panel" data-testid="card-panel">
      <header className="panel-head">
        <h2 data-testid="panel-title">{card.name}</h2>
        <button
          type="button"
          data-testid="panel-close"
          onClick={() =>
            void navigate({
              to: '.',
              search: (prev) => ({ ...prev, object: undefined }),
            })
          }
        >
          Close
        </button>
      </header>
      <dl className="panel-facts">
        <dt>Tier</dt>
        <dd>{card.tier}</dd>
        <dt>Owning team</dt>
        <dd>{card.team}</dd>
        <dt>Environment</dt>
        <dd>{card.environment}</dd>
        {card.serviceClass && (
          <>
            <dt>Service Class</dt>
            <dd>
              {card.serviceClass} {why('service-class')}
            </dd>
          </>
        )}
        <dt>Population</dt>
        <dd>
          <Link
            to="/estate"
            search={(prev) => ({ ...prev, view: 'list' as const, tier: card.tier })}
            className="count-door"
            data-testid={`panel-collectors-${card.tier}`}
          >
            {card.population.matched} matched
          </Link>
          {card.population.floor !== undefined && (
            <>
              , floor {card.population.floor} ({card.population.floorSource}) {why('floor')}
            </>
          )}
        </dd>
      </dl>
      <ul className="panel-bands">
        {BAND_ORDER.map((band) => {
          const { state, worstSeverity, worstFinding } = card.bands[band]
          return (
            <li key={band} className={`band band-${state} severity-${worstSeverity}`}>
              <span className="band-glyph" aria-hidden="true">
                {glyph(state, worstSeverity)}
              </span>
              <span className="band-name">{BAND_LABEL[band]}</span>
              <span className="band-state">{stateLabel(state)}</span>
              {why(`band:${band}`)}
              {worstFinding && <p className="band-finding">{worstFinding}</p>}
            </li>
          )
        })}
      </ul>
      <section className="panel-flow" data-testid="panel-flow">
        <h3>Flow</h3>
        <SignalMatrix tier={card.tier} signals={card.signals ?? []} />
        {card.churn && (
          <p className="section-summary" title={readingTitle(card.churn)}>
            {/* The restart-rate reading: presented, never judged — scaling
                out and crash-looping both raise it (ADR-0040 §4). */}
            Restarts: {formatChurn(card.churn)}
          </p>
        )}
      </section>
      <section className="panel-findings" data-testid="panel-findings">
        <h3>Findings</h3>
        {drawer.isPending && <p className="surface-status">Loading the drawer…</p>}
        {drawer.isError && <p className="surface-status">The drawer failed to load.</p>}
        {drawer.data &&
          (drawer.data.findings.length === 0 ? (
            <p className="section-summary">No findings.</p>
          ) : (
            <ul className="findings-list">
              {drawer.data.findings.map((finding) => (
                <li
                  key={finding.id}
                  className={`finding severity-${finding.severity}`}
                  data-testid={`finding-${finding.id}`}
                >
                  <p className="finding-head">
                    <span className="band-glyph" aria-hidden="true">
                      {glyph('finding', finding.severity)}
                    </span>
                    <span className="finding-kind">{finding.kind}</span>
                    <span className="finding-severity">{severityLabel(finding.severity)}</span>
                    {finding.dampening !== 'none' && (
                      <span
                        className={`dampening dampening-${finding.dampening}`}
                        data-testid={`dampening-${finding.id}`}
                      >
                        {finding.dampening}
                      </span>
                    )}
                  </p>
                  <p className="finding-summary">{finding.summary}</p>
                  <p className="finding-remediation">{finding.remediation}</p>
                  <WhoActsChip finding={finding} />
                </li>
              ))}
            </ul>
          ))}
      </section>
    </aside>
  )
}
