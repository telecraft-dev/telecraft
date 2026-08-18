import type { DragEvent } from 'react'
import { Link } from '@tanstack/react-router'
import type {
  BlueprintDoc,
  ComponentClass,
  ComposeFinding,
  ComposeVerdict,
  PaletteEntry,
} from '../../api/types'
import { laneOrder } from './draft'

// A · Composer, the primary editing surface (ADR-0043 §1): palette left —
// Catalogue ∩ effective Allow-list, judged live (ADR-0022 §5) — per-signal
// lanes right carrying floor chips and per-(component, signal) stability,
// findings as a full-width strip below. Three add gestures, one semantics
// (§4): click-add to every supported signal, per-lane targeted add, and
// drag-authoring where the drop target names the signal.

const DRAG_TYPE = 'application/x-telecraft-palette'

export function startPaletteDrag(event: DragEvent, entry: PaletteEntry) {
  event.dataTransfer.setData(DRAG_TYPE, JSON.stringify(entry))
  event.dataTransfer.effectAllowed = 'copy'
}

export function droppedEntry(event: DragEvent): PaletteEntry | undefined {
  const raw = event.dataTransfer.getData(DRAG_TYPE)
  if (raw === '') return undefined
  return JSON.parse(raw) as PaletteEntry
}

const CLASS_ORDER: ComponentClass[] = ['receiver', 'processor', 'exporter', 'connector', 'extension']

export function Palette({
  verdict,
  onAdd,
}: {
  verdict: ComposeVerdict | undefined
  onAdd: (entry: PaletteEntry, signals: string[]) => void
}) {
  if (verdict === undefined) return <p className="surface-status">Judging the palette…</p>
  const { entries, hidden } = verdict.palette
  return (
    <aside className="palette" data-testid="palette">
      <h3>Palette</h3>
      {CLASS_ORDER.filter((cls) => entries.some((e) => e.class === cls)).map((cls) => (
        <div key={cls} className="palette-group">
          <h4>{cls}s</h4>
          <ul>
            {entries
              .filter((e) => e.class === cls)
              .map((entry) => (
                <li key={entry.key}>
                  {/* Greyed is a verdict shown with its reason, never a trap:
                      the palette enforces nothing (ADR-0022 §5), and a
                      floor-breaching add produces a finding, not a wall. */}
                  <button
                    type="button"
                    className={entry.state === 'greyed' ? 'palette-entry greyed' : 'palette-entry'}
                    data-testid={`palette-${entry.key}`}
                    draggable
                    onDragStart={(event) => startPaletteDrag(event, entry)}
                    onClick={() => onAdd(entry, entry.add.signals)}
                  >
                    <span className="palette-label">{entry.label}</span>
                    <span className="palette-meta">
                      {entry.class}/{entry.type} · {entry.signals.join(' ')}
                    </span>
                    {entry.origin === 'grant' && entry.grant && (
                      <span className="palette-grant" data-testid={`palette-grant-${entry.key}`}>
                        via Grant {entry.grant.id} ({entry.grant.grantedBy} → {entry.grant.grantedTo})
                      </span>
                    )}
                    {entry.deprecated && (
                      <span className="palette-deprecated" title={entry.deprecated.migration}>
                        deprecated
                      </span>
                    )}
                    {entry.reason && <span className="palette-reason">{entry.reason}</span>}
                  </button>
                </li>
              ))}
          </ul>
        </div>
      ))}
      {/* The admission line: hiding without counting would read as healthy (P1 verdict). */}
      <p className="palette-hidden" data-testid="palette-hidden">
        {hidden} component{hidden === 1 ? '' : 's'} hidden by your allow-list
      </p>
    </aside>
  )
}

function FindingsStrip({ findings }: { findings: ComposeFinding[] }) {
  return (
    <section className="findings-strip" data-testid="findings-strip">
      <h3>Findings</h3>
      {findings.length === 0 ? (
        <p className="section-summary">No findings.</p>
      ) : (
        <ul>
          {findings.map((finding) => (
            <li
              key={finding.id}
              className={`compose-finding severity-${finding.severity}${
                finding.kind === 'allow-list' ? ' blocking' : ''
              }`}
              data-testid={`compose-finding-${finding.id}`}
            >
              <p className="finding-head">
                <span className="finding-kind">{finding.kind}</span>
                {finding.lane && <span className="finding-lane">{finding.lane}</span>}
                <span className="finding-summary">{finding.summary}</span>
              </p>
              <p className="finding-remediation">{finding.remediation}</p>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

export function Composer({
  draft,
  verdict,
  offendingLane,
  onAdd,
  onRemove,
}: {
  draft: BlueprintDoc
  verdict: ComposeVerdict | undefined
  offendingLane: string | undefined
  onAdd: (entry: PaletteEntry, signals: string[]) => void
  onRemove: (signal: string, index: number) => void
}) {
  const entries = verdict?.palette.entries ?? []
  const stabilityOf = (ref: string, signal: string): string | undefined => {
    const local = draft.locals[ref]
    const key = local
      ? `type:${local.class}/${local.type}`
      : `shared:${ref.split('@')[0] ?? ref}`
    return entries.find((e) => e.key === key)?.stability[signal]
  }
  const flagged = (signal: string, ref: string): boolean =>
    verdict?.findings.some((f) => f.lane === signal && f.ref === ref) ?? false

  return (
    <div className="composer">
      <Palette verdict={verdict} onAdd={onAdd} />
      <div className="composer-main">
        <div className="claims-row" data-testid="claims-row">
          <span className="claims-label">satisfies</span>
          {draft.satisfies.length === 0 && <span className="item-meta">no claims</span>}
          {draft.satisfies.map((claim) => (
            // A claim of intent, never of fact (REQ-031): the chip says
            // "claims" and links to the engine's verdict, never a status.
            <span key={claim} className="claim-chip" data-testid={`claim-${claim}`}>
              claims {claim}
              <Link
                to="/compose"
                search={(prev) => ({ ...prev, surface: 'requirements' as const })}
                data-testid={`claim-verdict-${claim}`}
              >
                → verdict
              </Link>
            </span>
          ))}
        </div>
        {laneOrder(draft).map((signal) => {
          const lane = draft.lanes[signal] ?? []
          return (
            <div
              key={signal}
              className={signal === offendingLane ? 'signal-lane offending' : 'signal-lane'}
              data-testid={`lane-${signal}`}
              onDragOver={(event) => event.preventDefault()}
              onDrop={(event) => {
                event.preventDefault()
                const entry = droppedEntry(event)
                // The drop target names the signal (ADR-0043 §4).
                if (entry) onAdd(entry, [signal])
              }}
            >
              <header className="lane-head">
                <h3>{signal}</h3>
                {verdict?.context.floor && (
                  <span className="floor-chip" data-testid={`floor-chip-${signal}`}>
                    floor {verdict.context.floor} ({verdict.context.serviceClass} ·{' '}
                    {verdict.context.environment})
                  </span>
                )}
              </header>
              <ol className="lane-components">
                {lane.map((ref, i) => (
                  <li
                    key={`${ref}-${i}`}
                    className={flagged(signal, ref) ? 'lane-entry flagged' : 'lane-entry'}
                  >
                    <span className="lane-ref">{ref}</span>
                    {stabilityOf(ref, signal) && (
                      <span className="stability-chip">{stabilityOf(ref, signal)}</span>
                    )}
                    <button
                      type="button"
                      className="lane-remove"
                      data-testid={`remove-${signal}-${i}`}
                      aria-label={`Remove ${ref} from ${signal}`}
                      onClick={() => onRemove(signal, i)}
                    >
                      ×
                    </button>
                  </li>
                ))}
              </ol>
              <label className="lane-add">
                + add to «{signal}»
                <select
                  data-testid={`lane-add-${signal}`}
                  value=""
                  onChange={(event) => {
                    const entry = entries.find((e) => e.key === event.target.value)
                    if (entry) onAdd(entry, [signal])
                  }}
                >
                  <option value="">choose a component…</option>
                  {entries
                    .filter((e) => e.signals.includes(signal))
                    .map((e) => (
                      <option key={e.key} value={e.key}>
                        {e.label}
                        {e.state === 'greyed' ? ' (below floor)' : ''}
                      </option>
                    ))}
                </select>
              </label>
            </div>
          )
        })}
        {draft.extensions.length > 0 && (
          <div className="signal-lane" data-testid="lane-extensions">
            <header className="lane-head">
              <h3>extensions</h3>
            </header>
            <ol className="lane-components">
              {draft.extensions.map((ref, i) => (
                <li key={`${ref}-${i}`} className="lane-entry">
                  <span className="lane-ref">{ref}</span>
                </li>
              ))}
            </ol>
          </div>
        )}
        <FindingsStrip findings={verdict?.findings ?? []} />
      </div>
    </div>
  )
}
