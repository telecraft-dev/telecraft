import { defineConfig } from '@playwright/test'
import { OUTPUT_DIR } from './tools/capture'

// The suite runs the built console against the fixture backend: the same
// server serves the documented API and the bundle, so every test exercises
// the console exactly as it boots against a real backend. The setup
// project signs the fixture user in through the login endpoint and
// persists the session; auth.spec.ts opts back out to test the gate.
export default defineConfig({
  testDir: 'e2e',
  // Everything the browser harness writes lands here, absolute and resolved
  // from the console package rather than from the working directory (issue
  // #99): failure traces and per-test artefacts, and the captures
  // `npm run capture` takes. Leaving it unset put a relative path at the
  // mercy of the directory the command was run from, and twice a screenshot
  // landed in the repository root and was committed by accident.
  // `console/.gitignore` ignores the directory, and the suite clears it at
  // the start of every run.
  outputDir: OUTPUT_DIR,
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    baseURL: 'http://127.0.0.1:4700',
  },
  projects: [
    {
      name: 'setup',
      testMatch: /auth\.setup\.ts/,
    },
    {
      name: 'console',
      dependencies: ['setup'],
      testIgnore: /auth\.setup\.ts/,
      use: { storageState: 'e2e/.auth/state.json' },
    },
  ],
  webServer: {
    command: 'node tools/fixture-backend.mjs --dist dist --port 4700',
    // The providers listing answers signed out; me would 401 here.
    url: 'http://127.0.0.1:4700/api/v1/auth/providers',
    reuseExistingServer: !process.env.CI,
  },
})
