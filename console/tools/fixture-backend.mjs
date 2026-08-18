// The fixture backend: a real HTTP server implementing the documented
// platform API (console/README.md) over the fixture estate in
// console/fixtures/estate.json. The console consumes only this contract
// (ADR-0045 §6); when the platform binary grows the API endpoint, this
// server is what it replaces. With --dist it also serves the built bundle,
// which is how the Playwright suite runs the console end to end.
//
// Usage: node tools/fixture-backend.mjs [--port 4700] [--dist dist]

import { createServer } from 'node:http'
import { readFile } from 'node:fs/promises'
import { extname, join, normalize } from 'node:path'
import { fileURLToPath } from 'node:url'

const args = process.argv.slice(2)
const argValue = (flag) => {
  const i = args.indexOf(flag)
  return i >= 0 ? args[i + 1] : undefined
}
const port = Number(argValue('--port') ?? process.env.PORT ?? 4700)
const dist = argValue('--dist')

const root = join(fileURLToPath(import.meta.url), '..', '..')
const estate = JSON.parse(await readFile(join(root, 'fixtures', 'estate.json'), 'utf8'))
const catalogues = JSON.parse(await readFile(join(root, 'fixtures', 'catalogues.json'), 'utf8'))

const activeCatalogue = () =>
  catalogues.versions.find((v) => v.version === catalogues.active)

// The jump-to-object index: every authored object, by kind, name-searchable.
function objects() {
  const out = []
  const walkTeams = (team) => {
    out.push({ kind: 'team', id: team.id, name: team.name, team: team.id })
    for (const child of team.teams ?? []) walkTeams(child)
  }
  walkTeams(estate.teams)
  for (const card of estate.cards) {
    out.push({
      kind: 'tier',
      id: card.tier,
      name: card.name,
      team: card.team,
      environment: card.environment,
    })
  }
  for (const service of estate.services) {
    out.push({ kind: 'service', id: service.id, name: service.name, team: service.team })
  }
  for (const blueprint of estate.blueprints) {
    out.push({ kind: 'blueprint', id: blueprint.id, name: blueprint.name, team: blueprint.team })
  }
  for (const component of estate.catalogue) {
    out.push({ kind: 'component', id: component.id, name: component.name, team: component.team })
  }
  // Catalogue entries of the active version: browsable and deep-linkable,
  // machine-generated, owned by nobody — the index carries no team.
  for (const entry of activeCatalogue().components) {
    out.push({
      kind: 'entry',
      id: `${entry.class}/${entry.type}`,
      name: entry.displayName ?? entry.type,
    })
  }
  return out
}

const api = {
  '/api/v1/me': () => estate.me,
  '/api/v1/objects': objects,
  '/api/v1/estate': () => ({
    environments: estate.environments,
    teams: estate.teams,
    cards: estate.cards,
  }),
  // The on-demand drawer (ADR-0041 §3): findings with who-acts routing and
  // why-provenance. A Tier without a seeded drawer answers empty, honestly.
  '/api/v1/drawer': (url) => {
    const tier = url.searchParams.get('tier') ?? ''
    return (
      estate.drawers[tier] ?? { contractVersion: 1, tier, findings: [], provenance: [] }
    )
  },
  // Per-collector detail, served flat: list surfaces are its only home
  // (ADR-0042 rule 3.4); the console filters client-side.
  '/api/v1/collectors': () => estate.collectors,
  '/api/v1/topology': () => ({
    environments: estate.environments,
    tiers: estate.cards.map((card) => ({
      id: card.tier,
      name: card.name,
      team: card.team,
      environment: card.environment,
    })),
    sources: estate.topology.sources,
    hops: estate.topology.hops,
    paths: estate.topology.paths,
  }),
  '/api/v1/blueprints': () => estate.blueprints,
  '/api/v1/catalogue': () => estate.catalogue,
  // Installed catalogues are retained per version, one designated active
  // (ADR-0020 §9); browsing consults the version asked for.
  '/api/v1/catalogue/versions': () => ({
    active: catalogues.active,
    versions: catalogues.versions.map((v) => ({
      version: v.version,
      active: v.version === catalogues.active,
      components: v.components.length,
      source: v.source,
    })),
  }),
  '/api/v1/catalogue/entries': (url) => {
    const version = url.searchParams.get('version') ?? catalogues.active
    return catalogues.versions.find((v) => v.version === version)?.components
  },
  // The authored, git-resident Allow-list policy (ADR-0021 §5): the console
  // derives effective palettes from this plus the active catalogue.
  '/api/v1/governance': () => ({
    owners: estate.owners,
    allowLists: estate.allowLists,
    grants: estate.grants,
  }),
}

// ---- Governance proposals: the PR exit (ADR-0042 §6) --------------------
//
// The platform binary submits through the forge adapter (internal/forge
// Submit); the fixture stands in for that seam: it validates the edited
// policy fail-closed exactly as internal/allowlist loading does, refuses
// with the problems named (422), and otherwise answers with the opened
// proposal — opaque id, URL, branch — as the forge seam would.

const PIPELINE_CLASSES = new Set(['receiver', 'processor', 'exporter', 'connector', 'extension'])

// The allow-list pattern vocabulary: literals plus * and ? only.
function globMatch(pattern, s) {
  const rx = new RegExp(
    `^${pattern.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*').replace(/\?/g, '.')}$`,
  )
  return rx.test(s)
}

function entrySelects(entry, component) {
  return (
    globMatch(entry.pattern, component.type) ||
    (component.deprecatedType !== undefined && globMatch(entry.pattern, component.deprecatedType))
  )
}

// Validates one entry list, mirroring internal/allowlist parseEntries: an
// entry must parse and must select something in the active catalogue.
function entryProblems(ctx, raw, cat) {
  const problems = []
  const seen = new Set()
  for (const s of raw) {
    if (seen.has(s)) {
      problems.push(`${ctx}: entry "${s}" appears twice`)
      continue
    }
    seen.add(s)
    const cut = s.indexOf('/')
    const cls = cut > 0 ? s.slice(0, cut) : ''
    const pattern = cut > 0 ? s.slice(cut + 1) : ''
    if (cls === '' || pattern === '') {
      problems.push(`${ctx}: entry "${s}" is not class/type-pattern — like receiver/otlp, exporter/kafka* or processor/*`)
      continue
    }
    if (!PIPELINE_CLASSES.has(cls)) {
      problems.push(`${ctx}: entry "${s}": "${cls}" is not a pipeline class — one of receiver, processor, exporter, connector, extension`)
      continue
    }
    if (/[[\]\\/]/.test(pattern)) {
      problems.push(`${ctx}: entry "${s}": a type pattern is literal characters plus * and ? only`)
      continue
    }
    const selects = cat.components.some(
      (c) => c.class === cls && entrySelects({ pattern }, c),
    )
    if (!selects) {
      problems.push(`${ctx}: entry "${s}" selects nothing in catalogue ${cat.version} — unknown component types fail load (REQ-011)`)
    }
  }
  return problems
}

function teamsById(node, parent, out = new Map()) {
  out.set(node.id, { parent })
  for (const child of node.teams ?? []) teamsById(child, node.id, out)
  return out
}

function properAncestor(teams, a, b) {
  for (let id = teams.get(b)?.parent; id; id = teams.get(id)?.parent) {
    if (id === a) return true
  }
  return false
}

function partyProblems(ctx, team, owner, teams, owners) {
  const problems = []
  if (!team) problems.push(`${ctx} names no team`)
  else if (!teams.has(team)) problems.push(`${ctx} names team "${team}", which is not in the team tree`)
  if (!owner) problems.push(`${ctx} has no owner — every authored object carries one (ADR-0016)`)
  else if (!owners.has(owner)) problems.push(`${ctx} names owner "${owner}", which is not in the team tree`)
  return problems
}

// Mirrors internal/allowlist Load: fail closed, every problem named.
function governanceProblems(request) {
  const problems = []
  const cat = activeCatalogue()
  const teams = teamsById(estate.teams, undefined)
  const owners = new Map(estate.owners.map((o) => [o.id, o]))

  if (typeof request.title !== 'string' || request.title.trim() === '') {
    problems.push('the proposal carries no title')
  }
  const seenTeams = new Set()
  for (const list of request.allowLists ?? []) {
    const ctx = `allow-list for team "${list.team}"`
    problems.push(...partyProblems(ctx, list.team, list.owner, teams, owners))
    if (!list.allow || list.allow.length === 0) {
      problems.push(`${ctx} declares no entries — to inherit the parent's effective list unchanged, declare no list at all; an empty list would ban everything (ADR-0021 §4)`)
    } else {
      problems.push(...entryProblems(ctx, list.allow, cat))
    }
    if (seenTeams.has(list.team)) {
      problems.push(`team "${list.team}" declares two allow-lists — a Team's declared list is one intersection term (ADR-0021 §2)`)
    }
    seenTeams.add(list.team)
  }
  const seenGrants = new Set()
  for (const grant of request.grants ?? []) {
    const ctx = `grant "${grant.id}"`
    if (!grant.id) {
      problems.push('a grant has no id — everything a team may use traces to the root list or a named Grant (ADR-0021 §3)')
    }
    problems.push(...partyProblems(ctx, grant.team, grant.owner, teams, owners))
    const author = owners.get(grant.owner)
    if (author && teams.has(grant.team) && !properAncestor(teams, author.team, grant.team)) {
      problems.push(`${ctx} is authored by owner "${grant.owner}" of team "${author.team}", which is not an ancestor of target team "${grant.team}" — a Grant is a parent-authored exception (ADR-0021 §3)`)
    }
    if (!grant.adds || grant.adds.length === 0) {
      problems.push(`${ctx} adds no entries — a Grant exists to widen a palette (ADR-0021 §3)`)
    } else {
      problems.push(...entryProblems(ctx, grant.adds, cat))
    }
    if (grant.id) {
      if (seenGrants.has(grant.id)) {
        problems.push(`grant "${grant.id}" defined twice — the id is the audit chain's name for it`)
      }
      seenGrants.add(grant.id)
    }
  }
  return problems
}

let proposalCount = 0

async function handleProposal(req, res) {
  const chunks = []
  for await (const chunk of req) chunks.push(chunk)
  let request
  try {
    request = JSON.parse(Buffer.concat(chunks).toString('utf8'))
  } catch {
    res.writeHead(400, { 'content-type': 'application/json; charset=utf-8' })
    res.end(JSON.stringify({ problems: ['the request body is not JSON'] }))
    return
  }
  const problems = governanceProblems(request)
  if (problems.length > 0) {
    res.writeHead(422, { 'content-type': 'application/json; charset=utf-8' })
    res.end(JSON.stringify({ problems }))
    return
  }
  proposalCount += 1
  const proposal = {
    id: `governance-${proposalCount}`,
    url: `https://forge.example/estate/pull/${100 + proposalCount}`,
    branch: `telecraft/governance-${proposalCount}`,
  }
  res.writeHead(200, { 'content-type': 'application/json; charset=utf-8' })
  res.end(JSON.stringify(proposal))
}

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.woff2': 'font/woff2',
}

const server = createServer(async (req, res) => {
  const url = new URL(req.url, `http://${req.headers.host}`)
  if (url.pathname === '/api/v1/governance/proposals' && req.method === 'POST') {
    await handleProposal(req, res)
    return
  }
  const handler = api[url.pathname]
  if (handler) {
    const payload = handler(url)
    if (payload === undefined) {
      // Say "cannot know", never fabricate: an unknown catalogue version
      // is a 404, not an empty answer.
      res.writeHead(404, { 'content-type': 'application/json; charset=utf-8' })
      res.end(JSON.stringify({ error: `nothing here for ${url.search}` }))
      return
    }
    res.writeHead(200, { 'content-type': 'application/json; charset=utf-8' })
    res.end(JSON.stringify(payload))
    return
  }
  if (url.pathname.startsWith('/api/')) {
    res.writeHead(404, { 'content-type': 'application/json; charset=utf-8' })
    res.end(JSON.stringify({ error: `no such endpoint: ${url.pathname}` }))
    return
  }
  if (!dist) {
    res.writeHead(404, { 'content-type': 'text/plain; charset=utf-8' })
    res.end('API only: start with --dist to serve the built console\n')
    return
  }
  // Static bundle with a single-page fallback: deep-link URLs must load
  // fresh (ADR-0042 §3.5), so unknown paths serve the shell.
  const safePath = normalize(url.pathname).replace(/^([/\\]|\.\.)+/, '')
  try {
    const file = join(root, dist, safePath)
    const data = await readFile(file)
    res.writeHead(200, {
      'content-type': MIME[extname(file)] ?? 'application/octet-stream',
    })
    res.end(data)
  } catch {
    const index = await readFile(join(root, dist, 'index.html'))
    res.writeHead(200, { 'content-type': MIME['.html'] })
    res.end(index)
  }
})

server.listen(port, '127.0.0.1', () => {
  const mode = dist ? `API and ${dist}/` : 'API only'
  console.log(`fixture backend (${mode}) on http://127.0.0.1:${port}`)
})
