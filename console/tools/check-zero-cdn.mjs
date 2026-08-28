// The zero-CDN check (ADR-0045 §5, REQ-006): no built artefact references
// an external host: fonts, scripts, styles, everything is vendored. The
// check lives beside the vendor-word lint in CI, not in a doc.
//
// Every text file in the bundle is scanned for absolute and
// protocol-relative URLs. HTML, CSS, and SVG tolerate none that could be
// fetched. JavaScript additionally tolerates the allowlisted never-fetched
// string literals below (error-message documentation links, and the
// version link the profile menu names); anything else fails the build.
//
// XML namespace identifiers are the one exception everywhere: `xmlns` is a
// name, not an address, and no parser has ever dereferenced one. An SVG
// asset cannot be authored without it.
//
// Usage: node tools/check-zero-cdn.mjs <dist-dir>

import { readdir, readFile } from 'node:fs/promises'
import { extname, join, relative } from 'node:path'
import process from 'node:process'

const TEXT_EXTENSIONS = new Set(['.html', '.css', '.js', '.mjs', '.svg', '.json', '.txt', '.map', '.webmanifest'])

// Namespace identifiers: names, not addresses, and never dereferenced.
// Allowed in every file type, because an SVG cannot declare itself
// without one.
const NAMESPACES = [/http:\/\/www\.w3\.org\//]

// String literals that name a URL without ever fetching it. Each entry
// must stay justifiable as never-fetched; growing this list is a
// reviewable event.
const NEVER_FETCHED = [
  // Framework error messages link their documentation.
  /https:\/\/react\.dev\//,
  // The canvas library's attribution link: the console hides the
  // attribution element (proOptions in FlowCanvas.tsx), so the string is
  // never rendered, navigated, or fetched.
  /https:\/\/reactflow\.dev\b/,
  // The version link in the profile menu (ADR-0065): the bundle names
  // the release or commit the build came from and never loads anything
  // from it. A navigation the reader chooses is not a runtime dependency
  // (ADR-0045 §5); an air-gapped deployment renders identically with the
  // address unreachable.
  /https:\/\/github\.com\/telecraft-dev\/telecraft\b/,
]

const URL_PATTERN = /(?:https?:)?\/\/[a-z0-9][a-z0-9.-]*\.[a-z]{2,}(?![a-z0-9.-])[^\s"'`)<>\\]*/gi

async function* walk(dir) {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name)
    if (entry.isDirectory()) yield* walk(path)
    else yield path
  }
}

const dist = process.argv[2]
if (!dist) {
  console.error('usage: node tools/check-zero-cdn.mjs <dist-dir>')
  process.exit(2)
}

let scanned = 0
const violations = []
for await (const path of walk(dist)) {
  const ext = extname(path)
  if (!TEXT_EXTENSIONS.has(ext)) continue
  scanned++
  const text = await readFile(path, 'utf8')
  const allowJsLiterals = ext === '.js' || ext === '.mjs' || ext === '.map'
  for (const match of text.matchAll(URL_PATTERN)) {
    const url = match[0]
    if (NAMESPACES.some((re) => re.test(url))) continue
    if (allowJsLiterals && NEVER_FETCHED.some((re) => re.test(url))) continue
    const line = text.slice(0, match.index).split('\n').length
    violations.push(`${relative(dist, path)}:${line}: external reference ${url}`)
  }
}

if (scanned === 0) {
  console.error(`zero-CDN check: no text files under ${dist}. Was the console built?`)
  process.exit(2)
}
if (violations.length > 0) {
  console.error('zero-CDN check failed (ADR-0045 §5): the bundle references external hosts:')
  for (const violation of violations) console.error(`  ${violation}`)
  process.exit(1)
}
console.log(`zero-CDN check passed: ${scanned} text files, no external references`)
