import { expect, test as setup } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { join } from 'node:path'

// Sign the fixture user in once through the documented login endpoint and
// persist the session cookie; every spec runs signed in unless it opts
// into an empty storageState (auth.spec.ts does, to test the gate).
//
// The saved state also carries one presentation preference: the welcome
// Tour, already offered. It opens itself once per reader on a bare landing
// URL (ADR-0051 §7), and every spec but tour.spec.ts is testing the
// product rather than the Tour. So the suite runs as a reader who has seen
// it, and tour.spec.ts clears the store to opt back in — the same shape as
// auth.spec.ts opting out of the session.
const PRESENTATION_KEY = 'telecraft.console.presentation.v1.demo-user'

setup('sign the fixture user in', async ({ page }) => {
  // `page.request` shares the browser context's cookie jar, so the session
  // this returns is the one the saved state carries.
  const res = await page.request.post('/api/v1/auth/login', {
    data: { provider: 'basic', username: 'demo@example.com', secret: 'demo-password' },
  })
  expect(res.ok()).toBeTruthy()

  // localStorage is per origin, so the page has to be on it to write.
  await page.goto('/estate')
  await page.evaluate((key) => {
    localStorage.setItem(
      key,
      JSON.stringify({ collapsedSections: {}, arrangement: {}, toursSeen: { welcome: true } }),
    )
  }, PRESENTATION_KEY)

  await mkdir(join('e2e', '.auth'), { recursive: true })
  await page.context().storageState({ path: join('e2e', '.auth', 'state.json') })
})
