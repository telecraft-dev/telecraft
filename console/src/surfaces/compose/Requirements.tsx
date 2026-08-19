import type { ComposeVerdict, RequirementVerdict } from '../../api/types'
import { Button } from '../../ui/Button'

/**
 * B · Requirement-first, the compliance overview (ADR-0043 §1): what this
 * Service owes, coverage, and one-click suggestion adds. The claim column
 * is the draft's `satisfies` intent; the verdict column is the engine's
 * judgement — carried side by side and never blurred (REQ-031). Claims are
 * judged against the requirement's current version whatever version they
 * stamp (ADR-0026 §5).
 */
export function Requirements({
  verdict,
  editable,
  onSuggest,
}: {
  verdict: ComposeVerdict | undefined
  editable: boolean
  onSuggest: (row: RequirementVerdict) => void
}) {
  if (verdict === undefined) return <p className="surface-status">Judging the draft…</p>
  const rows = verdict.requirements
  const met = rows.filter((r) => r.met).length

  return (
    <div className="requirements-surface">
      <p className="coverage" data-testid="coverage">
        {met}/{rows.length} requirements met by the current draft
      </p>
      <div className="coverage-bar" aria-hidden="true">
        <div
          className="coverage-fill"
          style={{ width: rows.length === 0 ? '0%' : `${(met / rows.length) * 100}%` }}
        />
      </div>
      <table className="requirements-table" data-testid="requirements-table">
        <thead>
          <tr>
            <th>Requirement</th>
            <th>Claim (intent)</th>
            <th>Verdict (fact)</th>
            <th>Action</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.id} data-testid={`requirement-${row.id}`}>
              <td>
                <span className="req-id">
                  {row.id}@{row.version}
                </span>
                <p className="req-summary">{row.summary}</p>
              </td>
              <td className="claim-cell" data-testid={`claimed-${row.id}`}>
                {row.claimed ? `claimed @${row.claimedVersion ?? row.version}` : 'unclaimed'}
              </td>
              <td
                className={row.met ? 'verdict-cell met' : 'verdict-cell unmet'}
                data-testid={`verdict-${row.id}`}
              >
                {row.met ? 'met' : 'not met'}
              </td>
              <td>
                {!row.met && (
                  <>
                    {editable && (
                      <Button
                        data-testid={`suggest-${row.id}`}
                        onClick={() => onSuggest(row)}
                      >
                        Add {row.suggestion.ref ?? row.suggestion.type} to{' '}
                        {row.suggestion.signals.join(', ')}
                      </Button>
                    )}
                    <p className="finding-remediation">{row.remediation}</p>
                  </>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="item-meta">
        A `satisfies` entry is a claim of intent; the verdict column is the engine's judgement of
        the draft — the two never blend (REQ-031).
      </p>
    </div>
  )
}
