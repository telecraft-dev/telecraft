import { expect, test } from '@playwright/test'

// The issue #27 acceptance criteria, end to end: the drawer's findings
// with who-acts chips that travel to the fixing surface, collector counts
// as doors to the flat list, the environment lens as emphasis and
// evaluation context (never a filter), the roll-up with waived counts
// visible, and the "why?" provenance popover with its trace action.

const GATEWAY = 'object=tier%3Adata-flow%2Fgateway'

test('the card panel opens in place with the drawer findings, waivers visible', async ({
  page,
}) => {
  await page.goto(`/estate?${GATEWAY}`)
  await expect(page.getByTestId('card-panel')).toBeVisible()
  await expect(page.getByTestId('panel-title')).toHaveText('gateway')
  // The drawer is fetched on demand and lists every finding with its
  // mandatory remediation (ADR-0041 §3).
  await expect(page.getByTestId('panel-findings').locator('.finding')).toHaveCount(3)
  // A waiver waives the count, never the diagnosis (ADR-0037): the waived
  // finding stays listed, its dampening state visible.
  await expect(page.getByTestId('dampening-gateway-grant-redaction')).toHaveText('waived')
})

test('a Blueprint-shaped who-acts chip travels to Compose at the offending lane', async ({
  page,
}) => {
  await page.goto(`/estate?${GATEWAY}`)
  await page.getByTestId('who-acts-gateway-expectation-logs').click()
  await expect(page).toHaveURL(/\/compose\?.*lane=logs/)
  await expect(page).toHaveURL(/object=blueprint%3Adata-flow%2Fgateway-standard/)
  await expect(page.getByTestId('lane-logs')).toHaveClass(/offending/)
})

test('a grant-shaped who-acts chip travels to Governance', async ({ page }) => {
  await page.goto(`/estate?${GATEWAY}`)
  await page.getByTestId('who-acts-gateway-grant-redaction').click()
  await expect(page).toHaveURL(/\/catalogue\?.*object=component%3Ainfosec%2Fpii-redaction/)
  await expect(page.getByTestId('component-infosec/pii-redaction')).toHaveClass(/selected/)
})

test('a collector count is a door to the flat list, pre-filtered and URL-addressable', async ({
  page,
}) => {
  await page.goto('/estate')
  await page.getByTestId('card-collectors-data-flow/gateway').click()
  await expect(page).toHaveURL(/view=list/)
  await expect(page).toHaveURL(/tier=data-flow%2Fgateway/)
  await expect(page.getByTestId('collector-table').locator('tbody tr')).toHaveCount(3)
  // The same state loads fresh from its URL (ADR-0042 §3.5).
  await page.goto('/estate?view=list&tier=data-flow%2Fgateway')
  await expect(page.getByTestId('collector-gw-0')).toBeVisible()
  await expect(page.getByTestId('collector-table').locator('tbody tr')).toHaveCount(3)
  // Widening the filter shows every collector: the flat list is the only
  // home of per-collector detail (ADR-0042 §3.4).
  await page.getByTestId('filter-tier').selectOption('')
  await expect(page.getByTestId('collector-table').locator('tbody tr')).toHaveCount(10)
})

test('the lens changes emphasis and evaluation context without removing rows', async ({
  page,
}) => {
  await page.goto('/estate?lens=production')
  const rows = page.locator('.environment-row')
  const before = await rows.count()
  await expect(
    page.getByTestId('section-data-flow').locator('.environment-row').first(),
  ).toHaveAttribute('data-environment', 'production')
  await page.getByTestId('lens-control').selectOption('staging')
  // Emphasis moved: staging leads and carries the emphasis class...
  const first = page.getByTestId('section-data-flow').locator('.environment-row').first()
  await expect(first).toHaveAttribute('data-environment', 'staging')
  await expect(first).toHaveClass(/lens-leading/)
  // ...and nothing was filtered out.
  await expect(rows).toHaveCount(before)
})

test('the roll-up shows ratio-plus-worst per kind with waived counts visible', async ({
  page,
}) => {
  await page.goto('/estate?view=rollup&lens=production')
  const dataFlow = page.getByTestId('rollup-data-flow')
  await expect(dataFlow.locator('[data-kind="conformance"] .rollup-ratio')).toHaveText('1/2')
  await expect(dataFlow.locator('[data-kind="conformance"] .rollup-waived')).toHaveText(
    '1 waived',
  )
  // Waived counts ride every roll-up level (ADR-0017, ADR-0037).
  await expect(
    page.getByTestId('rollup-engineering').locator('[data-kind="conformance"] .rollup-waived'),
  ).toHaveText('1 waived')
  // The lens is the evaluation context: switching it re-judges the ratios
  // but removes no team row.
  const teamRows = page.getByTestId('rollup-table').locator('tbody tr')
  const before = await teamRows.count()
  await page.getByTestId('lens-control').selectOption('staging')
  await expect(page.getByTestId('rollup-lens')).toHaveText('staging')
  await expect(teamRows).toHaveCount(before)
  await expect(
    page.getByTestId('rollup-all-data-flow'),
  ).toHaveText('2 findings, 1 waived')
})

test('the "why?" popover shows provenance and its trace action lights the canvas', async ({
  page,
}) => {
  await page.goto(`/estate?${GATEWAY}`)
  await page.getByTestId('why-band:conformance').click()
  const popover = page.getByTestId('why-popover')
  await expect(popover).toBeVisible()
  // Claim, implying config lines, judged SHA (ADR-0041 §3, ADR-0042 §5).
  await expect(popover.locator('.why-claim')).toContainText('C1 floor')
  await expect(popover.locator('.why-file').first()).toHaveText('services/checkout.yaml:4')
  await expect(popover.locator('.why-sha')).toContainText('9c2f4e1')
  // The spatial derivation travels: trace the Service's Paths (rule 3.3).
  await page.getByTestId('why-trace-band:conformance').click()
  await expect(page).toHaveURL(/\/topology\?.*object=service%3Aproduct%2Fcheckout/)
  await expect(
    page.getByTestId('topology-canvas').locator('.canvas-node.kind-tier.dimmed'),
  ).toHaveCount(1)
})
