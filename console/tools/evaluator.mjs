// The validation engine behind POST /api/v1/validate and /api/v1/proposals:
// one evaluator, exposed as an API on the instance (ADR-0022 §1). The
// composer calls it continuously (advisory) and the proposal exit calls the
// same rulebook with enforcement on. The console never carries a copy,
// because policy state (catalogue, Allow-lists, Grants, floors) lives with
// the instance. This module is the fixture stand-in for that instance
// evaluator (the Go evaluator in internal/allowlist and internal/blueprint
// is the judgement of record at render), judging with the same authored
// governance policy /api/v1/governance serves and the active catalogue's
// entries; the platform binary replaces it when the endpoint lands there
// (ADR-0045 §6).
//
// Verdicts implemented, each to its ADR:
// - Palette: Catalogue ∩ effective Allow-list, narrowing-only inheritance
//   with Grants unioned back in (ADR-0021, the same composition rules as
//   src/governance/effective.ts); allowed shown, floor-breaching greyed
//   with the reason, non-allowed hidden with an admitted count
//   (ADR-0022 §5).
// - Findings: reference, allow-list, floor (per component and signal
//   actually routed, ADR-0023 §4), lifecycle, and ordering. Ordering
//   wisdom is keyed on catalogue types, never a re-sort (ADR-0024 §6).
// - Exactly one rule hard-blocks: an allow-list violation (ADR-0022 §3).
//   Floors, lifecycle and ordering are findings with remediation, never
//   blocks.
// - Requirements: `satisfies` is a claim of intent; the evaluator judges
//   the fact separately and never blends the two (REQ-031, ADR-0026).
// - The rendered artefact preview for the YAML flyout (REQ-035), mirroring
//   the renderer's otelcol output shape: `type/name` for local Components,
//   `type/team.name` for shared ones (ADR-0024 §5).

const SIGNALS = ['traces', 'logs', 'metrics', 'profiles']

// The maturity ladder (ADR-0023); deprecated and unmaintained are
// lifecycle end-states judged by the lifecycle rule, never floor rungs.
const STABILITY_RANK = { development: 0, alpha: 1, beta: 2, stable: 3 }

// The shipped ordering wisdom (ADR-0024 §6), mirroring the platform's
// default rules: back-pressure first, batching last.
const ORDERING_RULES = [
  {
    class: 'processor',
    type: 'memory_limiter',
    slot: 'first',
    reason: 'back-pressure must engage before any other processor buffers or fans out',
  },
  {
    class: 'processor',
    type: 'batch',
    slot: 'last',
    reason: 'batching belongs after every shaping processor, so the exporter sees the final shape',
  },
]

/** The teams from the root down to `team`, inclusive (ADR-0021 §2). */
function chain(teams, team) {
  const walk = (node, trail) => {
    const here = [...trail, node.id]
    if (node.id === team) return here
    for (const child of node.teams ?? []) {
      const found = walk(child, here)
      if (found) return found
    }
    return undefined
  }
  return walk(teams, []) ?? []
}

// The allow-list pattern vocabulary is literals plus * and ? only,
// mirroring internal/allowlist parseEntry (and src/governance/effective.ts).
function globToRegExp(pattern) {
  const escaped = pattern
    .replace(/[.+^${}()|[\]\\]/g, '\\$&')
    .replace(/\*/g, '.*')
    .replace(/\?/g, '.')
  return new RegExp(`^${escaped}$`)
}

/**
 * Whether one authored `class/type-pattern` entry selects the catalogue
 * entry: the class side exact, the pattern tried against the canonical
 * type and the `deprecated_type` alias (ADR-0020 §3).
 */
function entrySelects(pattern, entry) {
  const cut = pattern.indexOf('/')
  if (cut <= 0) return false
  if (pattern.slice(0, cut) !== entry.class) return false
  const type = globToRegExp(pattern.slice(cut + 1))
  if (type.test(entry.type)) return true
  return entry.deprecatedType !== undefined && type.test(entry.deprecatedType)
}

/**
 * The effective Allow-list decision for one catalogue entry: walking the
 * chain root→team, each declared list intersects, then each Grant targeting
 * that team unions back in, so a Grant widens from its target's subtree
 * downward and a descendant's list narrows it back out (ADR-0021 §2 to §3).
 * A `(class, type)` the Catalogue does not know is not allowed.
 */
function judgeMembership(estate, team, catEntry) {
  if (!catEntry) return { allowed: false }
  const teamChain = chain(estate.teams, team)
  const lists = new Map(estate.allowLists.map((list) => [list.team, list]))
  const ownerTeams = new Map(estate.owners.map((owner) => [owner.id, owner.team]))
  const anyList = teamChain.some((t) => lists.has(t))
  // Grants apply in id order: the id is the audit chain's name for them.
  const grants = [...estate.grants].sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))

  let allowed = true
  let via
  for (const t of teamChain) {
    const list = lists.get(t)
    if (list && !list.allow.some((pattern) => entrySelects(pattern, catEntry))) {
      allowed = false
      via = undefined
    }
    if (!allowed) {
      for (const grant of grants) {
        if (grant.team === t && grant.adds.some((pattern) => entrySelects(pattern, catEntry))) {
          allowed = true
          via = grant
          break
        }
      }
    }
  }
  if (!allowed) return { allowed: false }
  return {
    allowed: true,
    origin: via ? 'grant' : anyList ? 'allow-list' : 'default-allow',
    grant: via
      ? { id: via.id, grantedBy: ownerTeams.get(via.owner) ?? via.owner, grantedTo: via.team }
      : undefined,
  }
}

/** The active catalogue's entry for a `(class, type)`, alias-resolving (ADR-0020 §3). */
function typeEntry(entries, cls, type) {
  return entries.find((e) => e.class === cls && (e.type === type || e.deprecatedType === type))
}

/** The floor stability rank for the evaluation context, or undefined. */
function floorFor(estate, serviceClass, environment) {
  const level = estate.floors?.[environment]?.[serviceClass]
  return level === undefined ? undefined : { level, rank: STABILITY_RANK[level] }
}

/**
 * Resolves one lane reference (ADR-0024 §4): a bare name is a local
 * Component of the draft; `team/name@pin` is a shared Component. Returns
 * undefined when nothing provides the reference.
 */
function resolve(estate, draft, ref) {
  if (!ref.includes('/')) {
    const local = draft.locals[ref]
    if (!local) return undefined
    return { ref, label: ref, class: local.class, type: local.type, renderedId: `${local.type}/${ref}` }
  }
  const [id] = ref.split('@')
  const shared = estate.catalogue.find((c) => c.id === id)
  if (!shared) return undefined
  return {
    ref,
    label: id,
    class: shared.class,
    type: shared.type,
    renderedId: `${shared.type}/${shared.team}.${shared.name}`,
  }
}

/** The evaluation context: the owning team plus the bound Tier's Service Class. */
function contextOf(estate, draft) {
  const card = estate.cards.find((c) => c.tier === draft.tier)
  return { team: draft.team, serviceClass: card?.serviceClass }
}

/** The signal-lane deprecation notices of a catalogue entry, if any. */
function deprecationOf(catEntry) {
  if (!catEntry?.deprecation) return undefined
  const signals = Object.keys(catEntry.deprecation)
  if (signals.length === 0) return undefined
  return { migration: catEntry.deprecation[signals[0]].migration }
}

/**
 * The palette (ADR-0022 §5): every Catalogue entry and shared Component the
 * team may use, judged for the evaluation context. Non-allowed entries are
 * hidden and counted; floor-breaching entries are greyed with the reason;
 * the palette enforces nothing.
 */
function palette(estate, entries, draft, environment) {
  const { team, serviceClass } = contextOf(estate, draft)
  const floor = serviceClass ? floorFor(estate, serviceClass, environment) : undefined
  const out = []
  let hidden = 0

  // What an add gesture inserts: a shared entry inserts a pinned reference,
  // a type entry a fresh local Component (ADR-0024: palette adds create a
  // local by default); click-add targets every supported signal.
  const judge = (cls, type, base, addRef) => {
    const catType = typeEntry(entries, cls, type)
    const membership = judgeMembership(estate, team, catType)
    if (!membership.allowed) {
      hidden += 1
      return
    }
    const stability = catType?.stability ?? {}
    const signals = SIGNALS.filter((s) => stability[s] !== undefined)
    let state = 'allowed'
    let reason
    if (floor) {
      const breaching = signals.filter(
        (s) => stability[s] in STABILITY_RANK && STABILITY_RANK[stability[s]] < floor.rank,
      )
      if (breaching.length > 0) {
        state = 'greyed'
        reason = `${stability[breaching[0]]} on ${breaching.join(', ')}: below this Service's ${serviceClass} floor in ${environment} (${floor.level})`
      }
    }
    const deprecated = deprecationOf(catType)
    out.push({
      ...base,
      class: cls,
      type,
      signals,
      stability,
      add: { ...(addRef ? { ref: addRef } : {}), signals },
      state,
      ...(reason ? { reason } : {}),
      origin: membership.origin,
      ...(membership.grant ? { grant: membership.grant } : {}),
      ...(deprecated ? { deprecated } : {}),
    })
  }

  for (const t of entries) {
    judge(t.class, t.type, {
      key: `type:${t.class}/${t.type}`,
      label: t.type,
      residence: 'type',
    })
  }
  for (const c of estate.catalogue) {
    judge(
      c.class,
      c.type,
      { key: `shared:${c.id}`, label: c.id, residence: 'shared' },
      `${c.id}@${c.version}`,
    )
  }
  return { entries: out, hidden }
}

/** All findings the engine raises for the draft in this context (advisory). */
function findings(estate, entries, draft, environment) {
  const { serviceClass } = contextOf(estate, draft)
  const floor = serviceClass ? floorFor(estate, serviceClass, environment) : undefined
  const out = []

  for (const signal of SIGNALS) {
    const lane = draft.lanes[signal]
    if (!lane) continue
    for (const [i, ref] of lane.entries()) {
      const c = resolve(estate, draft, ref)
      if (!c) {
        out.push({
          id: `reference-${signal}-${i}`,
          kind: 'reference',
          severity: 'violation',
          lane: signal,
          ref,
          summary: `${ref} resolves to nothing: no local or shared Component provides it`,
          remediation: 'Fix the reference, or restore the Component it names.',
        })
        continue
      }
      const key = `${c.class}/${c.type}`
      const catType = typeEntry(entries, c.class, c.type)
      const membership = judgeMembership(estate, draft.team, catType)
      if (!membership.allowed) {
        out.push({
          id: `allowlist-${signal}-${i}`,
          kind: 'allow-list',
          severity: 'violation',
          lane: signal,
          ref,
          summary: `${ref} (${key}) is outside this team's effective Allow-list`,
          remediation:
            'Request a Grant from the parent team that owns the wider list, or remove the Component. An allow-list violation is the only finding that blocks the render.',
        })
      }
      const level = catType?.stability?.[signal]
      if (catType && level === undefined) {
        out.push({
          id: `reference-${signal}-${i}`,
          kind: 'reference',
          severity: 'advisory',
          lane: signal,
          ref,
          summary: `${ref} (${key}) declares no ${signal} support`,
          remediation: `Route ${signal} through a component that declares it, or remove the entry from this lane.`,
        })
      }
      // Floors judge each (component, signal) the lane actually routes
      // (ADR-0023 §4): a finding, never a block (§5). Lifecycle end-states
      // are the lifecycle rule's, not a floor rung (ADR-0023 §6).
      if (floor && level in STABILITY_RANK && STABILITY_RANK[level] < floor.rank) {
        out.push({
          id: `floor-${signal}-${i}`,
          kind: 'floor',
          severity: 'violation',
          lane: signal,
          ref,
          summary: `${ref} is ${level} on ${signal}: below the ${serviceClass} floor in ${environment} (${floor.level})`,
          remediation: `Use a component at ${floor.level} or better on ${signal}, or take an Exemption.`,
        })
      }
      const notice = catType?.deprecation?.[signal]
      if (notice) {
        out.push({
          id: `lifecycle-${signal}-${i}`,
          kind: 'lifecycle',
          severity: 'advisory',
          lane: signal,
          ref,
          summary: `${ref} routes ${signal} through the deprecated ${key}`,
          remediation: notice.migration,
        })
      }
    }

    // Ordering wisdom judges same-class entries in authored order and only
    // raises findings; the renderer never re-sorts a lane (ADR-0024 §6).
    for (const rule of ORDERING_RULES) {
      const classed = lane
        .map((ref) => ({ ref, c: resolve(estate, draft, ref) }))
        .filter((e) => e.c?.class === rule.class)
      for (const [pos, e] of classed.entries()) {
        if (e.c.type !== rule.type) continue
        if ((rule.slot === 'first' && pos !== 0) || (rule.slot === 'last' && pos !== classed.length - 1)) {
          out.push({
            id: `ordering-${signal}-${e.ref}`,
            kind: 'ordering',
            severity: 'advisory',
            lane: signal,
            ref: e.ref,
            summary: `orders ${e.ref} at ${rule.class} position ${pos + 1} of ${classed.length}, but ${rule.type} belongs ${rule.slot}`,
            remediation: `Reorder the ${signal} lane: ${rule.reason}.`,
          })
        }
      }
    }
  }
  return out
}

/**
 * Requirement verdicts for surface B: what the Blueprint owes, whether the
 * draft claims it (`satisfies`, the intent), and whether the engine judges it
 * met (fact). The two never blend (REQ-031); claims are judged against the
 * requirement's current version whatever version they stamp (ADR-0026 §5).
 */
function requirements(estate, draft) {
  const out = []
  for (const req of estate.requirements) {
    if (!req.appliesTo.includes(draft.id)) continue
    const claim = draft.satisfies
      .map((c) => {
        const at = c.lastIndexOf('@')
        return { id: at > 0 ? c.slice(0, at) : c, version: at > 0 ? Number(c.slice(at + 1)) : undefined }
      })
      .find((c) => c.id === req.id)
    const met = req.verifiedBy.signals.every((signal) => {
      const lane = draft.lanes[signal] ?? []
      return lane.some((ref) => {
        const c = resolve(estate, draft, ref)
        if (!c) return false
        if (req.verifiedBy.ref !== undefined) return ref.split('@')[0] === req.verifiedBy.ref
        return `${c.class}/${c.type}` === req.verifiedBy.type
      })
    })
    out.push({
      id: req.id,
      version: req.version,
      summary: req.summary,
      remediation: req.remediation,
      claimed: claim !== undefined,
      ...(claim?.version !== undefined ? { claimedVersion: claim.version } : {}),
      met,
      suggestion: { ...req.verifiedBy },
    })
  }
  return out
}

/**
 * The rendered-artefact preview for the YAML flyout (REQ-035): the draft
 * compiled to otelcol shape with provenance-carrying ids (ADR-0024 §5).
 * Advisory: the authoritative render is the one the PR carries (ADR-0028).
 */
function renderYAML(estate, draft, environment) {
  const sections = { receiver: new Map(), processor: new Map(), exporter: new Map() }
  const lanes = SIGNALS.filter((s) => draft.lanes[s])
  for (const signal of lanes) {
    for (const ref of draft.lanes[signal]) {
      const c = resolve(estate, draft, ref)
      if (c && sections[c.class]) sections[c.class].set(c.renderedId, true)
    }
  }
  const lines = [
    '# Rendered preview: the validation API compiles the open Blueprint.',
    '# Read-only here. To edit by hand, change the file in git. The authoritative',
    '# render lands in the change proposal.',
    `# Tier ${draft.tier ?? '(unbound)'} (${environment}), Blueprint ${draft.id}@${draft.version}: draft, unstamped.`,
  ]
  const section = (title, ids) => {
    if (ids.size === 0) return
    lines.push(`${title}:`)
    for (const id of [...ids.keys()].sort()) lines.push(`  ${id}: {}`)
  }
  section('receivers', sections.receiver)
  section('processors', sections.processor)
  section('exporters', sections.exporter)
  lines.push('service:')
  if (draft.extensions.length > 0) {
    lines.push('  extensions:')
    for (const ref of draft.extensions) {
      const c = resolve(estate, draft, ref)
      lines.push(`    - ${c ? c.renderedId : ref}`)
    }
  }
  lines.push('  pipelines:')
  for (const signal of lanes) {
    lines.push(`    ${signal}:`)
    for (const [plural, cls] of [
      ['receivers', 'receiver'],
      ['processors', 'processor'],
      ['exporters', 'exporter'],
    ]) {
      const ids = draft.lanes[signal]
        .map((ref) => resolve(estate, draft, ref))
        .filter((c) => c?.class === cls)
        .map((c) => c.renderedId)
      if (ids.length === 0) continue
      lines.push(`      ${plural}:`)
      for (const id of ids) lines.push(`        - ${id}`)
    }
  }
  return lines.join('\n') + '\n'
}

/**
 * The one evaluator call (ADR-0022 §2): draft Blueprint plus context in,
 * verdicts out: findings, palette states, requirement verdicts, the save
 * gate, and the rendered preview. Stateless; the composer calls it on every
 * interaction and the proposal exit calls it with enforcement on.
 * `entries` is the active catalogue's entry list.
 */
export function validate(estate, entries, draft, environment) {
  const found = findings(estate, entries, draft, environment)
  const blocking = found.filter((f) => f.kind === 'allow-list')
  const { team, serviceClass } = contextOf(estate, draft)
  const floor = serviceClass ? floorFor(estate, serviceClass, environment) : undefined
  return {
    context: {
      team,
      environment,
      ...(serviceClass ? { serviceClass } : {}),
      ...(floor ? { floor: floor.level } : {}),
    },
    findings: found,
    palette: palette(estate, entries, draft, environment),
    requirements: requirements(estate, draft),
    save: {
      blocked: blocking.length > 0,
      reasons: blocking.map((f) => f.summary),
    },
    yaml: renderYAML(estate, draft, environment),
  }
}

let proposalCounter = 200

/**
 * The composer exit (ADR-0043 §6): the draft becomes a change proposal
 * through the forge adapter, user-attributed, render-in-PR (ADR-0028):
 * the console proposes, the PR decides. Enforcement is on: an allow-list
 * violation refuses the proposal, fail closed (ADR-0022 §3, ADR-0028 §3).
 * A claim context (the claim flow's draft-new-Tier path, ADR-0042 §6),
 * already judged by the claim rulebook at the endpoint, makes this the
 * PR authoring the Tier binding beside the Blueprint, named accordingly.
 */
export function propose(estate, entries, draft, environment, claim) {
  const verdict = validate(estate, entries, draft, environment)
  if (verdict.save.blocked) {
    const failure = new Error(
      `render refused, no proposal: ${verdict.save.reasons.join('; ')}. Request a Grant.`,
    )
    failure.status = 409
    throw failure
  }
  proposalCounter += 1
  return {
    id: `PR-${proposalCounter}`,
    url: `https://forge.example/estate/pull/${proposalCounter}`,
    branch: claim !== undefined ? `claim/${claim.tier}` : `compose/${draft.id}`,
    attributedTo: `${estate.me.name} <${estate.me.email}>`,
  }
}
