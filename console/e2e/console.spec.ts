import { expect, test } from '@playwright/test'

// The issue #25 acceptance criteria, end to end: the console boots against
// the fixture backend, the five Workspaces navigate, jump-to-object finds
// authored objects and deep-links, a deep-link URL restores state, the
// presentation store persists per user, and nothing is ever fetched from
// outside the origin.

test('boots against the fixture backend onto Home', async ({ page }) => {
  // `/` is Home now, and it no longer redirects (ADR-0056 §1).
  await page.goto('/')
  await expect(page).toHaveURL(/\/(\?|$)/)
  await expect(page.getByTestId('home')).toBeVisible()
  await expect(page.getByTestId('home-standing')).toBeVisible()
})

test('the shelf is one click from Home, at its resting scope', async ({ page }) => {
  await page.goto('/')
  await page.getByTestId('nav-estate').click()
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

test('all five Workspaces are navigable', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('home')).toBeVisible()
  await page.getByTestId('nav-topology').click()
  await expect(page.getByTestId('topology-canvas')).toBeVisible()
  await page.getByTestId('nav-compose').click()
  await expect(page.getByTestId('blueprint-data-flow/gateway-standard')).toBeVisible()
  await page.getByTestId('nav-catalogue').click()
  await expect(page.getByTestId('catalogue-table')).toBeVisible()
  await page.getByTestId('nav-estate').click()
  await expect(page.getByTestId('shelf')).toBeVisible()
  await page.getByTestId('nav-home').click()
  await expect(page.getByTestId('home')).toBeVisible()
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
  ).toHaveCount(2)
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

// Issue #97: a who-acts control is one primitive wherever it appears, the
// secondary Button, worn by a router Link, so the plate says "a door" and
// the element stays an anchor (ADR-0048 §1).
test('every who-acts control is the same primitive, and still an anchor', async ({ page }) => {
  const controls: { url: string; testId: string }[] = [
    // A finding's routing target (ADR-0042 §3.3).
    {
      url: '/estate?object=tier%3Adata-flow%2Fgateway',
      testId: 'who-acts-gateway-expectation-logs',
    },
    // The way out of the one hard block (ADR-0022 §3).
    { url: '/compose?object=blueprint%3Adata-flow%2Fgateway-standard', testId: 'request-grant' },
    // A narrowed-out palette row's door to the request flow.
    { url: '/catalogue?view=palette', testId: 'request-grant-exporter/debug' },
    // The entry panel's question, answered on another view.
    { url: '/catalogue?object=entry%3Aexporter%2Fotlphttp', testId: 'entry-see-palette' },
  ]
  for (const control of controls) {
    await page.goto(control.url)
    const element = page.getByTestId(control.testId)
    await expect(element).toHaveClass(/\bbutton\b/)
    await expect(element).toHaveClass(/\bbutton-secondary\b/)
    await expect(element).toHaveClass(/\bwho-acts\b/)
    await expect(element).toHaveJSProperty('tagName', 'A')
  }

  // The "why?" popover's travel action is the fifth, and reached by asking
  // (ADR-0042 §5).
  await page.goto('/estate?object=tier%3Adata-flow%2Fgateway')
  await page.getByTestId('why-band:conformance').click()
  const trace = page.getByTestId('why-trace-band:conformance')
  await expect(trace).toHaveClass(/\bbutton-secondary\b/)
  await expect(trace).toHaveJSProperty('tagName', 'A')

  // Two links carried the class and were never who-acts controls: a
  // Component's entry in a data cell, and an entry's instances in a list.
  // Both inspect rather than act, so both stay bare anchors.
  await page.goto('/catalogue')
  const entry = page.getByTestId('component-entry-data-flow/gateway-exporter')
  await expect(entry).not.toHaveClass(/\bbutton\b/)
  await expect(entry).not.toHaveClass(/\bwho-acts\b/)
  await entry.click()
  const instance = page.getByTestId('entry-instance-data-flow/gateway-exporter')
  await expect(instance).not.toHaveClass(/\bbutton\b/)
  await expect(instance).not.toHaveClass(/\bwho-acts\b/)
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
