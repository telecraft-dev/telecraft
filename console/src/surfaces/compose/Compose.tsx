import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { useState } from 'react'
import { api } from '../../api/client'
import type {
  BlueprintDoc,
  ClaimContext,
  Me,
  PaletteEntry,
  RequirementVerdict,
} from '../../api/types'
import { canActOn } from '../../auth/authz'
import { useLens } from '../../chrome/LensControl'
import { formatSelector, parseSelector } from '../../estate/claim'
import { formatObjectRef, parseObjectRef } from '../../objectref'
import type { ComposeSurface } from '../../router'
import { Composer } from './Composer'
import { NodeCanvas } from './NodeCanvas'
import { Requirements } from './Requirements'
import { YamlFlyout } from './YamlFlyout'
import { addEntry, addSuggestion, removeEntry } from './draft'
import { Button, buttonClass } from '../../ui/Button'

/**
 * The Compose Workspace (ADR-0043): three surfaces over the one open
 * Blueprint (A · Composer, B · Requirement-first, D · Node canvas),
 * switched without losing state, with the read-only YAML flyout resident on
 * all three (REQ-035). One validation engine re-judges every interaction
 * (ADR-0022): the workspace holds the draft and the verdict; the surfaces
 * are projections. The exit is a change proposal through the forge adapter
 * (ADR-0028): save proposes, the PR decides; the console never writes
 * live state.
 *
 * Whether a Blueprint is yours to author is the ownership tree's answer,
 * not the surface's (ADR-0019 §2): the editing gestures and Save are
 * offered exactly when the owning team is in the signed-in user's
 * editableTeams; otherwise the same surfaces render honestly read-only.
 */
export function Compose() {
  const blueprints = useQuery({ queryKey: ['blueprints'], queryFn: api.blueprints })
  const me = useQuery({ queryKey: ['me'], queryFn: api.me })
  const search = useSearch({ strict: false })
  const navigate = useNavigate()

  if (blueprints.isPending) return <p className="surface-status">Loading Blueprints…</p>
  if (blueprints.isError) return <p className="surface-status">Blueprints failed to load.</p>

  const selected = parseObjectRef(search.object)
  const chosen =
    selected?.kind === 'blueprint'
      ? blueprints.data.find((bp) => bp.id === selected.id)
      : undefined

  // The claim flow's draft-new-Tier handoff (ADR-0042 §6): the claim
  // params open a fresh draft Blueprint bound to the new Tier, selector
  // pre-filled; Save proposes the Tier binding beside it.
  const claim: ClaimContext | undefined =
    search.claim !== undefined && search.tier !== undefined && search.team !== undefined
      ? {
          selector: parseSelector(search.claim),
          tier: search.tier,
          team: search.team,
          environment: search.env ?? 'production',
        }
      : undefined
  const claimDoc: BlueprintDoc | undefined = claim
    ? {
        id: `${claim.tier}-standard`,
        name: `${claim.tier.split('/').pop() ?? claim.tier}-standard`,
        version: 1,
        team: claim.team,
        tier: claim.tier,
        locals: {},
        lanes: {},
        extensions: [],
        satisfies: [],
      }
    : undefined

  return (
    <div className="compose-layout">
      <section className="compose-list" data-tour="compose">
        <h1>Compose</h1>
        {/* The list needs its noun: three bare cards read as unlabelled
            navigation, and Blueprint is the glossary's word for what they
            are. */}
        <h2 className="compose-list-label">Blueprints</h2>
        <ul>
          {blueprints.data.map((bp) => (
            <li key={bp.id}>
              <button
                type="button"
                data-testid={`blueprint-${bp.id}`}
                className={bp.id === chosen?.id ? 'list-item active' : 'list-item'}
                onClick={() =>
                  void navigate({
                    to: '/compose',
                    search: (prev) => ({
                      ...prev,
                      object: formatObjectRef({ kind: 'blueprint', id: bp.id }),
                    }),
                  })
                }
              >
                <span className="item-name">{bp.name}</span>
                <span className="item-meta">
                  v{bp.version} · {bp.team}
                </span>
              </button>
            </li>
          ))}
        </ul>
      </section>
      {claimDoc && claim ? (
        <Workspace key={claimDoc.id} doc={claimDoc} me={me.data} claim={claim} />
      ) : (
        chosen && <Workspace key={chosen.id} doc={chosen} me={me.data} />
      )}
    </div>
  )
}

const SURFACES: { surface: ComposeSurface; label: string; testid: string }[] = [
  { surface: 'composer', label: 'Composer', testid: 'view-composer' },
  { surface: 'requirements', label: 'Requirement-first', testid: 'view-requirements' },
  { surface: 'canvas', label: 'Node canvas', testid: 'view-canvas' },
]

function Workspace({
  doc,
  me,
  claim,
}: {
  doc: BlueprintDoc
  me: Me | undefined
  claim?: ClaimContext
}) {
  const [draft, setDraft] = useState(doc)
  const lens = useLens()
  const search = useSearch({ strict: false })
  const navigate = useNavigate()

  // The one evaluator call, re-run on every interaction and lens change
  // (ADR-0022 §2); the previous verdict holds while the next one loads so
  // the surfaces never flicker empty.
  const verdict = useQuery({
    queryKey: ['validate', draft, lens],
    queryFn: () => api.validate(draft, lens),
    placeholderData: keepPreviousData,
  })

  const dirty = JSON.stringify(draft) !== JSON.stringify(doc)
  // A content change lands with its version bump in the same PR
  // (ADR-0024 §7): the proposal carries the next monotonic version.
  const proposed: BlueprintDoc = dirty ? { ...draft, version: doc.version + 1 } : draft
  const proposal = useMutation({ mutationFn: () => api.propose(proposed, lens, claim) })

  const editable = me !== undefined && canActOn(me, doc.team)
  const surface: ComposeSurface = search.surface ?? 'composer'
  const yamlOpen = search.yaml === true
  const closeYaml = () =>
    void navigate({ to: '/compose', search: (prev) => ({ ...prev, yaml: undefined }) })

  const onAdd = (entry: PaletteEntry, signals: string[]) =>
    setDraft((d) => addEntry(d, entry, signals))
  const onRemove = (signal: string, index: number) =>
    setDraft((d) => removeEntry(d, signal, index))
  const onSuggest = (row: RequirementVerdict) =>
    setDraft((d) => addSuggestion(d, row, verdict.data?.palette.entries ?? []))

  const blocked = verdict.data?.save.blocked ?? false

  return (
    <section className="compose-detail" data-testid="compose-detail">
      <header className="compose-header">
        <h2>
          {doc.name}{' '}
          <span className="item-meta">
            v{doc.version}
            {dirty ? ' · edited' : ''}
          </span>
        </h2>
        <nav className="view-switcher" aria-label="Compose surfaces">
          {SURFACES.map(({ surface: s, label, testid }) => (
            <Link
              key={s}
              to="/compose"
              search={(prev) => ({ ...prev, surface: s })}
              className={s === surface ? 'scope-link active' : 'scope-link'}
              data-testid={testid}
            >
              {label}
            </Link>
          ))}
        </nav>
        <Button
          className={yamlOpen ? 'yaml-toggle selected' : 'yaml-toggle'}
          data-testid="yaml-toggle"
          aria-expanded={yamlOpen}
          onClick={() =>
            void navigate({
              to: '/compose',
              search: (prev) => ({ ...prev, yaml: yamlOpen ? undefined : true }),
            })
          }
        >
          YAML
        </Button>
      </header>

      {/* The claim flow's draft path (ADR-0042 §6): the selector arrives
          pre-filled from the herd's shared identity attributes, never a
          list of instance ids, and the proposal authors the Tier binding
          beside this Blueprint. */}
      {claim && (
        <div className="claim-banner" data-testid="claim-banner">
          <p>
            Claiming into new Tier <span className="mono">{claim.tier}</span> (
            {claim.environment}, owned by {claim.team}) with selector{' '}
            <code data-testid="claim-banner-selector">{formatSelector(claim.selector)}</code>.
            The selector is built from the identity attributes the collectors share. Save
            proposes the Tier binding and this Blueprint as one pull request.
          </p>
        </div>
      )}

      {/* Ownership decides the affordance (ADR-0019 §2): the surface says which it is. */}
      {me &&
        (editable ? (
          <p className="authoring editable" data-testid="compose-authoring">
            You can edit this Blueprint: {doc.team} is one of your teams.
          </p>
        ) : (
          <p className="authoring readonly" data-testid="compose-authoring">
            Read-only: {doc.team} owns this Blueprint. Ask its owners for changes.
          </p>
        ))}

      {editable && (
        <div className="save-area">
          {blocked && (
            /* Only an Allow-list violation blocks a save; every other
               finding is advisory (ADR-0022 §3, ADR-0028 §3). The notice
               states the reasons and offers the Grant door. */
            <div className="save-blocked" data-testid="save-blocked">
              <p>
                <strong>Save is disabled</strong>: {verdict.data?.save.reasons.join('; ')}.
              </p>
              <Link
                to="/catalogue"
                search={(prev) => ({ lens: prev.lens })}
                className={buttonClass('secondary', 'who-acts')}
                data-testid="request-grant"
              >
                Request a Grant in Governance
              </Link>
            </div>
          )}
          <Button
            tone="primary"
            data-testid="save-button"
            disabled={blocked || proposal.isPending || verdict.data === undefined}
            onClick={() => proposal.mutate()}
          >
            Save: propose v{proposed.version} as a pull request
          </Button>
          {proposal.isError && (
            <p className="save-error" data-testid="save-error">
              {proposal.error.message}
            </p>
          )}
          {proposal.data && (
            <div className="proposal" data-testid="proposal">
              <p>
                Change proposal{' '}
                <a href={proposal.data.url} data-testid="proposal-url">
                  {proposal.data.id}
                </a>{' '}
                opened on branch <code data-testid="proposal-branch">{proposal.data.branch}</code>.
                The pull request carries the rendered configuration, and its review decides.
              </p>
              <p className="item-meta" data-testid="proposal-attribution">
                Attributed to {proposal.data.attributedTo}.
              </p>
            </div>
          )}
        </div>
      )}

      <div className="compose-work">
        {/* Click-off closes the resident flyout without swallowing the click. */}
        <div
          className="compose-surface"
          onClickCapture={yamlOpen ? closeYaml : undefined}
        >
          {surface === 'composer' && (
            <Composer
              draft={draft}
              verdict={verdict.data}
              offendingLane={search.lane}
              editable={editable}
              onAdd={onAdd}
              onRemove={onRemove}
            />
          )}
          {surface === 'requirements' && (
            <Requirements verdict={verdict.data} editable={editable} onSuggest={onSuggest} />
          )}
          {surface === 'canvas' && (
            <NodeCanvas
              draft={draft}
              verdict={verdict.data}
              editable={editable}
              onAdd={onAdd}
              onRemove={onRemove}
            />
          )}
        </div>
        {yamlOpen && <YamlFlyout yaml={verdict.data?.yaml} onClose={closeYaml} />}
      </div>
    </section>
  )
}
