import { expect, test } from '@playwright/test'

// The issue #31 acceptance criteria, end to end: ungoverned collectors in
// view with the onboard CTA and outside every compliance denominator
// (ADR-0031); the claim flow herd-first over a flat-list multi-select,
// its suggested selector generalised over shared identity attributes and
// never an enumeration of instance ids (ADR-0042 §6); completion a
// user-attributed PR authoring the Tier binding (ADR-0014, ADR-0028):
// the console proposes, the PR decides, and after merge and serve the
// collectors read as governed. Collectors served the Unmatched artefact
// (ADR-0030) enter the same flow as foreign ones.

test('ungoverned collectors surface with the onboard CTA, outside every denominator', async ({
  page,
}) => {
  await page.goto('/estate')
  // The dedicated band above governed Tiers (ADR-0031 §2): counts split by
  // referent, concern phrased as concern, never failure.
  const band = page.getByTestId('ungoverned-band')
  await expect(band).toBeVisible()
  await expect(band).toContainText('5 ungoverned collectors')
  await expect(band).toContainText('3 served the Unmatched artefact')
  await expect(band).toContainText('2 foreign')
  // Excluded from compliance denominators: the roll-up's ratios are the
  // governed Tiers' alone: the same numbers as before ungoverned rows
  // existed (P2's denominator discipline, ADR-0017/0031).
  await page.goto('/estate?view=rollup&lens=production')
  const dataFlow = page.getByTestId('rollup-data-flow')
  await expect(dataFlow.locator('[data-kind="conformance"] .rollup-ratio')).toHaveText('1/2')
  // The CTA is a door to the flat list, pre-filtered to the ungoverned.
  await page.goto('/estate')
  await page.getByTestId('onboard-cta').click()
  await expect(page).toHaveURL(/view=list/)
  await expect(page).toHaveURL(/ungoverned=true/)
  await expect(page.getByTestId('collector-table').locator('tbody tr')).toHaveCount(5)
})

test('the flow is herd-first and the selector generalises, never enumerates', async ({
  page,
}) => {
  await page.goto('/estate?view=list&ungoverned=true')
  // Multi-select the served herd: the flow operates on the selection.
  await page.getByTestId('herd-pay-edge-7f3a').check()
  await page.getByTestId('herd-pay-edge-b41c').check()
  await page.getByTestId('herd-pay-edge-e9d0').check()
  const panel = page.getByTestId('claim-panel')
  await expect(panel).toBeVisible()
  await expect(page.getByTestId('claim-title')).toHaveText('Claim 3 collectors')
  // The suggested selector generalises over shared identity attributes…
  const selector = page.getByTestId('claim-selector')
  await expect(selector).toContainText('k8s.namespace=payments')
  await expect(selector).toContainText('telecraft.tier=edge')
  // …and never enumerates: no instance id appears, and no gesture offers one.
  await expect(selector).not.toContainText('service.instance.id')
  await expect(selector).not.toContainText('pay-edge')
  await expect(panel.getByTestId('claim-term-service.instance.id')).toHaveCount(0)
  // The impact preview is exact over the ungoverned population.
  await expect(page.getByTestId('claim-matched')).toContainText('Matches 3 ungoverned collectors')
  // Constraining only removes pairs; widening past a governed population
  // surfaces the blast radius instead of hiding it (ADR-0028).
  await page.getByTestId('claim-term-k8s.namespace').uncheck()
  await expect(page.getByTestId('claim-overlaps')).toContainText('product/storefront-edge')
  await page.getByTestId('claim-term-k8s.namespace').check()
  await expect(page.getByTestId('claim-overlaps')).toHaveCount(0)
})

test('attach exits as a user-attributed PR widening the closest candidate Tier', async ({
  page,
}) => {
  await page.goto('/estate?view=list&ungoverned=true&herd=pay-edge-7f3a%2Cpay-edge-b41c%2Cpay-edge-e9d0')
  await page.getByTestId('claim-path-attach').check()
  // Candidates rank by selector proximity: data-flow/edge shares two pairs.
  const candidates = page.getByTestId('claim-candidates').locator('li')
  await expect(candidates.first()).toContainText('data-flow/edge')
  await expect(candidates.first()).toContainText('satisfies 2 of 3')
  await page.getByTestId('claim-candidate-data-flow/edge').check()
  // The rendered impact preview shows the widened binding the PR carries.
  await expect(page.getByTestId('claim-rendered')).toContainText('selector widened by the claim')
  await page.getByTestId('claim-propose').click()
  const opened = page.getByTestId('claim-opened')
  await expect(opened).toBeVisible()
  await expect(opened).toContainText('claim/data-flow/edge')
  await expect(page.getByTestId('claim-attribution')).toContainText('Demo user')
  await expect(page.getByTestId('claim-proposal-url')).toHaveAttribute(
    'href',
    /forge\.example/,
  )
})

test('draft opens Compose with the selector pre-filled and proposes the Tier binding', async ({
  page,
}) => {
  await page.goto('/estate?view=list&ungoverned=true&herd=pay-edge-7f3a%2Cpay-edge-b41c')
  await page.getByTestId('claim-path-draft').check()
  await page.getByTestId('claim-tier-name').fill('payments-edge')
  // The drafted binding renders before the handoff: the impact preview
  // rides the whole flow (ADR-0042 §6).
  await expect(page.getByTestId('claim-rendered')).toContainText(
    'teams/data-flow/tiers/payments-edge.yaml',
  )
  await page.getByTestId('claim-draft').click()
  // Compose opens on a fresh draft bound to the new Tier, selector
  // pre-filled: generalised, never a list of instance ids.
  await expect(page).toHaveURL(/\/compose\?/)
  await expect(page).toHaveURL(/tier=data-flow%2Fpayments-edge/)
  const banner = page.getByTestId('claim-banner')
  await expect(banner).toBeVisible()
  await expect(page.getByTestId('claim-banner-selector')).toContainText('k8s.namespace=payments')
  await expect(page.getByTestId('claim-banner-selector')).not.toContainText('pay-edge')
  // The exit is the composer's own PR, now authoring the Tier binding too.
  await page.getByTestId('save-button').click()
  await expect(page.getByTestId('proposal')).toBeVisible()
  await expect(page.getByTestId('proposal-branch')).toHaveText('claim/data-flow/payments-edge')
  await expect(page.getByTestId('proposal-attribution')).toContainText('Demo user')
})

test('served-Unmatched and foreign collectors enter the same flow', async ({ page }) => {
  await page.goto('/estate?view=list&ungoverned=true')
  // Both referents carry the same selection affordance (ADR-0031 §1): a
  // collector running the Unmatched artefact and a foreign one join one herd.
  await expect(page.getByTestId('ungoverned-pay-edge-7f3a')).toContainText(
    'served the Unmatched artefact',
  )
  await expect(page.getByTestId('ungoverned-host-watch-a')).toContainText('foreign')
  await page.getByTestId('herd-pay-edge-7f3a').check()
  await page.getByTestId('herd-host-watch-a').check()
  await expect(page.getByTestId('claim-title')).toHaveText('Claim 2 collectors')
  // A mixed herd generalises to what it truly shares, nothing more.
  await expect(page.getByTestId('claim-selector')).toHaveText('deployment.environment=production')
  // Governed rows offer no herd checkbox: the claim flow is for the ungoverned.
  await page.goto('/estate?view=list')
  await expect(page.getByTestId('collector-edge-0').locator('input[type=checkbox]')).toHaveCount(0)
})
