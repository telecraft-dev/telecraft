// The fixture backend: a real HTTP server implementing the documented
// platform API (console/README.md) over the fixture estate in
// console/fixtures/estate.json. The console consumes only this contract
// (ADR-0045 §6); when the platform binary grows the API endpoint, this
// server is what it replaces. With --dist it also serves the built bundle,
// which is how the Playwright suite runs the console end to end.
//
// Auth mirrors the internal/auth handler (REQ-017, ADR-0019): every
// /api/v1/* endpoint outside /api/v1/auth/* wants a session cookie, basic
// auth signs the fixture user in, and /api/v1/me derives editableTeams as
// the user's team subtree. Sessions are in-memory, gone on restart — the
// same stateless posture as the real server (ADR-0013).
//
// Usage: node tools/fixture-backend.mjs [--port 4700] [--dist dist]

import { randomBytes } from 'node:crypto'
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

// The fixture credential, printed at start-up. A fixture holds it in the
// clear; the platform binary verifies users.yaml PBKDF2 hashes instead.
const credentials = { username: 'demo@example.com', secret: 'demo-password' }
const sessions = new Set()

// The signed-in user's edit horizon: their team subtree, derived from the
// fixture team tree exactly as the platform derives it from the ownership
// tree (ADR-0019 §2 over ADR-0017).
function subtree(teamId) {
  const find = (node) => {
    if (node.id === teamId) return node
    for (const child of node.teams ?? []) {
      const hit = find(child)
      if (hit) return hit
    }
    return undefined
  }
  const rooted = find(estate.teams)
  if (!rooted) return []
  const out = []
  const walk = (node) => {
    out.push(node.id)
    for (const child of node.teams ?? []) walk(child)
  }
  walk(rooted)
  return out
}

function me() {
  return { ...estate.me, editableTeams: subtree(estate.me.team) }
}

function sessionOf(req) {
  const cookies = req.headers.cookie ?? ''
  for (const part of cookies.split(';')) {
    const [name, value] = part.trim().split('=')
    if (name === 'telecraft_session' && sessions.has(value)) return value
  }
  return undefined
}

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
  return out
}

const api = {
  '/api/v1/me': me,
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
  // Tiers at authored grain with selector-matched counts (ADR-0007): the
  // matched number is the card face's population, single-sourced; the
  // served/git split is Tier-aggregated delivery-path detail.
  '/api/v1/topology': () => ({
    environments: estate.environments,
    tiers: estate.cards.map((card) => ({
      id: card.tier,
      name: card.name,
      team: card.team,
      environment: card.environment,
      ...(card.serviceClass ? { serviceClass: card.serviceClass } : {}),
      matched: card.population.matched,
      delivery: estate.topology.delivery[card.tier] ?? {
        served: card.population.matched,
        git: 0,
      },
    })),
    sources: estate.topology.sources,
    hops: estate.topology.hops,
    paths: estate.topology.paths,
  }),
  '/api/v1/blueprints': () => estate.blueprints,
  '/api/v1/catalogue': () => estate.catalogue,
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

const sendJSON = (res, status, body, headers = {}) => {
  res.writeHead(status, { 'content-type': 'application/json; charset=utf-8', ...headers })
  res.end(JSON.stringify(body))
}

const readBody = (req) =>
  new Promise((resolve) => {
    let raw = ''
    req.on('data', (chunk) => (raw += chunk))
    req.on('end', () => {
      try {
        resolve(JSON.parse(raw))
      } catch {
        resolve({})
      }
    })
  })

const server = createServer(async (req, res) => {
  const url = new URL(req.url, `http://${req.headers.host}`)

  // The auth slice, open to the signed-out (console/README.md).
  if (url.pathname === '/api/v1/auth/providers' && req.method === 'GET') {
    sendJSON(res, 200, [{ name: 'basic', flow: 'password' }])
    return
  }
  if (url.pathname === '/api/v1/auth/login' && req.method === 'POST') {
    const body = await readBody(req)
    if (body.username !== credentials.username || body.secret !== credentials.secret) {
      sendJSON(res, 401, { error: 'invalid credentials' })
      return
    }
    const token = randomBytes(16).toString('hex')
    sessions.add(token)
    sendJSON(res, 200, me(), {
      'set-cookie': `telecraft_session=${token}; Path=/; HttpOnly; SameSite=Lax`,
    })
    return
  }
  if (url.pathname === '/api/v1/auth/logout' && req.method === 'POST') {
    const token = sessionOf(req)
    if (token) sessions.delete(token)
    res.writeHead(204, {
      'set-cookie': 'telecraft_session=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0',
    })
    res.end()
    return
  }

  const handler = api[url.pathname]
  if (handler) {
    if (!sessionOf(req)) {
      sendJSON(res, 401, { error: 'sign in to use this API' })
      return
    }
    sendJSON(res, 200, handler(url))
    return
  }
  if (url.pathname.startsWith('/api/')) {
    sendJSON(res, 404, { error: `no such endpoint: ${url.pathname}` })
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
  console.log(`sign in as ${credentials.username} / ${credentials.secret}`)
})
