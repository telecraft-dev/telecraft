// The bundle budget check (issue #125, ADR-0042 §1, ADR-0045).
//
// The console is five Workspaces behind TanStack Router, and `src/router.tsx`
// loads all but Home's component through a dynamic import (Home is the
// landing, and is eager on purpose; ADR-0056). That split is only
// worth having if it stays split, so this is the check that holds it: the
// third of the small Node scripts over `dist` that guard a rule review
// cannot, beside the zero-CDN check and the palette check.
//
// It measures one number: the gzipped size of the **entry chunk**, which is
// the JavaScript module `dist/index.html` loads. That is the file every
// reader downloads before anything renders, whichever Workspace their URL
// names, so it is the one that has to be argued for. The Workspace chunks
// and the chunks they share are reported as notes and are not budgeted:
// they are paid for on navigation by the reader who navigates.
//
// Gzip is the number, because that is what a host transfers and what Vite
// prints. Sizes are stated in kB of 1000 bytes for the same reason: it makes
// the failure message comparable with the build output the reader just saw.
//
// Usage: node tools/check-bundle-budget.mjs <dist-dir>

import { readdir, readFile } from 'node:fs/promises'
import { extname, join, relative } from 'node:path'
import process from 'node:process'
import { gzipSync } from 'node:zlib'

// The ceiling, in gzipped bytes.
//
// The split measured 121.6 kB gzipped, so this is 18.4 kB of headroom,
// about 15%. That number is chosen against the thing the check exists to
// catch rather than picked as a round percentage.
//
// What is left in the entry chunk is the framework and the shell: React,
// TanStack Router and Query, the Radix dialog primitive, the icons, the API
// client, and `src/tours/`, which grows by a file of prose every time a Tour
// is written (ADR-0051). That growth is real and it is the growth the
// headroom is for. A Tour and a chrome control are hundreds of bytes each,
// so 18.4 kB buys years of them.
//
// What the headroom deliberately does not cover is a Workspace falling back
// out of its dynamic import. The canvas substrate alone is 58.8 kB gzipped,
// so either canvas Workspace becoming eager again overshoots this ceiling
// more than three times over and fails loudly, which is the whole point:
// "the entry chunk no longer carries the canvas engine" is an acceptance
// criterion, and this is what keeps it true.
//
// The demo bundle is the larger of the two, because `src/api/demo.ts`
// answers the whole contract from a snapshot and rides in the entry chunk:
// it measured 126.3 kB gzipped against the instance bundle's 121.6 kB. One
// ceiling covers both, and it is the demo bundle that will reach it first.
//
// Raising this number is a reviewable event. Raise it in the commit that
// needs it, with the reason, or move the weight behind a route.
const CEILING = 140_000

const kb = (bytes) => `${(bytes / 1000).toFixed(1)} kB`

/**
 * The entry chunk, as the entry document names it. Reading it out of the
 * HTML rather than matching a filename is what makes this a measure of
 * first-load cost: Vite hashes chunk names, and the module the browser is
 * told to load is the only definition of "entry" that cannot drift.
 */
function entryChunk(html) {
  const pattern = /<script\b[^>]*\btype="module"[^>]*\bsrc="([^"]+)"/g
  return [...html.matchAll(pattern)].map(([, src]) => src.replace(/^\//, ''))
}

/** Every JavaScript chunk under the built tree, entry and lazy alike. */
async function* chunks(dir) {
  for (const item of await readdir(dir, { withFileTypes: true })) {
    const path = join(dir, item.name)
    if (item.isDirectory()) yield* chunks(path)
    else if (extname(path) === '.js') yield path
  }
}

const dist = process.argv[2]
if (!dist) {
  console.error('usage: node tools/check-bundle-budget.mjs <dist-dir>')
  process.exit(2)
}

let html
try {
  html = await readFile(join(dist, 'index.html'), 'utf8')
} catch {
  console.error(`bundle budget check: no ${join(dist, 'index.html')}, so was the console built?`)
  process.exit(2)
}

const entries = entryChunk(html)
if (entries.length !== 1) {
  console.error(
    `bundle budget check: ${join(dist, 'index.html')} loads ${entries.length} module scripts, expected exactly one entry chunk`,
  )
  process.exit(2)
}

const [entry] = entries

const measured = []
for await (const path of chunks(dist)) {
  const bytes = await readFile(path)
  measured.push({ name: relative(dist, path), gzipped: gzipSync(bytes).length, raw: bytes.length })
}
measured.sort((a, b) => b.gzipped - a.gzipped)

for (const chunk of measured) {
  const role = chunk.name === entry ? 'entry' : 'lazy '
  console.log(`  note: ${role} ${chunk.name} is ${kb(chunk.raw)}, ${kb(chunk.gzipped)} gzipped`)
}

const gzipped = measured.find((chunk) => chunk.name === entry)?.gzipped
if (gzipped === undefined) {
  console.error(`bundle budget check: ${join(dist, 'index.html')} loads ${entry}, which is not in the built tree`)
  process.exit(2)
}
console.log(`  note: the ceiling is ${kb(CEILING)} gzipped, leaving ${kb(CEILING - gzipped)}`)

if (gzipped > CEILING) {
  console.error(
    `bundle budget check failed (issue #125): the entry chunk is ${kb(gzipped)} gzipped, over the ${kb(CEILING)} ceiling.`,
  )
  console.error('  Every reader pays this before anything renders, whichever Workspace their URL names.')
  console.error('  Move the weight behind a route: a Workspace component reached through a dynamic')
  console.error('  import in src/router.tsx lands in its own chunk, loaded on navigation. If the')
  console.error('  growth is genuinely the shell, raise CEILING in this file in the same commit and')
  console.error('  say why, so the new number is reviewed rather than absorbed.')
  process.exit(1)
}

console.log(
  `bundle budget check passed: the entry chunk is ${kb(gzipped)} gzipped, within the ${kb(CEILING)} ceiling`,
)
