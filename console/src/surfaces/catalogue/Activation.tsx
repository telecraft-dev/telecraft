import { useMutation, useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import type {
  ActivationCandidate,
  ActivationRecord,
  ProposalOutcome,
  SubstrateActivation,
} from '../../api/types'
import { Button } from '../../ui/Button'

/**
 * The activation surface: which version of each Catalogue-pattern substrate
 * the estate judges against, what activating each retained version would
 * change, and every activation so far (ADR-0020 §6, §9, ADR-0034 §1).
 *
 * The report is computed before activation and shown in full, and the
 * control is offered only to an operator: the server answers whether this
 * user is one, and this surface honours the answer rather than re-deriving
 * it, exactly as it does for editableTeams.
 */
export function ActivationView() {
  const activations = useQuery({ queryKey: ['activations'], queryFn: api.activations })
  const me = useQuery({ queryKey: ['me'], queryFn: api.me })

  if (activations.isPending || me.isPending) {
    return <p className="surface-status">Loading the activations…</p>
  }
  if (activations.isError || me.isError) {
    return <p className="surface-status">The activations failed to load.</p>
  }

  return (
    <div className="activation-layout" data-testid="activation-view">
      {activations.data.substrates.map((substrate) => (
        <Substrate key={substrate.kind} substrate={substrate} operator={me.data.operator} />
      ))}
    </div>
  )
}

function Substrate({
  substrate,
  operator,
}: {
  substrate: SubstrateActivation
  operator: boolean
}) {
  return (
    <section className="activation-substrate" data-testid={`substrate-${substrate.kind}`}>
      <header className="activation-header">
        <h2>{substrate.name}</h2>
        <p className="activation-active" data-testid={`active-${substrate.kind}`}>
          {substrate.active === ''
            ? 'No version is active. Import a version and activate it.'
            : `Active: ${substrate.active}`}
        </p>
      </header>

      <h3>On offer</h3>
      {substrate.candidates.length === 0 ? (
        <p className="surface-status">
          {substrate.active === ''
            ? 'Nothing is installed to activate.'
            : 'Nothing else is installed. The active version is the only one imported.'}
        </p>
      ) : (
        <ul className="activation-candidates">
          {substrate.candidates.map((candidate) => (
            <Candidate
              key={candidate.version}
              candidate={candidate}
              substrate={substrate}
              operator={operator}
            />
          ))}
        </ul>
      )}

      <h3>Activations</h3>
      {substrate.history.length === 0 ? (
        <p className="surface-status">The {substrate.name} has never been activated.</p>
      ) : (
        <ol className="activation-history">
          {[...substrate.history].reverse().map((record) => (
            <Record key={`${record.version}-${record.at}`} record={record} />
          ))}
        </ol>
      )}
    </section>
  )
}

function Candidate({
  candidate,
  substrate,
  operator,
}: {
  candidate: ActivationCandidate
  substrate: SubstrateActivation
  operator: boolean
}) {
  // Activating leaves the console as a pull request, like every other
  // change to the estate: the operator decides, and the review is the
  // record of the decision (ADR-0028, ADR-0042 §6).
  const propose = useMutation<ProposalOutcome>({
    mutationFn: () => api.proposeActivation({ kind: substrate.kind, version: candidate.version }),
  })
  const readable = candidate.blocked === undefined || candidate.blocked === ''

  return (
    <li className="activation-candidate" data-testid={`candidate-${candidate.version}`}>
      <div className="activation-candidate-head">
        <span className="activation-version">{candidate.version}</span>
        {operator ? (
          <Button
            tone="primary"
            data-testid={`activate-${substrate.kind}-${candidate.version}`}
            disabled={!readable || propose.isPending}
            onClick={() => propose.mutate()}
          >
            Activate {candidate.version}
          </Button>
        ) : (
          <span className="activation-withheld" data-testid={`withheld-${candidate.version}`}>
            Activating a version is an operator's to do.
          </span>
        )}
      </div>
      {readable ? (
        <>
          <p className="activation-summary">{candidate.summary}</p>
          <Lines lines={candidate.lines} />
        </>
      ) : (
        <p className="activation-summary">{candidate.blocked}</p>
      )}
      <Outcome outcome={propose.data} failed={propose.isError} />
    </li>
  )
}

function Outcome({ outcome, failed }: { outcome?: ProposalOutcome; failed: boolean }) {
  if (failed) {
    return <p className="activation-outcome">The activation could not be proposed.</p>
  }
  if (outcome === undefined) return null
  if (outcome.proposal !== undefined) {
    return (
      <p className="activation-outcome">
        Proposed as{' '}
        <a href={outcome.proposal.url} rel="noreferrer">
          {outcome.proposal.id}
        </a>
        . The review decides.
      </p>
    )
  }
  return (
    <ul className="activation-problems">
      {(outcome.problems ?? []).map((problem) => (
        <li key={problem}>{problem}</li>
      ))}
    </ul>
  )
}

function Record({ record }: { record: ActivationRecord }) {
  return (
    <li className="activation-record">
      <div className="activation-candidate-head">
        <span className="activation-version">
          {record.previous === undefined || record.previous === ''
            ? record.version
            : `${record.previous} to ${record.version}`}
        </span>
        <span className="activation-by">
          {record.by} on {record.at.slice(0, 10)}
        </span>
      </div>
      <p className="activation-summary">{record.summary}</p>
      <Lines lines={record.lines} />
    </li>
  )
}

function Lines({ lines }: { lines: string[] }) {
  if (lines.length === 0) return null
  return (
    <ul className="activation-lines">
      {lines.map((line) => (
        <li key={line}>{line}</li>
      ))}
    </ul>
  )
}
