import { useMutation, useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { useState } from 'react'
import { api } from '../../api/client'
import type { EstatePayload, Selector, TeamNode } from '../../api/types'
import { useLens } from '../../chrome/LensControl'
import { parseSelector } from '../../estate/claim'
import { usePresentation } from '../../presentation/usePresentation'
import { Button, buttonClass } from '../../ui/Button'
import { Panel } from '../../ui/Panel'

/**
 * The Add-a-Tier flow (ADR-0060 §1, §2): one Tier-authoring flow, two
 * doors. The Estate header's door opens it empty; the claim flow's
 * draft branch opens it pre-filled from the herd's shared attributes,
 * and the two never fork. The pre-fills ride the URL: `add` opens the
 * panel, `name` and `selector` seed their fields, `blueprint` rides as
 * `id@version` from the Blueprints view door (ADR-0061), and the
 * pre-existing flat-list filters `team` and `env` double as the owning
 * Team and Environment pre-fills while `add` is set. The exit is a pull
 * request through the forge adapter (ADR-0028), user-attributed
 * (ADR-0014): the console proposes, the PR decides.
 */

// The same four-line walker FlatList and Governance keep locally: a
// surface does not import across surfaces for it.
function flattenTeams(root: TeamNode): TeamNode[] {
  const out: TeamNode[] = []
  const walk = (team: TeamNode) => {
    out.push(team)
    for (const child of team.teams ?? []) walk(child)
  }
  walk(root)
  return out
}

/** Parses the textarea's one `key=value` per line; malformed lines drop. */
function parseSelectorLines(text: string): Selector {
  const out: Selector = {}
  for (const line of text.split('\n')) {
    const cut = line.indexOf('=')
    if (cut > 0) out[line.slice(0, cut).trim()] = line.slice(cut + 1).trim()
  }
  return out
}

/** Renders a selector as the textarea's one `key=value` per line. */
function selectorLines(selector: Selector): string {
  return Object.keys(selector)
    .sort()
    .map((key) => `${key}=${selector[key]}`)
    .join('\n')
}

export function AddTierPanel({ payload }: { payload: EstatePayload }) {
  const search = useSearch({ from: '/estate' })
  const blueprints = useQuery({ queryKey: ['blueprints'], queryFn: api.blueprints })
  const governance = useQuery({ queryKey: ['governance'], queryFn: api.governance })
  const { me } = usePresentation()
  const lens = useLens()
  const navigate = useNavigate()

  const [name, setName] = useState(search.name ?? '')
  const [team, setTeam] = useState<string>()
  const [owner, setOwner] = useState('')
  const [environment, setEnvironment] = useState<string>()
  const [blueprint, setBlueprint] = useState(search.blueprint ?? '')
  const [selectorText, setSelectorText] = useState(() =>
    selectorLines(parseSelector(search.selector)),
  )
  const [minExpected, setMinExpected] = useState('')
  // The title's default follows the fields until the first edit takes it
  // over; after that the typed title stands.
  const [editedTitle, setEditedTitle] = useState<string>()
  const propose = useMutation({ mutationFn: api.proposeTier })

  // Defaults mirror the claim flow's: the acting human's team, and the
  // lens as the Environment context (ADR-0042 §4), each behind an
  // explicit pre-fill from the URL.
  const chosenTeam = team ?? search.team ?? me?.team ?? ''
  const chosenEnvironment = environment ?? search.env ?? lens
  const trimmedName = name.trim()
  // The id is composed server-side as `<team>/<name>`; the preview shows
  // the same composition.
  const tierId = `${chosenTeam}/${trimmedName}`
  const selector = parseSelectorLines(selectorText)
  const pairs = Object.keys(selector).length
  const owners = (governance.data?.owners ?? []).filter((o) => o.team === chosenTeam)
  const title = editedTitle ?? (trimmedName === '' ? 'Add a Tier' : `Add Tier ${tierId}`)

  const close = () =>
    void navigate({
      to: '.',
      search: (prev) => ({
        ...prev,
        add: undefined,
        blueprint: undefined,
        name: undefined,
        selector: undefined,
      }),
    })

  const submit = () => {
    const at = blueprint.lastIndexOf('@')
    propose.mutate({
      title,
      name: trimmedName,
      team: chosenTeam,
      owner,
      environment: chosenEnvironment,
      blueprint: blueprint.slice(0, at),
      blueprintVersion: Number(blueprint.slice(at + 1)),
      selector,
      ...(minExpected === '' ? {} : { minExpected: Number(minExpected) }),
    })
  }

  const incomplete =
    trimmedName === '' ||
    chosenTeam === '' ||
    owner === '' ||
    chosenEnvironment === '' ||
    blueprint === '' ||
    pairs === 0

  return (
    <Panel
      name="add-tier"
      testId="add-tier-panel"
      className="add-tier-panel"
      initialWidth={400}
      title="Add a Tier"
      titleTestId="add-tier-title"
      closeTestId="add-tier-close"
      onClose={close}
    >
      <section className="add-tier-section">
        <label>
          Tier name
          <input
            data-testid="tier-name"
            value={name}
            placeholder="payments-edge"
            onChange={(event) => setName(event.target.value)}
          />
        </label>
        <p className="item-meta" data-testid="tier-id-preview">
          {tierId}
        </p>
      </section>

      <section className="add-tier-section add-tier-context">
        <label>
          Owning team
          <select
            data-testid="tier-team"
            value={chosenTeam}
            onChange={(event) => {
              setTeam(event.target.value)
              // An Owner belongs to one Team; a new Team starts the
              // choice over.
              setOwner('')
            }}
          >
            {flattenTeams(payload.teams).map((node) => (
              <option key={node.id} value={node.id}>
                {node.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Owner
          <select
            data-testid="tier-owner"
            value={owner}
            onChange={(event) => setOwner(event.target.value)}
          >
            <option value="">choose an Owner</option>
            {owners.map((o) => (
              <option key={o.id} value={o.id}>
                {o.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Environment
          <select
            data-testid="tier-environment"
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

      <section className="add-tier-section">
        <label>
          Blueprint
          <select
            data-testid="tier-blueprint"
            value={blueprint}
            onChange={(event) => setBlueprint(event.target.value)}
          >
            <option value="">choose a Blueprint</option>
            {(blueprints.data ?? []).map((doc) => (
              <option key={doc.id} value={`${doc.id}@${doc.version}`}>
                {doc.name} v{doc.version} ({doc.team})
              </option>
            ))}
          </select>
        </label>
        {/* The picker doors into the Blueprints view (ADR-0061 §3). */}
        <Link
          to="/compose"
          search={{ browse: true }}
          className={buttonClass('quiet')}
          data-testid="browse-blueprints-door"
        >
          Browse Blueprints
        </Link>
      </section>

      <section className="add-tier-section">
        <h3>Selector</h3>
        <p className="section-summary">
          The identity attributes a collector must report to join this Tier.
        </p>
        <textarea
          aria-label="Selector"
          data-testid="tier-selector"
          value={selectorText}
          rows={4}
          placeholder={'deployment.environment=production\ntelecraft.tier=payments-edge'}
          onChange={(event) => setSelectorText(event.target.value)}
        />
      </section>

      <section className="add-tier-section">
        <label>
          Population floor
          <input
            data-testid="tier-min-expected"
            type="number"
            min={1}
            value={minExpected}
            onChange={(event) => setMinExpected(event.target.value)}
          />
        </label>
        <label>
          Title
          <input
            data-testid="tier-title"
            value={title}
            onChange={(event) => setEditedTitle(event.target.value)}
          />
        </label>
      </section>

      <section className="add-tier-section">
        <Button
          tone="primary"
          className="propose-button"
          data-testid="tier-propose"
          disabled={propose.isPending || incomplete}
          onClick={submit}
        >
          {propose.isPending ? 'Proposing…' : 'Propose as a pull request'}
        </Button>
        {propose.isError && (
          <p className="surface-status">The proposal could not be submitted.</p>
        )}
        {propose.data?.problems && (
          <ul className="proposal-problems" data-testid="tier-problems">
            {propose.data.problems.map((problem) => (
              <li key={problem}>{problem}</li>
            ))}
          </ul>
        )}
        {propose.data?.proposal && (
          <p className="proposal-opened" data-testid="tier-opened">
            Proposal <span className="mono">{propose.data.proposal.id}</span> opened on
            branch <span className="mono">{propose.data.proposal.branch}</span>:{' '}
            <a href={propose.data.proposal.url} data-testid="tier-proposal-url">
              {propose.data.proposal.url}
            </a>
          </p>
        )}
      </section>
    </Panel>
  )
}
