import { expect, test } from '@playwright/test'

// The issue #25 acceptance criteria, end to end: the console boots against
// the fixture backend, the four Workspaces navigate, jump-to-object finds
// authored objects and deep-links, a deep-link URL restores state, the
// presentation store persists per user, and nothing is ever fetched from
// outside the origin.

test('boots against the fixture backend onto the shelf', async ({ page }) => {
  await page.goto('/')
  await expect(page).toHaveURL(/\/estate/)
  await expect(page.getByTestId('shelf')).toBeVisible()
  // The resting scope is the signed-in user's team subtree (ADR-0042 §2).
  await expect(page.getByTestId('section-data-flow')).toBeVisible()
  await expect(page.getByTestId('card-data-flow/gateway')).toBeVisible()
})

test('cards order worst-severity-first within an Environment row', async ({ page }) => {
  await page.goto('/estate')
  const row = page.locator('[data-environment="production"] .card-face')
  // gateway carries a violation, edge is healthy: worst leads.
  await expect(row.first()).toHaveAttribute('data-testid', 'card-data-flow/gateway')
})

test('all four Workspaces are navigable', async ({ page }) => {
  await page.goto('/')
  await page.getByTestId('nav-topology').click()
  await expect(page.getByTestId('topology-canvas')).toBeVisible()
  await page.getByTestId('nav-compose').click()
  await expect(page.getByTestId('blueprint-data-flow/gateway-standard')).toBeVisible()
  await page.getByTestId('nav-catalogue').click()
  await expect(page.getByTestId('catalogue-table')).toBeVisible()
  await page.getByTestId('nav-estate').click()
  await expect(page.getByTestId('shelf')).toBeVisible()
})

test('jump-to-object finds a Tier by name and deep-links to its card', async ({ page }) => {
  await page.goto('/topology')
  await page.getByTestId('jump-trigger').click()
  await page.getByTestId('jump-input').fill('gateway')
  await page.getByTestId('jump-result-tier-data-flow/gateway').click()
  await expect(page).toHaveURL(/\/estate\?.*object=tier%3Adata-flow%2Fgateway/)
  await expect(page.getByTestId('card-panel')).toBeVisible()
  await expect(page.getByTestId('panel-title')).toHaveText('gateway')
})

test('jump-to-object reaches a Service and traces its Paths on the canvas', async ({ page }) => {
  await page.goto('/estate')
  await page.getByTestId('jump-trigger').click()
  await page.getByTestId('jump-input').fill('checkout')
  await page.getByTestId('jump-result-service-product/checkout').click()
  await expect(page).toHaveURL(/\/topology\?.*object=service%3Aproduct%2Fcheckout/)
  // Tiers off the traced Paths dim; the traced ones stay lit (ADR-0044 §4).
  await expect(
    page.getByTestId('topology-canvas').locator('.canvas-node.kind-tier.dimmed'),
  ).toHaveCount(1)
})

test('a deep-link URL restores workspace, object, and lens state', async ({ page }) => {
  await page.goto('/topology?object=tier%3Adata-flow%2Fgateway&lens=staging')
  await expect(page.getByTestId('nav-topology')).toHaveAttribute('aria-current', 'page')
  await expect(page.getByTestId('lens-control')).toHaveValue('staging')
  await expect(page.getByTestId('card-panel')).toBeVisible()
  await expect(page.getByTestId('panel-title')).toHaveText('gateway')
})

test('the lens preference persists per user; an explicit URL lens beats it', async ({ page }) => {
  await page.goto('/estate')
  await expect(page.getByTestId('lens-control')).toHaveValue('production')
  await page.getByTestId('lens-control').selectOption('staging')
  // A fresh visit on a bare URL falls back to the persisted preference.
  await page.goto('/estate')
  await expect(page.getByTestId('lens-control')).toHaveValue('staging')
  // An explicit lens in the URL beats the preference (ADR-0042 §4).
  await page.goto('/estate?lens=production')
  await expect(page.getByTestId('lens-control')).toHaveValue('production')
  // The store is per user and holds presentation state only.
  const stored = await page.evaluate(() => {
    const raw = window.localStorage.getItem('telecraft.console.presentation.v1.demo-user')
    return raw === null ? null : (JSON.parse(raw) as Record<string, unknown>)
  })
  expect(stored).not.toBeNull()
  for (const key of Object.keys(stored!)) {
    expect(['lens', 'collapsedSections', 'arrangement', 'toursSeen']).toContain(key)
  }
})

test('the console never fetches from outside its origin', async ({ page }) => {
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
  await page.getByTestId('nav-topology').click()
  await expect(page.getByTestId('topology-canvas')).toBeVisible()
  await page.getByTestId('nav-compose').click()
  await page.getByTestId('nav-catalogue').click()
  await expect(page.getByTestId('catalogue-table')).toBeVisible()
  expect(external).toEqual([])
})
