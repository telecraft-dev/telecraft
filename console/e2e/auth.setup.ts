import { expect, test as setup } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { join } from 'node:path'

// Sign the fixture user in once through the documented login endpoint and
// persist the session cookie; every spec runs signed in unless it opts
// into an empty storageState (auth.spec.ts does, to test the gate).
setup('sign the fixture user in', async ({ request }) => {
  const res = await request.post('/api/v1/auth/login', {
    data: { provider: 'basic', username: 'demo@example.com', secret: 'demo-password' },
  })
  expect(res.ok()).toBeTruthy()
  await mkdir(join('e2e', '.auth'), { recursive: true })
  await request.storageState({ path: join('e2e', '.auth', 'state.json') })
})
