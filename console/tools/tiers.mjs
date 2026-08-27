// The Tier-first onboarding flow's server side (ADR-0060): validation for
// POST /api/v1/tiers/proposals and the setup guidance behind
// GET /api/v1/setup. The platform binary submits the proposal through the
// forge adapter (internal/forge Submit), user-attributed (ADR-0014); the
// fixture stands in for that seam exactly as the governance and claim
// handlers do: validate fail-closed with the problems named (422), or
// answer with the opened proposal.
//
// The guidance is documentation, never an artefact (ADR-0060 §4): it is
// generated on view from the Tier, the activated Catalogue version, and
// the estate settings, and it is never committed, rendered, or judged.

/** The Tier name segment's vocabulary: the id becomes `<team>/<name>`. */
const NAME_PATTERN = /^[a-z0-9-]+$/

function teamIds(node, out = new Set()) {
  out.add(node.id)
  for (const child of node.teams ?? []) teamIds(child, out)
  return out
}

/** Selector well-formedness: a non-empty object of non-empty string pairs. */
function selectorProblems(selector) {
  if (selector === undefined || typeof selector !== 'object' || Array.isArray(selector)) {
    return ['the proposal carries no selector: a Tier matches collectors by the identity attributes they share']
  }
  const entries = Object.entries(selector)
  if (entries.length === 0) {
    return ['the selector is empty: keep at least one identity attribute']
  }
  const problems = []
  for (const [key, value] of entries) {
    if (typeof value !== 'string' || value === '') {
      problems.push(`the selector key ${key} carries no value: a selector pair needs a string to match`)
    }
  }
  return problems
}

/**
 * Validates a Tier proposal, mirroring how the platform's loader would
 * refuse the authored Tier (ADR-0060 §2): every problem named, in the
 * reader's words, fail closed.
 */
export function tierProblems(estate, request) {
  const problems = []
  const body = request ?? {}

  if (typeof body.title !== 'string' || body.title.trim() === '') {
    problems.push('the proposal carries no title')
  }

  if (typeof body.name !== 'string' || body.name === '') {
    problems.push('the Tier carries no name')
  } else if (!NAME_PATTERN.test(body.name)) {
    problems.push(`the name ${body.name} can use lower-case letters, digits and hyphens only`)
  }

  const teams = teamIds(estate.teams)
  if (typeof body.team !== 'string' || body.team === '') {
    problems.push('the Tier names no owning team')
  } else if (!teams.has(body.team)) {
    problems.push(`the team ${body.team} is not in the team tree`)
  }

  const owner = (estate.owners ?? []).find((o) => o.id === body.owner)
  if (typeof body.owner !== 'string' || body.owner === '') {
    problems.push('the Tier names no owner: every authored object needs one')
  } else if (owner === undefined) {
    problems.push(`the owner ${body.owner} is not on this estate`)
  } else if (typeof body.team === 'string' && teams.has(body.team) && owner.team !== body.team) {
    problems.push(`the owner ${body.owner} is not in the team ${body.team}`)
  }

  if (typeof body.environment !== 'string' || body.environment === '') {
    problems.push('the Tier names no environment')
  } else if (!estate.environments.includes(body.environment)) {
    problems.push(`the environment ${body.environment} is not declared on this estate`)
  }

  const blueprint = (estate.blueprints ?? []).find((bp) => bp.id === body.blueprint)
  if (typeof body.blueprint !== 'string' || body.blueprint === '') {
    problems.push('the Tier names no Blueprint')
  } else if (blueprint === undefined) {
    problems.push(`the Blueprint ${body.blueprint} is not on this estate`)
  } else if (blueprint.version !== body.blueprintVersion) {
    problems.push(
      `the Blueprint ${body.blueprint} is at version ${blueprint.version}, not version ${body.blueprintVersion}`,
    )
  }

  problems.push(...selectorProblems(body.selector))

  if (body.minExpected !== undefined) {
    if (!Number.isInteger(body.minExpected) || body.minExpected < 1) {
      problems.push('the minimum expected population must be a whole number of at least 1')
    }
  }

  if (typeof body.team === 'string' && typeof body.name === 'string' && body.name !== '') {
    const id = `${body.team}/${body.name}`
    if (estate.cards.some((card) => card.tier === id)) {
      problems.push(`the Tier ${id} already exists`)
    }
  }

  return problems
}

/**
 * GET /api/v1/setup: the named Tier's setup guidance, generated on view
 * from the Tier, the activated Catalogue version, and the estate settings
 * (ADR-0060 §4). An unknown Tier is undefined: say "cannot know", never
 * fabricate.
 */
export function setupGuidance(estate, activeVersion, tier) {
  const card = estate.cards.find((c) => c.tier === tier)
  if (card === undefined) return undefined
  const name = tier.split('/').pop()
  return {
    tier,
    environment: card.environment,
    artefactPath: `rendered/${card.team}/${name}.yaml`,
    opampEndpoint: estate.settings.opampEndpoint,
    selfTelemetryEndpoint: estate.settings.selfTelemetryEndpoint,
    identityAttributes: estate.selectors[tier] ?? {},
    collectorRelease: activeVersion,
  }
}
