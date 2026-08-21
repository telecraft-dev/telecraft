import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

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
