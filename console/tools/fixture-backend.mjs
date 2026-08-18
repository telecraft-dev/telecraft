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
import { propose, validate } from './evaluator.mjs'

const args = process.argv.slice(2)
const argValue = (flag) => {
  const i = args.indexOf(flag)
  return i >= 0 ? args[i + 1] : undefined
}
const port = Number(argValue('--port') ?? process.env.PORT ?? 4700)
const dist = argValue('--dist')

const root = join(fileURLToPath(import.meta.url), '..', '..')
const estate = JSON.parse(await readFile(join(root, 'fixtures', 'estate.json'), 'utf8'))

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
}

// The composing endpoints (ADR-0022 §1): one evaluator behind both — the
// composer's continuous advisory call, and the proposal exit with
// enforcement on. A blocked proposal answers 409, fail closed (ADR-0028 §3).
const postApi = {
  '/api/v1/validate': (body) => validate(estate, body.draft, body.environment),
  '/api/v1/proposals': (body) => propose(estate, body.draft, body.environment),
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = []
    req.on('data', (chunk) => chunks.push(chunk))
    req.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')))
    req.on('error', reject)
  })
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
  const postHandler = req.method === 'POST' ? postApi[url.pathname] : undefined
  if (postHandler) {
    try {
      const body = JSON.parse(await readBody(req))
      const result = postHandler(body)
      res.writeHead(200, { 'content-type': 'application/json; charset=utf-8' })
      res.end(JSON.stringify(result))
    } catch (err) {
      res.writeHead(err.status ?? 400, { 'content-type': 'application/json; charset=utf-8' })
      res.end(JSON.stringify({ error: err.message }))
    }
    return
  }
  const handler = api[url.pathname]
  if (handler) {
    const body = JSON.stringify(handler(url))
    res.writeHead(200, { 'content-type': 'application/json; charset=utf-8' })
    res.end(body)
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
