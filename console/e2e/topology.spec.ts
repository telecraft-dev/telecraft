import { expect, test } from '@playwright/test'

// The issue #28 acceptance criteria, end to end: the P3-scale fixture
// renders selector-matched counts over zero per-collector nodes, edges
// derive from the model with no gesture that draws one, drag is
// row-constrained and per-user persisted while simulate persists nothing,
// multiple Paths per Service render distinctly, and the universal card
// panel and its deep links work from canvas nodes.

const CANVAS = 'topology-canvas'

test('the P3-scale topology renders counts and zero per-collector nodes', async ({ page }) => {
  await page.goto('/topology')
  const canvas = page.getByTestId(CANVAS)
  // ~22k matched collectors, five authored nodes: 4 Tiers + 1 ungoverned
  // source. Scale lives in the counts only (ADR-0007).
  await expect(canvas.locator('.canvas-node')).toHaveCount(5)
  await expect(page.getByTestId('node-collectors-data-flow/edge')).toHaveText('21,614 matched')
  // The served/git split is visible per Tier (delivery path is a visible
  // collector property, ADR-0007).
  await expect(canvas.locator('.canvas-node', { hasText: '21,608 served · 6 git' })).toBeVisible()
})

test('edges derive from the model; no gesture draws or redraws one', async ({ page }) => {
  await page.goto('/topology')
  const canvas = page.getByTestId(CANVAS)
  // Every drawn edge is a Hop signal lane: 3 Hops carrying 3+3+2 signals.
  await expect(canvas.locator('.canvas-edge')).toHaveCount(8)
  // Nothing is connectable: there is no handle a drag could start an
  // edge from (ADR-0044 §3).
  await expect(canvas.locator('.react-flow__handle.connectable')).toHaveCount(0)
  // A drag gesture across the canvas rearranges at most; it never adds
  // an edge.
  const gateway = canvas.locator('.react-flow__node[data-id="data-flow/gateway"]')
  const box = (await gateway.boundingBox())!
  await page.mouse.move(box.x + box.width / 2, box.y + 10)
  await page.mouse.down()
  await page.mouse.move(box.x + box.width / 2 + 80, box.y + 10, { steps: 6 })
  await page.mouse.up()
  await expect(canvas.locator('.canvas-edge')).toHaveCount(8)
})

test('drag is row-constrained and persists in the per-user store', async ({ page }) => {
  await page.goto('/topology')
  const canvas = page.getByTestId(CANVAS)
  const edge = canvas.locator('.react-flow__node[data-id="data-flow/edge"]')
  const gateway = canvas.locator('.react-flow__node[data-id="data-flow/gateway"]')
  const edgeBefore = (await edge.boundingBox())!
  const before = (await gateway.boundingBox())!
  const gapBefore = before.x - edgeBefore.x
  // Drag right and down: only the horizontal component may land — a node
  // can never leave its Environment row (ADR-0044 §3).
  await page.mouse.move(before.x + before.width / 2, before.y + 10)
  await page.mouse.down()
  await page.mouse.move(before.x + before.width / 2 + 120, before.y + 10 + 80, { steps: 8 })
  await page.mouse.up()
  const after = (await gateway.boundingBox())!
  expect(after.x - before.x).toBeGreaterThan(60)
  expect(Math.abs(after.y - before.y)).toBeLessThan(2)
  // The within-row offset lands in the presentation store, per user
  // (ADR-0042 §7) — presentation only, never model truth.
  const stored = await page.evaluate(() => {
    const raw = window.localStorage.getItem('telecraft.console.presentation.v1.demo-user')
    return raw === null ? null : (JSON.parse(raw) as { arrangement?: Record<string, Record<string, number>> })
  })
  expect(stored?.arrangement?.['topology']?.['data-flow/gateway']).toBeGreaterThan(50)
  // A fresh load re-derives the layout with the persisted arrangement.
  await page.reload()
  const edgeReloaded = (await edge.boundingBox())!
  const gatewayReloaded = (await gateway.boundingBox())!
  expect(gatewayReloaded.x - edgeReloaded.x).toBeGreaterThan(gapBefore + 40)
})

test('simulate animates journeys and changes nothing persistent', async ({ page }) => {
  await page.goto('/topology')
  const storeBefore = await page.evaluate(() =>
    window.localStorage.getItem('telecraft.console.presentation.v1.demo-user'),
  )
  await page.getByTestId('simulate-toggle').click()
  // Per-journey dots born at a receiver, traversing the full chain,
  // signal groups staggered (ADR-0044 §5).
  await expect(page.locator('.journey-dot').first()).toBeVisible()
  // Cosmetic only: no URL state, no store state.
  expect(page.url()).not.toContain('simulate')
  const storeAfter = await page.evaluate(() =>
    window.localStorage.getItem('telecraft.console.presentation.v1.demo-user'),
  )
  expect(storeAfter).toBe(storeBefore)
  await page.getByTestId('simulate-toggle').click()
  await expect(page.locator('.journey-dot')).toHaveCount(0)
  await page.reload()
  await expect(page.getByTestId(CANVAS)).toBeVisible()
  await expect(page.locator('.journey-dot')).toHaveCount(0)
})

test('multiple Paths per Service render distinctly', async ({ page }) => {
  // checkout: an edge-chain Path and a gateway on-ramp Path (no collector
  // at all, ADR-0007) — each with its own overlay identity.
  await page.goto('/topology?object=service%3Aproduct%2Fcheckout')
  await expect(page.getByTestId('trace-path-0')).toHaveText('edge → gateway')
  await expect(page.getByTestId('trace-path-1')).toHaveText('straight to gateway-staging')
  const canvas = page.getByTestId(CANVAS)
  await expect(canvas.locator('.canvas-edge.trace-overlay.trace-path-0')).toHaveCount(1)
  // The single-Tier Path has no Hop to overlay: its distinct colour lives
  // on the Tier it lands on.
  await expect(
    canvas.locator('[data-id="data-flow/gateway-staging"] .canvas-node.on-trace.trace-path-1'),
  ).toBeVisible()
  // Tracing dims everything not on the Paths (P3): storefront-edge.
  await expect(canvas.locator('.canvas-node.kind-tier.dimmed')).toHaveCount(1)
  // The view-switcher rule in miniature: tracing another Service swaps
  // the overlays, still URL-addressable.
  await page.getByTestId('trace-product/storefront').click()
  await expect(page).toHaveURL(/object=service%3Aproduct%2Fstorefront/)
  await expect(page.getByTestId('trace-path-0')).toHaveText('storefront-edge → gateway')
  await expect(page.getByTestId('trace-path-1')).toHaveText('straight to gateway')
})

test('the universal card panel and deep links work from canvas nodes', async ({ page }) => {
  await page.goto('/topology')
  const canvas = page.getByTestId(CANVAS)
  // Clicking a Tier summons the universal card panel in place — the same
  // component as the shelf, inspection never navigates (ADR-0042 §3.2).
  await canvas.locator('[data-id="data-flow/gateway"] .canvas-node-name').click()
  await expect(page).toHaveURL(/\/topology\?.*object=tier%3Adata-flow%2Fgateway/)
  await expect(page.getByTestId('card-panel')).toBeVisible()
  await expect(page.getByTestId('panel-title')).toHaveText('gateway')
  // One face payload, many representations (ADR-0041 §4): the flow
  // readings the shelf shows are the same readings here, from the same
  // contract — volume, freshness and shape per signal lane.
  const flow = page.getByTestId('panel-flow')
  await expect(flow.getByTestId('matrix-data-flow/gateway-traces')).toContainText('1M → 100k')
  await expect(flow.getByTestId('matrix-data-flow/gateway-traces')).toContainText('30s')
  await expect(flow.getByTestId('matrix-data-flow/gateway-metrics')).toContainText('1 of 2 missing')
  // A who-acts chip in that panel still travels to the fixing surface
  // (ADR-0042 §3.3).
  await expect(page.getByTestId('who-acts-gateway-expectation-logs')).toBeVisible()
  // The matched count on the canvas node is a door to the flat list,
  // pre-filtered (ADR-0042 §3.4).
  await page.getByTestId('node-collectors-data-flow/gateway').click()
  await expect(page).toHaveURL(/\/estate\?.*view=list/)
  await expect(page).toHaveURL(/tier=data-flow%2Fgateway/)
  await expect(page.getByTestId('collector-table')).toBeVisible()
})
