import { expect, test } from '@playwright/test'

// The Blueprints browse view (ADR-0061 §3), end to end: the discovery
// scenario ("I run Kubernetes, production, C1; which configuration?")
// filters the list from the URL, endorsed-only narrows to organisation-
// backed Blueprints, a stale Endorsement pin stays visible and says so
// (ADR-0061 §2), and each entry's two doors land in the composer and in
// the Tier-first flow (ADR-0060 §1).

const BROWSE = '/compose?browse=true'
const SCENARIO = `${BROWSE}&substrate=kubernetes&env=production&serviceClass=C1`

test('the scenario filters keep exactly the Blueprints declared to fit', async ({ page }) => {
  await page.goto(SCENARIO)
  await expect(page.getByTestId('blueprint-entry-data-flow/gateway-standard')).toBeVisible()
  await expect(page.getByTestId('blueprint-entry-data-flow/edge-standard')).toBeVisible()
  await expect(page.locator('[data-testid^="blueprint-entry-"]')).toHaveCount(2)
})

test('endorsed-only narrows the scenario to the endorsed Blueprint', async ({ page }) => {
  await page.goto(`${SCENARIO}&endorsed=true`)
  await expect(page.getByTestId('blueprint-entry-data-flow/gateway-standard')).toBeVisible()
  await expect(page.getByTestId('blueprint-entry-data-flow/edge-standard')).toHaveCount(0)
  await expect(page.locator('[data-testid^="blueprint-entry-"]')).toHaveCount(1)
})

test('a current Endorsement reads endorsed; a stale pin names its version', async ({ page }) => {
  await page.goto(BROWSE)
  await expect(page.getByTestId('endorsed-data-flow/gateway-standard')).toHaveText('endorsed')
  // The pin behind the current version stays visible and says so
  // (ADR-0061 §2): the words carry the state, never the tone alone.
  await expect(page.getByTestId('endorsed-infosec/audit-standard')).toHaveText('endorsed at v1')
})

test('the add-a-Tier door lands on the Estate with the Blueprint version pre-filled', async ({
  page,
}) => {
  await page.goto(BROWSE)
  await page.getByTestId('add-tier-data-flow/gateway-standard').click()
  await page.waitForURL(/\/estate/)
  const url = decodeURIComponent(page.url())
  expect(url).toContain('/estate')
  expect(url).toContain('add=true')
  expect(url).toContain('blueprint=data-flow/gateway-standard@4')
})

test('the open-in-the-composer door lands back in the composer with the doc open', async ({
  page,
}) => {
  await page.goto(BROWSE)
  await page.getByTestId('open-composer-data-flow/gateway-standard').click()
  await expect(page.getByTestId('compose-detail')).toBeVisible()
  await expect(page.getByTestId('compose-detail')).toContainText('gateway-standard')
  expect(decodeURIComponent(page.url())).toContain('object=blueprint:data-flow/gateway-standard')
})

test('the compose list carries the browse door', async ({ page }) => {
  await page.goto('/compose')
  await page.getByTestId('browse-blueprints').click()
  await expect(page.getByTestId('blueprint-browse')).toBeVisible()
  await expect(page).toHaveURL(/browse=true/)
})
