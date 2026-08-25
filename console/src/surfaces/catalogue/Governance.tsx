import { useMutation, useQuery } from '@tanstack/react-query'
import { useSearch } from '@tanstack/react-router'
import { useState } from 'react'
import { api } from '../../api/client'
import type { GovernancePayload, TeamNode } from '../../api/types'
import { Button } from '../../ui/Button'

function flattenTeams(node: TeamNode, out: { id: string; name: string }[] = []) {
  out.push({ id: node.id, name: node.name })
  for (const child of node.teams ?? []) flattenTeams(child, out)
  return out
}

function splitEntries(text: string): string[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '')
}

/**
 * The governance editor: Allow-lists and Grants in their authored,
 * git-resident shapes (ADR-0021 §5), edited here and exiting only as a PR
 * via the forge adapter (ADR-0042 §6). The console proposes, the PR
 * decides. A refused proposal comes back with the load problems named,
 * exactly the fail-closed shape the estate loader enforces.
 */
export function GovernanceView() {
  const governance = useQuery({ queryKey: ['governance'], queryFn: api.governance })
  const estate = useQuery({ queryKey: ['estate'], queryFn: api.estate })
  const search = useSearch({ from: '/catalogue' })

  if (governance.isPending || estate.isPending) {
    return <p className="surface-status">Loading the governance policy…</p>
  }
  if (governance.isError || estate.isError) {
    return <p className="surface-status">The governance policy failed to load.</p>
  }

  return (
    <GovernanceEditor
      payload={governance.data}
      teams={flattenTeams(estate.data.teams)}
      request={search.request}
      targetTeam={search.team}
    />
  )
}

function GovernanceEditor({
  payload,
  teams,
  request,
  targetTeam,
}: {
  payload: GovernancePayload
  teams: { id: string; name: string }[]
  request?: string
  targetTeam?: string
}) {
  const [title, setTitle] = useState(
    request ? `Grant ${request} to ${targetTeam ?? 'a team'}` : 'Update the Allow-list policy',
  )
  const [lists, setLists] = useState(() =>
    payload.allowLists.map((list) => ({
      team: list.team,
      owner: list.owner,
      text: list.allow.join('\n'),
    })),
  )
  const [grant, setGrant] = useState(() => ({
    id: request ? `${request.replace('/', '-')}-for-${targetTeam ?? 'team'}` : '',
    owner: '',
    team: targetTeam ?? '',
    adds: request ?? '',
  }))
  const propose = useMutation({ mutationFn: api.proposeGovernance })

  const grantDrafted = grant.id.trim() !== '' || grant.adds.trim() !== ''

  const submit = () =>
    propose.mutate({
      title,
      allowLists: lists.map((list) => ({
        team: list.team,
        owner: list.owner,
        allow: splitEntries(list.text),
      })),
      grants: [
        ...payload.grants,
        ...(grantDrafted
          ? [
              {
                id: grant.id.trim(),
                owner: grant.owner,
                team: grant.team,
                adds: splitEntries(grant.adds),
              },
            ]
          : []),
      ],
    })

  return (
    <div className="governance-view">
      <section className="governance-section">
        <h2>Allow-lists</h2>
        <p className="section-summary">
          Each entry is a pattern of the form <code>class/type-pattern</code>, one per line. A
          team&rsquo;s list can only narrow its parent&rsquo;s effective list. An empty list is
          refused: to inherit the parent&rsquo;s list unchanged, declare no list at all.
        </p>
        {lists.map((list, i) => (
          <div key={list.team} className="governance-list">
            <h3>
              {list.team} <span className="item-meta">owner {list.owner}</span>
            </h3>
            <textarea
              data-testid={`allowlist-entries-${list.team}`}
              rows={Math.max(4, list.text.split('\n').length + 1)}
              value={list.text}
              onChange={(event) =>
                setLists(lists.map((l, j) => (i === j ? { ...l, text: event.target.value } : l)))
              }
            />
          </div>
        ))}
      </section>
      <section className="governance-section">
        <h2>Grants</h2>
        <ul className="governance-grants">
          {payload.grants.map((existing) => (
            <li key={existing.id} data-testid={`grant-${existing.id}`}>
              <span className="mono">{existing.id}</span>: {existing.owner} grants{' '}
              {existing.team} <span className="mono">{existing.adds.join(', ')}</span>
            </li>
          ))}
        </ul>
        <h3>New Grant</h3>
        <p className="section-summary">
          A Grant is an exception a parent team makes for a team below it. Its owner&rsquo;s
          team must sit above the target, and it allows its entries for the target and every
          team below it.
        </p>
        <div className="governance-form">
          <label>
            Id
            <input
              data-testid="grant-id"
              value={grant.id}
              onChange={(event) => setGrant({ ...grant, id: event.target.value })}
            />
          </label>
          <label>
            Owner
            <select
              data-testid="grant-owner"
              value={grant.owner}
              onChange={(event) => setGrant({ ...grant, owner: event.target.value })}
            >
              <option value="">choose an owner</option>
              {payload.owners.map((owner) => (
                <option key={owner.id} value={owner.id}>
                  {owner.name} ({owner.team})
                </option>
              ))}
            </select>
          </label>
          <label>
            Target team
            <select
              data-testid="grant-team"
              value={grant.team}
              onChange={(event) => setGrant({ ...grant, team: event.target.value })}
            >
              <option value="">choose a team</option>
              {teams.map((node) => (
                <option key={node.id} value={node.id}>
                  {node.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Adds
            <textarea
              data-testid="grant-adds"
              rows={3}
              value={grant.adds}
              onChange={(event) => setGrant({ ...grant, adds: event.target.value })}
            />
          </label>
        </div>
      </section>
      <section className="governance-section">
        <h2>Propose</h2>
        {/* The console never writes the policy itself: the proposal goes
            through pull request review (ADR-0028). */}
        <p className="section-summary">
          Proposing opens a pull request attributed to you.
        </p>
        <div className="governance-form">
          <label>
            Title
            <input
              data-testid="proposal-title"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
            />
          </label>
          <Button
            tone="primary"
            data-testid="propose"
            className="propose-button"
            disabled={propose.isPending}
            onClick={submit}
          >
            {propose.isPending ? 'Proposing…' : 'Propose as a pull request'}
          </Button>
        </div>
        {propose.isError && (
          <p className="surface-status">The proposal could not be submitted.</p>
        )}
        {propose.data?.problems && (
          <ul className="proposal-problems" data-testid="proposal-problems">
            {propose.data.problems.map((problem) => (
              <li key={problem}>{problem}</li>
            ))}
          </ul>
        )}
        {propose.data?.proposal && (
          <p className="proposal-opened" data-testid="proposal-opened">
            Proposal <span className="mono">{propose.data.proposal.id}</span> opened on branch{' '}
            <span className="mono">{propose.data.proposal.branch}</span>:{' '}
            <a href={propose.data.proposal.url} data-testid="proposal-url">
              {propose.data.proposal.url}
            </a>
          </p>
        )}
      </section>
    </div>
  )
}
