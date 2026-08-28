// Stages the built console where the platform binary embeds it from
// (internal/consoleassets/dist). A Go embed pattern cannot reach outside
// its own package, so the bundle is copied rather than referenced.
//
// The destination is emptied first, apart from its tracked placeholder, so
// a stale chunk from an earlier build never travels inside a binary.
//
// Usage: node tools/stage-bundle.mjs [source] [destination]

import { cp, mkdir, readdir, rm } from 'node:fs/promises'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(fileURLToPath(import.meta.url), '..', '..')
const source = join(root, process.argv[2] ?? 'dist')
const destination = join(root, process.argv[3] ?? '../internal/consoleassets/dist')

// The one file the destination keeps: it is tracked, and the embed needs a
// directory to point at even when no console has been built.
const KEEP = '.gitkeep'

await mkdir(destination, { recursive: true })
for (const entry of await readdir(destination)) {
  if (entry !== KEEP) await rm(join(destination, entry), { recursive: true, force: true })
}
await cp(source, destination, { recursive: true, filter: (path) => !path.endsWith(KEEP) })

console.log(`staged ${source} into ${destination}`)
