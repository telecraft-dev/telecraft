import { useNavigate } from '@tanstack/react-router'
import type { BandName, BandState, CardFace, Severity } from '../../api/types'
import { BAND_ORDER } from '../../api/types'
import type { CardStanding } from '../../estate/order'
import { totalFindings } from '../../estate/order'

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
    <button
      type="button"
      className={`card-face standing-${standing}${selected ? ' selected' : ''}`}
      data-testid={`card-${card.tier}`}
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
      <footer className="card-foot">
        <span>{card.population.matched} matched</span>
        <span>{totalFindings(card)} findings</span>
      </footer>
    </button>
  )
}

/** The card panel: the drawer-summoning side panel, in place (ADR-0042 §3.2). */
export function CardPanel({ card }: { card: CardFace }) {
  const navigate = useNavigate()
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
            <dd>{card.serviceClass}</dd>
          </>
        )}
        <dt>Population</dt>
        <dd>
          {card.population.matched} matched
          {card.population.floor !== undefined &&
            `, floor ${card.population.floor} (${card.population.floorSource})`}
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
              {worstFinding && <p className="band-finding">{worstFinding}</p>}
            </li>
          )
        })}
      </ul>
    </aside>
  )
}
