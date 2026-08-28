import { execSync } from 'node:child_process'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// The version the profile menu names (ADR-0065): an environment variable
// first, so a packaging step can state the version exactly; the git
// description otherwise; the literal `development` when neither exists,
// which is what a tarball checkout or a tagless shallow clone yields.
// Synchronous on purpose: config evaluation is the one moment the value
// can be captured, and a failed `git` is an expected outcome, not an
// error.
function consoleVersion(): string {
  const fromEnv = process.env.TELECRAFT_VERSION?.trim()
  if (fromEnv) return fromEnv
  try {
    const described = execSync('git describe --tags --always --dirty', {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
    }).trim()
    if (described) return described
  } catch {
    // No repository, or no git: fall through to the honest word.
  }
  return 'development'
}

// The dev server proxies the documented platform API to the fixture
// backend (tools/fixture-backend.mjs); `npm run backend` starts it.
// Nothing is ever fetched from outside the origin: ADR-0045 §5's zero-CDN
// rule is checked over the built bundle by tools/check-zero-cdn.mjs.
//
// The snapshot proxy is for `npm run dev:demo`, which runs this server in
// demo mode against the local development environment rather than against
// a build-time snapshot: the loop there rebuilds the snapshot every few
// seconds from live collectors and a live backend (ADR-0052). Both proxies
// are dev-server only and neither exists in a built bundle.
export default defineConfig({
  plugins: [react()],
  define: {
    __TELECRAFT_VERSION__: JSON.stringify(consoleVersion()),
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:4700',
      '/demo-snapshot.json': 'http://127.0.0.1:4321',
    },
  },
  test: {
    include: ['tests/**/*.test.ts'],
    environment: 'node',
  },
})
