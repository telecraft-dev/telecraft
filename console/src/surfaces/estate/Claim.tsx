import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { api } from '../../api/client'
import type { ClaimCandidate, CollectorRow, EstatePayload, Selector } from '../../api/types'
import { useLens } from '../../chrome/LensControl'
import { formatSelector, suggestSelector } from '../../estate/claim'
import { usePresentation } from '../../presentation/usePresentation'
import { Button } from '../../ui/Button'
import { count } from '../../ui/text'
import { Panel } from '../../ui/Panel'

/**
 * The claim flow (ADR-0042 §6, ADR-0031): ungoverned to governed, in one
 * panel every ungoverned representation converges on. Herd-first: the
 * flat list multi-selects and this flow operates on the selection. The
 * suggested selector generalises over the herd's shared identity
 * attributes (evidence supplied by the Unmatched artefact's
 * self-telemetry, ADR-0030) and never enumerates instance ids: the user
 * constrains it by removing pairs, and no gesture can add an instance id.
 * After owning team + Environment (defaulted), two paths one question
 * apart: attach to an existing Tier (candidates ranked by selector
 * proximity) or draft a new Tier (opens the Add-a-Tier panel pre-filled,
 * ADR-0060 §1: one Tier-authoring flow, two doors).
 * Exit is always a PR via the forge adapter, user-attributed (ADR-0014),
 * carrying the rendered impact preview. The console proposes, the PR
 * decides. Quarantine routing stays a Compose concern (ADR-0031).
 */
export function ClaimPanel({ payload, herd }: { payload: EstatePayload; herd: string[] }) {
  const collectors = useQuery({ queryKey: ['collectors'], queryFn: api.collectors })
  const { me } = usePresentation()
  const lens = useLens()
  const navigate = useNavigate()

  const [dropped, setDropped] = useState<ReadonlySet<string>>(new Set())
  const [team, setTeam] = useState<string>()
  const [environment, setEnvironment] = useState<string>()
  const [mode, setMode] = useState<'attach' | 'draft'>()
  const [target, setTarget] = useState<string>()
  const [name, setName] = useState('')
  const claim = useMutation({ mutationFn: api.claim })

  const rows: CollectorRow[] = (collectors.data ?? []).filter(
    (row) => row.ungoverned !== undefined && herd.includes(row.id),
  )

  // The suggestion generalises over shared identity attributes; the user's
  // constraint only ever removes pairs (ADR-0042 §6).
  const suggested = suggestSelector(rows)
  const selector: Selector = Object.fromEntries(
    Object.entries(suggested).filter(([key]) => !dropped.has(key)),
  )
  const terms = Object.keys(selector).length

  // Defaults: the acting human's team, and the herd's Environment when it
  // agrees, and the lens otherwise (ADR-0042 §4: the lens as context).
  const herdEnvironments = [...new Set(rows.map((row) => row.environment))]
  const chosenTeam = team ?? me?.team ?? ''
  const chosenEnvironment =
    environment ?? (herdEnvironments.length === 1 ? herdEnvironments[0] : undefined) ?? lens
  const draftTier = name.trim() === '' ? undefined : `${chosenTeam}/${name.trim()}`
  const chosenTier = mode === 'attach' ? target : mode === 'draft' ? draftTier : undefined

  const preview = useQuery({
    queryKey: ['claim-preview', selector, chosenEnvironment, chosenTeam, mode, chosenTier],
    queryFn: () =>
      api.claimPreview({
        selector,
        environment: chosenEnvironment,
        team: chosenTeam,
        ...(mode ? { mode } : {}),
        ...(chosenTier ? { tier: chosenTier } : {}),
      }),
    enabled: rows.length > 0 && terms > 0,
    placeholderData: keepPreviousData,
  })

  const close = () =>
    void navigate({
      to: '.',
      search: (prev) => ({ ...prev, herd: undefined }),
    })

  if (collectors.isPending) return <aside className="panel">Loading the collectors…</aside>
  if (rows.length === 0) return null

  const attach = () => {
    if (target === undefined) return
    claim.mutate({
      selector,
      environment: chosenEnvironment,
      team: chosenTeam,
      mode: 'attach',
      tier: target,
      title: `Claim ${rows.length} ungoverned collectors into ${target}`,
    })
  }

  // The draft path is the claim flow's door into the Add-a-Tier panel
  // (ADR-0060 §1): one Tier-authoring flow, pre-filled from the herd's
  // shared attributes, the two never forking. The Compose claim handoff
  // stays URL-reachable; this branch no longer routes there.
  const draft = () => {
    if (draftTier === undefined) return
    void navigate({
      to: '.',
      search: (prev) => ({
        ...prev,
        herd: undefined,
        add: true,
        name: name.trim() || undefined,
        selector: formatSelector(selector),
        team: chosenTeam,
        env: chosenEnvironment,
      }),
    })
  }

  return (
    <Panel
      name="claim"
      testId="claim-panel"
      className="claim-panel"
      initialWidth={400}
      title={`Claim ${count(rows.length, 'collector')}`}
      titleTestId="claim-title"
      closeTestId="claim-close"
      onClose={close}
    >
      {/* The herd: live identity from the Unmatched artefact's
          self-telemetry (alive, version X, since when; ADR-0030). */}
      <ul className="claim-herd" data-testid="claim-herd">
        {rows.map((row) => (
          <li key={row.id}>
            <span className="mono">{row.id}</span> · {row.state.replace('_', ' ')} · v
            {row.version} ·{' '}
            {row.ungoverned === 'served' ? 'served the Unmatched artefact' : 'foreign'}
            {row.lastSeen ? ` · seen ${row.lastSeen}` : ''}
          </li>
        ))}
      </ul>

      <section className="claim-section">
        <h3>Suggested selector</h3>
        {/* The suggestion is shared identity attributes only, never a
            list of instance ids (ADR-0042 §6). */}
        <p className="section-summary">
          Built from the identity attributes the collectors share. Narrow it by removing
          pairs.
        </p>
        {Object.keys(suggested).length === 0 ? (
          <p className="surface-status" data-testid="claim-no-shared">
            These collectors share no identity attributes. Narrow the selection until
            they share at least one.
          </p>
        ) : (
          <ul className="claim-terms" data-testid="claim-terms">
            {Object.entries(suggested).map(([key, value]) => (
              <li key={key}>
                <label>
                  <input
                    type="checkbox"
                    data-testid={`claim-term-${key}`}
                    checked={!dropped.has(key)}
                    onChange={() => {
                      const next = new Set(dropped)
                      if (next.has(key)) next.delete(key)
                      else next.add(key)
                      setDropped(next)
                    }}
                  />
                  <code>
                    {key}={value}
                  </code>
                </label>
              </li>
            ))}
          </ul>
        )}
        {terms > 0 && (
          <p className="claim-selector">
            <code data-testid="claim-selector">{formatSelector(selector)}</code>
          </p>
        )}
      </section>

      <section className="claim-section claim-context">
        <label>
          Owning team
          <select
            data-testid="claim-team"
            value={chosenTeam}
            onChange={(event) => setTeam(event.target.value)}
          >
            {(me?.editableTeams ?? []).map((id) => (
              <option key={id} value={id}>
                {id}
              </option>
            ))}
          </select>
        </label>
        <label>
          Environment
          <select
            data-testid="claim-environment"
            value={chosenEnvironment}
            onChange={(event) => setEnvironment(event.target.value)}
          >
            {payload.environments.map((env) => (
              <option key={env} value={env}>
                {env}
              </option>
            ))}
          </select>
        </label>
      </section>

      {preview.data && terms > 0 && (
        <section className="claim-section" data-testid="claim-impact">
          <h3>Impact</h3>
          <p data-testid="claim-matched">
            Matches {preview.data.matched.total} ungoverned collectors (
            {preview.data.matched.served} served the Unmatched artefact,{' '}
            {preview.data.matched.foreign} foreign).
          </p>
          {preview.data.overlaps.length > 0 && (
            <ul className="claim-overlaps" data-testid="claim-overlaps">
              {preview.data.overlaps.map((overlap) => (
                <li key={overlap.tier}>
                  May also reach {count(overlap.matched, 'collector')} matched today by{' '}
                  <span className="mono">{overlap.tier}</span>. The pull request review
                  judges that reach.
                </li>
              ))}
            </ul>
          )}
          {preview.data.rendered && (
            <pre className="claim-rendered" data-testid="claim-rendered">
              {preview.data.rendered}
            </pre>
          )}
        </section>
      )}

      <section className="claim-section">
        <h3>Choose a Tier</h3>
        <label className="claim-path">
          <input
            type="radio"
            name="claim-path"
            data-testid="claim-path-attach"
            checked={mode === 'attach'}
            onChange={() => setMode('attach')}
          />
          Attach to an existing Tier
        </label>
        {mode === 'attach' && (
          <ul className="claim-candidates" data-testid="claim-candidates">
            {(preview.data?.candidates ?? []).map((candidate: ClaimCandidate) => (
              <li key={candidate.tier}>
                <label>
                  <input
                    type="radio"
                    name="claim-candidate"
                    data-testid={`claim-candidate-${candidate.tier}`}
                    checked={target === candidate.tier}
                    onChange={() => setTarget(candidate.tier)}
                  />
                  <span className="mono">{candidate.tier}</span>: satisfies{' '}
                  {candidate.satisfied} of {candidate.of} selector pairs, widens to{' '}
                  <code>{formatSelector(candidate.widened)}</code>
                </label>
              </li>
            ))}
            {preview.data !== undefined && preview.data.candidates.length === 0 && (
              <li className="section-summary">
                No Tier&rsquo;s selector shares a pair with this one. Draft a new Tier
                instead.
              </li>
            )}
          </ul>
        )}
        <label className="claim-path">
          <input
            type="radio"
            name="claim-path"
            data-testid="claim-path-draft"
            checked={mode === 'draft'}
            onChange={() => setMode('draft')}
          />
          Draft a new Tier
        </label>
        {mode === 'draft' && (
          <label className="claim-name">
            Tier name
            <input
              data-testid="claim-tier-name"
              value={name}
              placeholder="payments-edge"
              onChange={(event) => setName(event.target.value)}
            />
          </label>
        )}
      </section>

      <section className="claim-section">
        {mode === 'attach' && (
          <Button
            tone="primary"
            className="propose-button"
            data-testid="claim-propose"
            disabled={terms === 0 || target === undefined || claim.isPending}
            onClick={attach}
          >
            {claim.isPending ? 'Proposing…' : 'Propose the claim as a pull request'}
          </Button>
        )}
        {mode === 'draft' && (
          <Button
            tone="primary"
            className="propose-button"
            data-testid="claim-draft"
            disabled={terms === 0 || draftTier === undefined}
            onClick={draft}
          >
            Draft the Tier with this selector
          </Button>
        )}
        {claim.isError && (
          <p className="surface-status">The claim could not be submitted.</p>
        )}
        {claim.data?.problems && (
          <ul className="proposal-problems" data-testid="claim-problems">
            {claim.data.problems.map((problem) => (
              <li key={problem}>{problem}</li>
            ))}
          </ul>
        )}
        {claim.data?.proposal && (
          <div className="proposal-opened" data-testid="claim-opened">
            <p>
              Proposal <span className="mono">{claim.data.proposal.id}</span> opened on
              branch <span className="mono">{claim.data.proposal.branch}</span>:{' '}
              <a href={claim.data.proposal.url} data-testid="claim-proposal-url">
                {claim.data.proposal.url}
              </a>
              . Once it merges and the configuration is served, these collectors show as
              governed.
            </p>
            <p className="item-meta" data-testid="claim-attribution">
              Attributed to {claim.data.proposal.attributedTo}.
            </p>
          </div>
        )}
      </section>
    </Panel>
  )
}
