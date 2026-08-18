import { defineConfig } from '@playwright/test'

// The suite runs the built console against the fixture backend: the same
// server serves the documented API and the bundle, so every test exercises
// the console exactly as it boots against a real backend.
export default defineConfig({
  testDir: 'e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    baseURL: 'http://127.0.0.1:4700',
  },
  webServer: {
    command: 'node tools/fixture-backend.mjs --dist dist --port 4700',
    url: 'http://127.0.0.1:4700/api/v1/me',
    reuseExistingServer: !process.env.CI,
  },
})
