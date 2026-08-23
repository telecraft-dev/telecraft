// Assembles the static site a host publishes: the built console, the
// snapshot it reads, an entry document per Workspace URL, the not-found
// fallback, and the custom-domain file.
//
// The entry documents are the point. Every surface state is
// URL-addressable (ADR-0042 §3.5), which on a static host means those URLs
// have to resolve to something. A not-found document that happens to be
// the shell renders the right page but answers 404, and a 404 breaks link
// previews, sharing and every uptime check: the URL works for a human and
// is broken for everything else. So each Workspace URL gets a real
// document and answers 200; `404.html` stays for the deeper parameterised
// routes, whose shapes are unbounded and which no static host can
// enumerate.
//
// Both spellings of each route are written (`<route>.html` and
// `<route>/index.html`), because static hosts differ on which one a bare
// path resolves to, and the URL the router owns should answer whichever
// rule the host applies.
//
// Usage: node tools/assemble-site.mjs <dist-dir> [--snapshot file] [--domain host]

import { copyFile, mkdir, readFile, writeFile } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import process from 'node:process'

/**
 * The Workspace URLs to pre-render, mirroring src/chrome/workspaces.ts.
 * tests/site.test.ts holds the two lists together: a Workspace added to
 * the chrome and not here would ship a URL that 404s.
 */
export const WORKSPACE_ROUTES = ['/estate', '/topology', '/compose', '/catalogue']

/** The documents an assembled site carries beyond the built bundle. */
export function entryDocuments(routes = WORKSPACE_ROUTES) {
  const out = ['404.html']
  for (const route of routes) {
    const path = route.replace(/^\//, '')
    out.push(`${path}.html`, `${path}/index.html`)
  }
  return out
}

async function main() {
  const args = process.argv.slice(2)
  const dist = args[0]
  if (!dist || dist.startsWith('--')) {
    console.error('usage: node tools/assemble-site.mjs <dist-dir> [--snapshot file] [--domain host]')
    process.exit(2)
  }
  const value = (flag) => {
    const i = args.indexOf(flag)
    return i >= 0 ? args[i + 1] : undefined
  }
  const snapshot = value('--snapshot')
  const domain = value('--domain')

  const shell = await readFile(join(dist, 'index.html'), 'utf8')
  for (const document of entryDocuments()) {
    const path = join(dist, document)
    await mkdir(dirname(path), { recursive: true })
    await writeFile(path, shell)
  }

  if (snapshot) {
    const placed = join(dist, 'demo-snapshot.json')
    if (resolve(snapshot) !== resolve(placed)) {
      await copyFile(snapshot, placed)
    }
  }
  if (domain) {
    // The custom domain has to travel with every deployment, or the next
    // one drops it.
    await writeFile(join(dist, 'CNAME'), `${domain}\n`)
  }

  console.log(`assembled ${dist}: ${entryDocuments().length} entry documents` +
    `${snapshot ? ', snapshot' : ''}${domain ? `, ${domain}` : ''}`)
}

// Importable for the test, runnable for the deploy.
if (import.meta.url === `file://${process.argv[1]}`) {
  await main()
}
