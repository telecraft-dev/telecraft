// The claim flow's server side (ADR-0042 §6): preview and exit for the
// ungoverned-to-governed path behind POST /api/v1/claims/preview and
// POST /api/v1/claims, plus the claim-context validation the draft path
// rides into POST /api/v1/proposals. The platform binary submits through
// the forge adapter (internal/forge Submit), user-attributed (ADR-0014);
// the fixture stands in for that seam exactly as the governance proposal
// handler does: validate fail-closed with the problems named (422), or
// answer with the opened proposal.
//
// A selector is authored attribute pairs matched by string equality
// (ADR-0007, internal/serving): every pair must equal the reported
// attribute. The one rule this module owns beyond well-formedness is
// generalise-never-enumerate: a selector key that names one instance is
// refused however it arrives, so the UI's restraint is enforced, not
// assumed.

/** Attribute keys that name one instance, never a population. Mirrored
 * in src/estate/claim.ts, where the suggestion already drops them. */
export const INSTANCE_KEYS = new Set([
  'service.instance.id',
  'host.name',
  'host.id',
  'k8s.pod.name',
  'k8s.pod.uid',
])

/** String-equality selector match (ADR-0007): every pair must equal. */
function matches(selector, attributes) {
  return Object.entries(selector).every(([key, value]) => attributes?.[key] === value)
}

/** The pairs two selectors agree on: attach's widened selector. */
function sharedPairs(a, b) {
  const out = {}
  for (const [key, value] of Object.entries(a)) {
    if (b[key] === value) out[key] = value
  }
  return out
}

/** Whether a Tier's authored selector contradicts the claim's: some key
 * both carry, with different values. No contradiction means the claim may
 * reach that Tier's population: blast radius, reported, never hidden. */
function contradicts(tierSelector, selector) {
  return Object.entries(selector).some(
    ([key, value]) => tierSelector[key] !== undefined && tierSelector[key] !== value,
  )
}

function teamIds(node, out = new Set()) {
  out.add(node.id)
  for (const child of node.teams ?? []) teamIds(child, out)
  return out
}

/** Ungoverned collectors. The fixture's rows without a Tier are the whole
 * ungoverned population, so arithmetic over them is exact (ADR-0031). */
function ungovernedRows(estate) {
  return estate.collectors.filter((row) => row.tier === undefined)
}

/** The band summary /api/v1/estate carries (ADR-0031 §2). */
export function ungovernedSummary(estate) {
  const rows = ungovernedRows(estate)
  return {
    served: rows.filter((row) => row.ungoverned === 'served').length,
    foreign: rows.filter((row) => row.ungoverned === 'foreign').length,
  }
}

/** Selector well-formedness plus the one hard rule: never enumerate. */
function selectorProblems(selector) {
  const problems = []
  if (selector === undefined || typeof selector !== 'object' || Array.isArray(selector)) {
    return ['the claim carries no selector: a Tier binding matches collectors by the identity attributes they share']
  }
  const entries = Object.entries(selector)
  if (entries.length === 0) {
    problems.push(
      'the selector is empty: keep at least one shared identity attribute',
    )
  }
  for (const [key, value] of entries) {
    if (INSTANCE_KEYS.has(key)) {
      problems.push(
        `selector key "${key}" names one collector, not a population: a selector matches on shared identity attributes and never enumerates instance ids`,
      )
    }
    if (typeof value !== 'string' || value === '') {
      problems.push(`selector key "${key}" carries no value: a selector pair needs a string to match`)
    }
  }
  return problems
}

/** Validates the claim body shared by preview, exit, and the Compose
 * draft path; `mode`/`tier` join once the one question is answered. */
function claimProblems(estate, body) {
  const problems = selectorProblems(body.selector)
  if (body.team !== undefined && !teamIds(estate.teams).has(body.team)) {
    problems.push(`owning team "${body.team}" is not in the team tree`)
  }
  if (body.environment !== undefined && !estate.environments.includes(body.environment)) {
    problems.push(`environment "${body.environment}" is not declared on this estate`)
  }
  if (body.mode === 'attach') {
    const authored = estate.selectors[body.tier]
    if (authored === undefined) {
      problems.push(`tier "${body.tier ?? ''}" carries no authored selector to widen`)
    } else if (Object.keys(sharedPairs(authored, body.selector ?? {})).length === 0) {
      problems.push(
        `tier "${body.tier}" shares no selector pair with the claim, so there is nothing to widen. Draft a new Tier instead`,
      )
    }
  }
  if (body.mode === 'draft') {
    if (typeof body.tier !== 'string' || !body.tier.includes('/') || body.tier.endsWith('/')) {
      problems.push('a drafted Tier needs a team-qualified id, like data-flow/payments-edge')
    } else if (estate.selectors[body.tier] !== undefined || estate.cards.some((c) => c.tier === body.tier)) {
      problems.push(`tier "${body.tier}" already exists. Attach to it instead`)
    }
  }
  return problems
}

/** The claim context a Compose proposal carries on the draft path: the
 * same rulebook, so /api/v1/proposals refuses what /api/v1/claims would. */
export function claimContextProblems(estate, claim) {
  return claimProblems(estate, { ...claim, mode: 'draft' })
}

function renderSelector(selector) {
  return Object.keys(selector)
    .sort()
    .map((key) => `  ${key}: ${selector[key]}`)
    .join('\n')
}

/** The Tier binding as the PR would carry it: the rendered impact
 * preview the proposal rides (ADR-0042 §6, ADR-0028). */
function renderBinding(estate, body) {
  if (body.mode === 'draft' && typeof body.tier === 'string' && body.tier.includes('/')) {
    const name = body.tier.split('/').pop()
    return [
      `# teams/${body.team}/tiers/${name}.yaml: authored by the claim flow`,
      `owner: ${body.team}`,
      `environment: ${body.environment}`,
      // No blueprint line: the Blueprint the drafted Tier binds is chosen
      // in the flow this preview hands off to, and naming one here would
      // be naming an object nobody has authored.
      'selector:',
      renderSelector(body.selector),
      '',
    ].join('\n')
  }
  if (body.mode === 'attach' && estate.selectors[body.tier] !== undefined) {
    const widened = sharedPairs(estate.selectors[body.tier], body.selector)
    const card = estate.cards.find((c) => c.tier === body.tier)
    const blueprint = estate.blueprints.find((bp) => bp.tier === body.tier)
    const name = body.tier.split('/').pop()
    return [
      `# tiers/${name}.yaml: selector widened by the claim`,
      `environment: ${card?.environment ?? body.environment}`,
      ...(blueprint ? [`blueprint: ${blueprint.id}@${blueprint.version}`] : []),
      'selector:',
      renderSelector(widened),
      '',
    ].join('\n')
  }
  return undefined
}

/**
 * POST /api/v1/claims/preview: the impact of the constrained selector:
 * matched ungoverned collectors by how they are read, governed populations
 * the selector does not contradict, attach candidates ranked by selector
 * proximity, and the rendered Tier binding once a path is chosen. For
 * attach the judged selector is the widened one: what merge would serve.
 */
export function previewClaim(estate, body) {
  const selector = body.selector ?? {}
  const effective =
    body.mode === 'attach' && estate.selectors[body.tier] !== undefined
      ? sharedPairs(estate.selectors[body.tier], selector)
      : selector

  const hit = ungovernedRows(estate).filter((row) => matches(effective, row.attributes))
  const matched = {
    total: hit.length,
    served: hit.filter((row) => row.ungoverned === 'served').length,
    foreign: hit.filter((row) => row.ungoverned === 'foreign').length,
  }

  const overlaps = Object.entries(estate.selectors)
    .filter(([tier]) => tier !== body.tier)
    .filter(([, authored]) => Object.keys(effective).length > 0 && !contradicts(authored, effective))
    .map(([tier]) => ({
      tier,
      matched: estate.cards.find((c) => c.tier === tier)?.population.matched ?? 0,
    }))

  const candidates = Object.entries(estate.selectors)
    .map(([tier, authored]) => {
      const widened = sharedPairs(authored, selector)
      const card = estate.cards.find((c) => c.tier === tier)
      return {
        tier,
        name: card?.name ?? tier,
        team: card?.team ?? '',
        environment: card?.environment ?? '',
        selector: authored,
        satisfied: Object.keys(widened).length,
        of: Object.keys(authored).length,
        widened,
      }
    })
    .filter((candidate) => candidate.satisfied > 0)
    .sort(
      (a, b) =>
        b.satisfied - a.satisfied ||
        b.satisfied / b.of - a.satisfied / a.of ||
        a.tier.localeCompare(b.tier),
    )

  const rendered = renderBinding(estate, body)
  return { matched, overlaps, candidates, ...(rendered ? { rendered } : {}) }
}

let claimCounter = 300

/**
 * POST /api/v1/claims, the attach exit (draft exits ride Compose's
 * proposal): fail closed with the problems named, or the opened proposal,
 * a PR authoring the Tier binding, user-attributed (ADR-0014).
 */
export function submitClaim(estate, body) {
  const problems = claimProblems(estate, body)
  if (body.mode !== 'attach' && body.mode !== 'draft') {
    problems.push('the claim names no path: attach to an existing Tier or draft a new one')
  }
  if (typeof body.title !== 'string' || body.title.trim() === '') {
    problems.push('the proposal carries no title')
  }
  if (problems.length > 0) {
    return { problems }
  }
  claimCounter += 1
  return {
    proposal: {
      id: `claim-${claimCounter}`,
      url: `https://forge.example/estate/pull/${claimCounter}`,
      branch: `claim/${body.tier}`,
      attributedTo: `${estate.me.name} <${estate.me.email}>`,
    },
  }
}
