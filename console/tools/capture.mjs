// One command for a console screenshot, and one place its output goes
// (issue #99).
//
// A capture taken by hand used to land wherever the relative path it was
// given resolved, which for a command run at the repository root was the
// repository root. That happened twice, and both times the image was
// committed by accident and removed again. The cause was that nothing
// configured where a capture goes, so every caller invented a path.
//
// This tool takes a name rather than a path, and writes into the directory
// `playwright.config.ts` sets as `outputDir`, which it imports from here so
// the harness and this tool cannot drift apart. No argument points it
// anywhere else, and the directory is resolved from this file rather than
// from the working directory, so the answer does not depend on where the
// command was run.
//
// The directory is ignored by `console/.gitignore`, and the Playwright
// suite clears it at the start of every run. A capture is a working file,
// not an archive: copy anything worth keeping somewhere else.
//
// Usage:
//   node tools/capture.mjs <url-or-route> <name> [--full-page]
//
//   npm run backend -- --dist dist        # serves the API and the bundle
//   npm run capture -- /estate estate
//   npm run capture -- /topology topology --full-page
//   npm run capture -- http://localhost:5173/compose compose
//
// A route resolves against the fixture backend the Playwright suite runs
// over, and against that backend the tool signs the fixture user in the way
// the suite's setup project does, so a capture shows the console rather than
// the login gate. Any other URL is captured exactly as it answers.

import { chromium } from '@playwright/test'
import { access, mkdir } from 'node:fs/promises'
import { join, relative } from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

// The console package root, so every path below is independent of the
// directory the command was run from.
const CONSOLE_ROOT = fileURLToPath(new URL('..', import.meta.url))

// Everything the browser harness writes goes here: the Playwright suite's
// per-test artefacts, and the captures this tool takes. `playwright.config.ts`
// reads it as `outputDir`.
export const OUTPUT_DIR = join(CONSOLE_ROOT, 'test-results')

// Where the fixture backend serves the built console, which is the suite's
// own `use.baseURL` in `playwright.config.ts`.
const BASE_URL = 'http://127.0.0.1:4700'

// Written by the `setup` project in the Playwright suite. It carries the
// presentation preferences a capture wants, notably the welcome Tour
// already offered, so a capture is of the surface rather than of the Tour.
const STORAGE_STATE = join(CONSOLE_ROOT, 'e2e', '.auth', 'state.json')

// The fixture backend's user, which it prints at start-up. Its session is
// held in memory, so the saved state's cookie dies with the process that
// issued it and a capture signs in again.
const FIXTURE_LOGIN = { provider: 'basic', username: 'demo@example.com', secret: 'demo-password' }

const VIEWPORT = { width: 1440, height: 900 }

/**
 * Resolves a capture name to its path inside the configured directory.
 *
 * The argument is a name, never a path: a separator or a parent segment is
 * an error rather than a way out of the directory, which is what stops a
 * capture landing beside the working tree again.
 */
export function capturePath(name) {
  const file = name.endsWith('.png') ? name : `${name}.png`
  if (file === '.png' || file.includes('/') || file.includes('\\') || file.startsWith('.')) {
    throw new Error(`capture name must be a plain file name, not a path: ${name}`)
  }
  return join(OUTPUT_DIR, file)
}

async function exists(path) {
  try {
    await access(path)
    return true
  } catch {
    return false
  }
}

async function main() {
  const args = process.argv.slice(2)
  const fullPage = args.includes('--full-page')
  const [target, name, ...extra] = args.filter((arg) => !arg.startsWith('--'))

  if (!target || !name || extra.length > 0) {
    console.error('usage: node tools/capture.mjs <url-or-route> <name> [--full-page]')
    process.exit(2)
  }

  const url = new URL(target, BASE_URL).href
  let path
  try {
    path = capturePath(name)
  } catch (error) {
    console.error(`capture failed: ${error.message}`)
    process.exit(2)
  }
  await mkdir(OUTPUT_DIR, { recursive: true })

  const storageState = (await exists(STORAGE_STATE)) ? STORAGE_STATE : undefined
  const browser = await chromium.launch()
  let signedIn = false
  try {
    const context = await browser.newContext({ storageState, viewport: VIEWPORT })
    if (new URL(url).origin === BASE_URL) {
      // `context.request` shares the browser's cookie jar, so the session
      // this returns is the one the page loads with.
      const res = await context.request.post(`${BASE_URL}/api/v1/auth/login`, { data: FIXTURE_LOGIN })
      signedIn = res.ok()
    }
    const page = await context.newPage()
    await page.goto(url, { waitUntil: 'networkidle' })
    await page.screenshot({ path, fullPage })
  } finally {
    await browser.close()
  }

  console.log(`captured ${url} into ${relative(CONSOLE_ROOT, path)}` +
    `${signedIn ? ' (signed in as the fixture user)' : ''}`)
}

// Importable for the config and the test, runnable for a capture.
if (import.meta.url === `file://${process.argv[1]}`) {
  try {
    await main()
  } catch (error) {
    // A backend that is not running is the common one, and its stack trace
    // says nothing the message does not.
    console.error(`capture failed: ${error.message}`)
    if (error.message.includes('ECONNREFUSED')) {
      console.error('nothing is serving that URL: `npm run backend -- --dist dist` serves the built console')
    }
    process.exit(1)
  }
}
