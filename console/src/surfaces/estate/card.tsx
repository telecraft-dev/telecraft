import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { api } from '../../api/client'
import type {
  BandName,
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
  formatAge,
  formatChurn,
  formatFreshness,
  formatItems,
  formatReduction,
  formatShape,
  formatVolume,
  laneReads,
  laneUnread,
  NO_LANE,
  NO_LANE_TITLE,
  NO_READINGS_YET,
  readingTitle,
} from '../../estate/readings'
import {
  defaultImage,
  foreignRun,
  identityBlock,
  kubernetesManifest,
  packagesNotes,
  supervisorYaml,
} from '../../estate/setup'
import { deepLinkFor } from '../../objectref'
import { buttonClass, Button } from '../../ui/Button'
import { Chip } from '../../ui/Chip'
import { count, formatCount } from '../../ui/text'
import { BandMark, Mark, markFor, stateLabel } from '../../ui/Mark'
import { Panel } from '../../ui/Panel'

// The universal card, face and panel (ADR-0041, ADR-0042 §3.2): one
// component wherever a Tier appears, summoned in place: inspection never
// navigates. Glyphs map from band states; hue only reinforces them
// (mono-red rule, ADR-0041 §2).

const BAND_LABEL: Record<BandName, string> = {
  delivery: 'Delivery',
  expectation: 'Expectation',
  conformance: 'Conformance',
}

/**
 * The population line for a Tier whose selector has never matched a
 * collector: a normal Tuesday, never a severity hue (ADR-0030, ADR-0060
 * §3). The age doubles as how long the setup guidance has gone
 * unfollowed.
 */
function neverSeenLine(population: CardFace['population']): string {
  const since = population.since === undefined ? Number.NaN : Date.parse(population.since)
  if (Number.isNaN(since)) return 'no collectors yet'
  return `no collectors yet, watching ${formatAge((Date.now() - since) / 1000)}`
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
  const firstLane = signals[0]
  if (
    firstLane !== undefined &&
    laneReads(firstLane) &&
    signals.every((row) => laneReads(row) && laneUnread(row))
  ) {
    // Every lane is wired and waiting for its first reading: one line,
    // not a grid of cells all carrying the same words. A card with any
    // reading anywhere, or any unwired lane, keeps the full matrix, so a
    // partly-broken pipeline and a missing lane can never hide behind the
    // quiet line. The title keeps the last-known-plus-age story the cells
    // would have carried (ADR-0041 §2); the lanes share it.
    return (
      <p
        className="matrix-quiet"
        data-testid={`matrix-${tier}-quiet`}
        // A never_seen Tier's lanes carry no readings at all, not unknown
        // ones (ADR-0060 §3), so there is no as-of to put in the title.
        title={firstLane.volume !== undefined ? readingTitle(firstLane.volume) : undefined}
      >
        {NO_READINGS_YET}
      </p>
    )
  }
  return (
    // The matrix is the one part of a card whose height the payload
    // decides: four lanes each carrying a reduction and an error reading
    // measure 410px, against roughly 250px for the common card. Equal
    // heights at card grain are not negotiable (ADR-0042 §2, P4 rule 3),
    // so the matrix scrolls inside its own bounds rather than pushing the
    // foot off the card. The foot carries the door to the flat list, and
    // a door that silently disappears is worse than one that scrolls.
    <div className="signal-matrix-scroll">
    <table className="signal-matrix" data-testid={`matrix-${tier}`}>
      <tbody>
        {signals.map((row) => {
          if (!laneReads(row)) {
            // A lane the Tier's artefact never wired (#98). It is not a
            // reading of zero and it is not an unknown: there is no
            // pipeline here, so the row says so across the cells the
            // readings would have filled, under the mark ADR-0047 §7
            // gives the state: a plain rule, nothing to judge.
            return (
              <tr
                key={row.signal}
                className="lane-absent"
                data-lane={row.lane}
                data-testid={`matrix-${tier}-${row.signal}`}
              >
                <th scope="row">{row.signal}</th>
                <td className="cell-lane" colSpan={3} title={NO_LANE_TITLE}>
                  <Mark name="not_applicable" />
                  {NO_LANE}
                </td>
              </tr>
            )
          }
          const reduction = formatReduction(row.volume)
          const errors = errorReadings(row.volume)
          return (
            <tr key={row.signal} data-lane={row.lane} data-testid={`matrix-${tier}-${row.signal}`}>
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
    </div>
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
      // A Tour Step pointing here lands on the first card in the DOM,
      // which the shelf orders worst-first: the card worth reading
      // (ADR-0051 §5).
      data-tour="card"
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
        <ul className="card-bands" data-tour="card-bands">
          {bands.map((band) => {
            const { state, worstSeverity } = card.bands[band]
            return (
              <li key={band} className={`band band-${state} severity-${worstSeverity}`}>
                <BandMark state={state} severity={worstSeverity} />
                <span className="band-name">{BAND_LABEL[band]}</span>
                <span className="band-state">{stateLabel(state)}</span>
              </li>
            )
          })}
        </ul>
        <SignalMatrix tier={card.tier} signals={card.signals ?? []} />
      </button>
      {/* The waiting room's door (ADR-0060 §3, ADR-0063): the setup
          guidance lives on the card panel, so the door summons the panel
          exactly as selecting the card does. It sits in the space the
          matrix would fill, beside the quiet line, and only a never_seen
          population carries it. */}
      {card.population.state === 'never_seen' && (
        <Button
          tone="quiet"
          className="setup-door"
          data-testid={`setup-door-${card.tier}`}
          onClick={onSelect}
        >
          Set up its first collector
        </Button>
      )}
      <footer className="card-foot">
        {/* A collector count is a door to the flat list, pre-filtered
            (ADR-0042 §3.4). A never_seen population has no count to door
            on: the line says the Tier is waiting, in the neutral voice
            ADR-0030 gives the state (ADR-0060 §3). */}
        {card.population.state === 'never_seen' ? (
          <span className="population-quiet" data-testid={`card-collectors-${card.tier}`}>
            {neverSeenLine(card.population)}
          </span>
        ) : (
          <Link
            to="/estate"
            search={(prev) => ({ ...prev, view: 'list' as const, tier: card.tier })}
            className="count-door"
            data-testid={`card-collectors-${card.tier}`}
          >
            {formatCount(card.population.matched)} matched
          </Link>
        )}
        <span>{count(totalFindings(card), 'finding')}</span>
      </footer>
    </div>
  )
}

function severityLabel(severity: Severity): string {
  return severity === 'none' ? 'neutral' : severity
}

/**
 * The "why?" affordance (ADR-0042 §5): a provenance popover (claim, the
 * implying config lines, the judged SHA) with an optional travel action
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
      <Button
        tone="quiet"
        className="why-button"
        data-testid={`why-${provenance.key}`}
        aria-expanded={open}
        onClick={onToggle}
      >
        why?
      </Button>
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
              className={buttonClass('secondary', 'who-acts')}
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
      className={buttonClass('secondary', 'who-acts')}
      data-testid={`who-acts-${finding.id}`}
    >
      {finding.whoActs.label}
    </Link>
  )
}

type DeliveryPath = 'served' | 'foreign'
type Substrate = 'kubernetes' | 'linux' | 'windows'

/** A copy control: says what it does, then that it happened, in the same words. */
function CopyButton({ text, testId }: { text: string; testId: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <Button
      tone="quiet"
      className="setup-copy"
      data-testid={testId}
      onClick={() => {
        void navigator.clipboard.writeText(text)
        setCopied(true)
        window.setTimeout(() => setCopied(false), 2000)
      }}
    >
      {copied ? 'Copied' : 'Copy'}
    </Button>
  )
}

/** One copyable snippet: a label naming what it is, the text, a copy control. */
function SetupBlock({ label, testId, text }: { label: string; testId: string; text: string }) {
  return (
    <div className="setup-block" data-testid={testId}>
      <p className="setup-block-label">
        <span>{label}</span>
        <CopyButton text={text} testId={`${testId}-copy`} />
      </p>
      <pre>{text}</pre>
    </div>
  )
}

/**
 * Setup guidance on the never_seen card (ADR-0060 §3 to §6): the waiting
 * room's content, generated on view and never committed (§4). The
 * delivery-path and substrate choices are transient presentation, so they
 * live here rather than in the URL. The snippet text itself comes from
 * `estate/setup.ts`, pure and unit-tested.
 */
function SetupSection({ tier }: { tier: string }) {
  // Hooks first, before any pending or error rendering.
  const guidance = useQuery({
    queryKey: ['setup', tier],
    queryFn: () => api.setup(tier),
  })
  const [path, setPath] = useState<DeliveryPath>('served')
  const [substrate, setSubstrate] = useState<Substrate>('kubernetes')
  // On Served for Kubernetes the image is the adopter's to supply, never
  // a default (ADR-0060 §5); on Foreign the upstream image pre-fills and
  // stays editable (§6). Two fields, so switching paths forgets nothing.
  const [servedImage, setServedImage] = useState('')
  const [foreignImage, setForeignImage] = useState<string>()

  const pathChoice = (value: DeliveryPath, label: string) => (
    <button
      type="button"
      data-testid={`setup-${value}`}
      className={buttonClass('secondary', path === value ? 'setup-choice selected' : 'setup-choice')}
      aria-pressed={path === value}
      onClick={() => setPath(value)}
    >
      {label}
    </button>
  )
  const substrateChoice = (value: Substrate, label: string) => (
    <button
      type="button"
      data-testid={`setup-${value}`}
      className={buttonClass(
        'secondary',
        substrate === value ? 'setup-choice selected' : 'setup-choice',
      )}
      aria-pressed={substrate === value}
      onClick={() => setSubstrate(value)}
    >
      {label}
    </button>
  )

  const g = guidance.data
  const foreignValue = g === undefined ? '' : (foreignImage ?? defaultImage(g.collectorRelease))

  return (
    <section className="panel-setup" data-testid="panel-setup">
      <h3>Setup</h3>
      {guidance.isPending && <p className="surface-status">Loading the setup guidance…</p>}
      {guidance.isError && <p className="surface-status">The setup guidance failed to load.</p>}
      {g && (
        <>
          <div className="setup-toggle" role="group" aria-label="Delivery path">
            {pathChoice('served', 'Served')}
            {pathChoice('foreign', 'Foreign')}
          </div>
          {path === 'served' && (
            <div className="setup-toggle" role="group" aria-label="Substrate">
              {substrateChoice('kubernetes', 'Kubernetes')}
              {substrateChoice('linux', 'Linux server')}
              {substrateChoice('windows', 'Windows server')}
            </div>
          )}
          {path === 'served' && substrate === 'kubernetes' && (
            <>
              <label className="setup-field">
                <span>Image</span>
                <input
                  data-testid="setup-image"
                  value={servedImage}
                  placeholder="YOUR_IMAGE"
                  onChange={(event) => setServedImage(event.target.value)}
                />
              </label>
              <p className="setup-note">
                The image must contain both the collector and the Supervisor. OpenTelemetry
                publishes no combined image, so supply your own.
              </p>
            </>
          )}
          {path === 'foreign' && (
            <label className="setup-field">
              <span>Image</span>
              <input
                data-testid="setup-image"
                value={foreignValue}
                onChange={(event) => setForeignImage(event.target.value)}
              />
            </label>
          )}
          <SetupBlock
            label="Identity the selector matches"
            testId="block-identity"
            text={identityBlock(g)}
          />
          {path === 'served' && (
            <SetupBlock label="supervisor.yaml" testId="block-supervisor" text={supervisorYaml(g)} />
          )}
          {path === 'served' && substrate === 'kubernetes' && (
            <SetupBlock
              label="Kubernetes manifest template"
              testId="block-manifest"
              text={kubernetesManifest(g, servedImage.trim() === '' ? undefined : servedImage.trim())}
            />
          )}
          {path === 'served' && substrate !== 'kubernetes' && (
            <SetupBlock
              label="Packages and units"
              testId="block-packages"
              text={packagesNotes(g, substrate)}
            />
          )}
          {path === 'foreign' && (
            <SetupBlock
              label="Rendered artefact"
              testId="block-artefact"
              text={foreignRun(g, foreignValue)}
            />
          )}
        </>
      )}
    </section>
  )
}

/**
 * The card panel: face facts plus the on-demand drawer (ADR-0041 §3):
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
    <Panel
      name="card"
      testId="card-panel"
      title={card.name}
      titleTestId="panel-title"
      closeTestId="panel-close"
      onClose={() =>
        void navigate({
          to: '.',
          search: (prev) => ({ ...prev, object: undefined }),
        })
      }
    >
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
          {card.population.state === 'never_seen' ? (
            // No door: the flat list has no rows for this Tier yet, and a
            // door to an empty room says less than the line does
            // (ADR-0060 §3). Neutral, never a severity hue (ADR-0030).
            <span className="population-quiet">{neverSeenLine(card.population)}</span>
          ) : (
            <>
              <Link
                to="/estate"
                search={(prev) => ({ ...prev, view: 'list' as const, tier: card.tier })}
                className="count-door"
                data-testid={`panel-collectors-${card.tier}`}
              >
                {formatCount(card.population.matched)} matched
              </Link>
              {card.population.floor !== undefined && (
                <>
                  , floor {card.population.floor} ({card.population.floorSource}) {why('floor')}
                </>
              )}
            </>
          )}
        </dd>
      </dl>
      {card.population.state === 'never_seen' && <SetupSection tier={card.tier} />}
      <ul className="panel-bands">
        {BAND_ORDER.map((band) => {
          const { state, worstSeverity, worstFinding } = card.bands[band]
          return (
            <li key={band} className={`band band-${state} severity-${worstSeverity}`}>
              <BandMark state={state} severity={worstSeverity} />
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
            {/* The restart-rate reading: presented, never judged. Scaling
                out and crash-looping both raise it (ADR-0040 §4). */}
            Restarts: {formatChurn(card.churn)}
          </p>
        )}
      </section>
      <section className="panel-findings" data-testid="panel-findings">
        <h3>Findings</h3>
        {drawer.isPending && <p className="surface-status">Loading the findings…</p>}
        {drawer.isError && <p className="surface-status">The findings failed to load.</p>}
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
                    <Mark name={markFor('finding', finding.severity)} />
                    <span className="finding-kind">{finding.kind}</span>
                    <span className="finding-severity">{severityLabel(finding.severity)}</span>
                    {/* Placement is a glossary term, so the token itself is
                        the chip's text: landed or live, absent meaning
                        landed on payloads that predate the field. */}
                    {finding.placement && (
                      <Chip
                        className="placement"
                        data-testid={`placement-${finding.id}`}
                      >
                        {finding.placement}
                      </Chip>
                    )}
                    {finding.dampening !== 'none' && (
                      <Chip
                        className={`dampening dampening-${finding.dampening}`}
                        data-testid={`dampening-${finding.id}`}
                      >
                        {finding.dampening}
                      </Chip>
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
    </Panel>
  )
}
