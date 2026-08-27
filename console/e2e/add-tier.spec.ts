import { expect, test } from '@playwright/test'

// The Add-a-Tier flow end to end (ADR-0060 §1, §2): the Estate header's
// door opens the panel; a complete draft exits as a user-attributed PR
// through the forge adapter (ADR-0028, ADR-0014); a refusal comes back
// fail closed with the problems named, exactly as the loader would refuse
// the authored Tier; and the URL pre-fills seed the fields, so the
// Blueprints view door and the claim flow's draft branch land in one
// implementation, never a fork.

test('the Estate door opens the Add a Tier panel', async ({ page }) => {
  await page.goto('/estate')
  await page.getByTestId('add-tier').click()
  const panel = page.getByTestId('add-tier-panel')
  await expect(panel).toBeVisible()
  await expect(page.getByTestId('add-tier-title')).toHaveText('Add a Tier')
})

test('a complete Tier proposes as a pull request', async ({ page }) => {
  await page.goto('/estate?add=true')
  await page.getByTestId('tier-name').fill('payments-edge')
  // The team defaults to the acting human's; the id preview composes it.
  await expect(page.getByTestId('tier-id-preview')).toHaveText('data-flow/payments-edge')
  await page.getByTestId('tier-owner').selectOption('dataflow-lead')
  await page.getByTestId('tier-blueprint').selectOption('data-flow/gateway-standard@4')
  await page
    .getByTestId('tier-selector')
    .fill('deployment.environment=production\ntelecraft.tier=payments-edge')
  await page.getByTestId('tier-propose').click()
  const opened = page.getByTestId('tier-opened')
  await expect(opened).toBeVisible()
  await expect(page.getByTestId('tier-proposal-url')).toHaveAttribute('href', /\/pull\//)
})

test('a name that collides with an existing Tier is refused with the problem named', async ({
  page,
}) => {
  await page.goto('/estate?add=true')
  // data-flow/payments-gateway already exists on the fixture estate.
  await page.getByTestId('tier-name').fill('payments-gateway')
  await page.getByTestId('tier-owner').selectOption('dataflow-lead')
  await page.getByTestId('tier-blueprint').selectOption('data-flow/gateway-standard@4')
  await page.getByTestId('tier-selector').fill('deployment.environment=production')
  await page.getByTestId('tier-propose').click()
  const problems = page.getByTestId('tier-problems')
  await expect(problems).toBeVisible()
  await expect(problems).toContainText('the Tier data-flow/payments-gateway already exists')
  await expect(page.getByTestId('tier-opened')).toHaveCount(0)
})

test('the URL pre-fills seed the Blueprint and the name', async ({ page }) => {
  await page.goto('/estate?add=true&blueprint=data-flow%2Fgateway-standard%404&name=edge-two')
  await expect(page.getByTestId('tier-blueprint')).toHaveValue('data-flow/gateway-standard@4')
  await expect(page.getByTestId('tier-name')).toHaveValue('edge-two')
})
