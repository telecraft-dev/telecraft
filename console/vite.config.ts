import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// The dev server proxies the documented platform API to the fixture
// backend (tools/fixture-backend.mjs); `npm run backend` starts it.
// Nothing is ever fetched from outside the origin: ADR-0045 §5's zero-CDN
// rule is checked over the built bundle by tools/check-zero-cdn.mjs.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: { '/api': 'http://127.0.0.1:4700' },
  },
  test: {
    include: ['tests/**/*.test.ts'],
    environment: 'node',
  },
})
