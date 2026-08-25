import { Link } from '@tanstack/react-router'
import { BAND_ORDER, type BandName, type EstatePayload } from '../../api/types'
import { useLens } from '../../chrome/LensControl'
import { type KindRollup, rollupTree } from '../../estate/rollup'
import { formatObjectRef } from '../../objectref'
import { Mark } from '../../ui/Mark'

// The tree-table roll-up (ADR-0042 §1, ADR-0017): ratio-plus-worst per
// finding kind, never blended, waived counts always alongside. The lens is
// the evaluation context (the ratios are judged under it) and every team
// row stays visible whatever it names: emphasis and context, never a
// filter (ADR-0042 §4). The all-Environments column keeps the lens honest.

const KIND_LABEL: Record<BandName, string> = {
  delivery: 'Delivery',
  expectation: 'Expectation',
  conformance: 'Conformance',
}

function KindCell({ kind, rollup }: { kind: BandName; rollup: KindRollup }) {
  // The worst verdict in the group, as the same mark the band rows use.
  const badge =
    rollup.worst === 'violation' ? 'violation' : rollup.worst === 'advisory' ? 'advisory' : null
  return (
    <td className={`rollup-kind severity-${rollup.worst}`} data-kind={kind}>
      {rollup.counted === 0 ? (
        <span className="rollup-empty">no verdicts</span>
      ) : (
        <>
          <span className="rollup-ratio">
            {rollup.passing}/{rollup.counted}
          </span>
          {badge && <Mark name={badge} />}
        </>
      )}
      {rollup.waived > 0 && <span className="rollup-waived">{rollup.waived} exempt</span>}
    </td>
  )
}

export function RollUp({ payload }: { payload: EstatePayload }) {
  const lens = useLens()
  const rows = rollupTree(payload.teams, payload.cards, lens)

  return (
    <section className="rollup" data-testid="rollup">
      <p className="section-summary">
        Judged in <strong data-testid="rollup-lens">{lens}</strong>. Exempt counts appear at
        every level.
      </p>
      <table className="catalogue-table rollup-table" data-testid="rollup-table">
        <thead>
          <tr>
            <th>Team</th>
            <th>Tiers ({lens} / all)</th>
            {BAND_ORDER.map((kind) => (
              <th key={kind}>{KIND_LABEL[kind]}</th>
            ))}
            <th>Neutral</th>
            <th>All Environments</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.team.id} data-testid={`rollup-${row.team.id}`}>
              <td className="rollup-team" style={{ paddingLeft: `calc(var(--space-3) * ${row.depth + 1})` }}>
                <Link
                  to="/estate"
                  search={(prev) => ({
                    ...prev,
                    view: 'shelf' as const,
                    object: formatObjectRef({ kind: 'team', id: row.team.id }),
                  })}
                  className="count-door"
                >
                  {row.team.name}
                </Link>
              </td>
              <td>
                {row.tiersInEnvironment} / {row.tiersTotal}
              </td>
              {BAND_ORDER.map((kind) => (
                <KindCell key={kind} kind={kind} rollup={row.kinds[kind]} />
              ))}
              <td>{row.neutral}</td>
              <td className="rollup-all" data-testid={`rollup-all-${row.team.id}`}>
                {row.findingsAllEnvironments} finding
                {row.findingsAllEnvironments === 1 ? '' : 's'}, {row.waivedAllEnvironments}{' '}
                exempt
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}
