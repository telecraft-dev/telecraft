import { expect, test } from '@playwright/test'

// Activation is explicit and audited (ADR-0020 §6): the surface shows which
// version each substrate is judged against, what activating each retained
// version would change, and every activation so far. It offers the control
// to operators and to nobody else, and nothing here applies on its own.

test('the surface shows the active version, what is on offer, and the audit trail', async ({
  page,
}) => {
  await page.goto('/catalogue?view=activation')
  await expect(page.getByTestId('active-catalogue')).toHaveText('Active: v0.158.0')

  // The candidate carries the report the decision would be taken on, and
  // the report names whose Blueprint the change lands on.
  const candidate = page.getByTestId('candidate-v0.155.0')
  await expect(candidate).toContainText('1 component in use is removed')
  await expect(candidate).toContainText('processor/transform is removed')
  await expect(candidate).toContainText('data-flow/gateway-standard (Data flow)')

  // Every activation so far, most recent first, each with who and when.
  const history = page.getByTestId('substrate-catalogue').locator('.activation-record')
  await expect(history).toHaveCount(2)
  await expect(history.first()).toContainText('v0.155.0 to v0.158.0')
  await expect(history.first()).toContainText('engineering-lead')
  await expect(history.first()).toContainText('2026-07-14')
})

// A substrate nothing was imported for says so, rather than showing an
// empty list a reader has to interpret.
test('a substrate with nothing imported says so', async ({ page }) => {
  await page.goto('/catalogue?view=activation')
  const registry = page.getByTestId('substrate-schema_registry')
  await expect(registry.getByTestId('active-schema_registry')).toContainText('No version is active')
  await expect(registry).toContainText('Nothing is installed to activate')
})

// The fixture user sits one team below the root, so the control is
// withheld and the surface says whose it is.
test('activation is withheld from a user who is not an operator', async ({ page }) => {
  await page.goto('/catalogue?view=activation')
  await expect(page.getByTestId('withheld-v0.155.0')).toBeVisible()
  await expect(page.getByTestId('activate-catalogue-v0.155.0')).toHaveCount(0)
})

// The view switcher reaches it, and the position lives in the URL like
// every other console state.
test('the activation view is one of the workspace views', async ({ page }) => {
  await page.goto('/catalogue')
  await expect(page.getByTestId('activation-view')).toHaveCount(0)
  await page.getByTestId('view-activation').click()
  await expect(page).toHaveURL(/view=activation/)
  await expect(page.getByTestId('activation-view')).toBeVisible()
})
