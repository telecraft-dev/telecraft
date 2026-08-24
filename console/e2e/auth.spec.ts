import { expect, test } from '@playwright/test'

// The issue #26 acceptance criteria, end to end: signed out, every URL
// renders the sign-in surface and resumes after login (REQ-017,
// ADR-0019); signed in, the acting user's team membership determines
// which authoring affordances surfaces offer (ADR-0016/0017); and the
// whole flow touches nothing beyond its own origin (REQ-006).

test.describe('signed out', () => {
  test.use({ storageState: { cookies: [], origins: [] } })

  test('the gate renders sign-in, and a deep link resumes after login', async ({ page }) => {
    await page.goto('/compose?object=blueprint%3Adata-flow%2Fgateway-standard')
    await expect(page.getByTestId('login')).toBeVisible()

    await page.getByTestId('login-username').fill('demo@example.com')
    await page.getByTestId('login-secret').fill('demo-password')
    await page.getByTestId('login-submit').click()

    // The URL survived the gate: the same deep link is now the surface.
    await expect(page.getByTestId('compose-detail')).toBeVisible()
    await expect(page).toHaveURL(/\/compose\?object=blueprint%3Adata-flow%2Fgateway-standard/)
    await expect(page.getByTestId('chrome-user')).toHaveText('Demo user')
  })

  test('wrong credentials fail uniformly and stay on the gate', async ({ page }) => {
    await page.goto('/')
    await page.getByTestId('login-username').fill('demo@example.com')
    await page.getByTestId('login-secret').fill('not-the-password')
    await page.getByTestId('login-submit').click()
    await expect(page.getByTestId('login-error')).toBeVisible()
    await expect(page.getByTestId('login')).toBeVisible()
  })

  test('signing out returns to the gate', async ({ page }) => {
    // A session of this test's own: signing out must not revoke the
    // shared storageState the parallel specs ride on.
    // Arriving at `/`, because that is the bare landing the welcome Tour
    // opens itself on, and this test closes it below (ADR-0051 §7,
    // ADR-0056 §1 moved the landing from the shelf to Home).
    await page.goto('/')
    await page.getByTestId('login-username').fill('demo@example.com')
    await page.getByTestId('login-secret').fill('demo-password')
    await page.getByTestId('login-submit').click()
    await expect(page.getByTestId('home')).toBeVisible()

    // This session's reader is brand new, and the welcome Tour opens
    // itself for exactly that reader (ADR-0051 §7). It is a modal, so it
    // is in front of the chrome until it is closed, which is what a
    // welcome is for, and what tour.spec.ts tests.
    await page.getByTestId('tour-end').click()

    await page.getByTestId('sign-out').click()
    await expect(page.getByTestId('login')).toBeVisible()
  })

  test('sign-in itself never leaves the origin', async ({ page }) => {
    const external: string[] = []
    await page.route('**/*', async (route) => {
      const url = new URL(route.request().url())
      if (url.hostname !== '127.0.0.1' && url.hostname !== 'localhost') {
        external.push(route.request().url())
        await route.abort()
        return
      }
      await route.continue()
    })
    await page.goto('/')
    await page.getByTestId('login-username').fill('demo@example.com')
    await page.getByTestId('login-secret').fill('demo-password')
    await page.getByTestId('login-submit').click()
    await expect(page.getByTestId('home')).toBeVisible()
    expect(external).toEqual([])
  })
})

test('team membership decides the authoring affordance per Blueprint', async ({ page }) => {
  // data-flow/gateway-standard is owned inside the signed-in user's team
  // subtree: authoring is offered.
  await page.goto('/compose?object=blueprint%3Adata-flow%2Fgateway-standard')
  await expect(page.getByTestId('compose-authoring')).toHaveClass(/editable/)

  // infosec/audit-standard is a sibling team's: the same surface is
  // honestly read-only, and says whose it is.
  await page.getByTestId('blueprint-infosec/audit-standard').click()
  await expect(page.getByTestId('compose-authoring')).toHaveClass(/readonly/)
  await expect(page.getByTestId('compose-authoring')).toContainText('infosec')
})
