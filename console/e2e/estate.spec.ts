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
  // Widening the filter shows every collector — the ungoverned included,
  // in view and never hidden (ADR-0031): the flat list is the only home
  // of per-collector detail (ADR-0042 §3.4).
  await page.getByTestId('filter-tier').selectOption('')
  await expect(page.getByTestId('collector-table').locator('tbody tr')).toHaveCount(15)
})

test('the lens changes emphasis and evaluation context without removing rows', async ({
  page,
}) => {
  await page.goto('/estate?lens=production')
  // Settle the shelf before taking the baseline count: count() does not
  // auto-wait, and the auth gate's /api/v1/me round trip now precedes the
  // estate fetch, so an unrendered shelf would count zero rows.
  await expect(
    page.getByTestId('section-data-flow').locator('.environment-row').first(),
  ).toHaveAttribute('data-environment', 'production')
  const rows = page.locator('.environment-row')
  const before = await rows.count()
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

// The issue #35 acceptance criteria: the per-signal matrix under the
// reading bands (P4 variant D), the three reds distinguishable without
// hue, and reduction presented rather than judged (ADR-0040, ADR-0041).

test('every card carries the per-signal matrix under its reading bands', async ({ page }) => {
  await page.goto('/estate?scope=estate')

  const matrix = page.getByTestId('matrix-data-flow/gateway')
  await expect(matrix).toBeVisible()
  await expect(matrix.locator('tbody tr')).toHaveCount(3)

  // Volume, freshness and shape on the traces lane, read from the
  // contract rather than computed in the browser.
  const traces = page.getByTestId('matrix-data-flow/gateway-traces')
  await expect(traces).toContainText('1M → 100k')
  await expect(traces).toContainText('90% reduction')
  await expect(traces).toContainText('30s')

  // Reduction is presented and never graded: the row raises no error
  // reading (ADR-0040 §3).
  await expect(page.getByTestId('errors-data-flow/gateway-traces')).toHaveCount(0)

  // The error-rate readings are the meter's only reds, and they are their
  // own cell.
  await expect(page.getByTestId('errors-data-flow/gateway-metrics')).toContainText('100 refused')
})

test('an unread lane is last-known-plus-age, never a metered zero', async ({ page }) => {
  await page.goto('/estate?scope=estate')

  // The staging Tier has reported no self-telemetry at its serving SHA:
  // every lane says so, and none of them says nothing flowed.
  const staging = page.getByTestId('matrix-data-flow/gateway-staging-logs')
  await expect(staging.locator('.cell-volume')).toHaveText('—')
  await expect(staging.locator('.cell-volume')).toHaveAttribute(
    'title',
    /no self-telemetry has reported at the serving SHA yet/,
  )

  // A known-empty lane is a different reading, and reads differently: the
  // gateway's logs lane is silent, not unreadable (ADR-0008).
  const silent = page.getByTestId('matrix-data-flow/gateway-logs')
  await expect(silent.locator('.cell-freshness')).toHaveText('silent')
})

// #98: three lanes that all meter `in 0 / out 0`, and mean three
// different things. The zeros are true; the rendering has to be too.
test('a lane the artefact never wired does not read as a stopped one', async ({ page }) => {
  await page.goto('/estate?scope=estate')

  // edge-standard wires traces and logs and no metrics lane, so the edge
  // Tier's metrics row has no pipeline behind it. No numbers, and the
  // not_applicable mark ADR-0047 §7 gives the state.
  const absent = page.getByTestId('matrix-data-flow/edge-metrics')
  await expect(absent).toHaveClass(/lane-absent/)
  await expect(absent.locator('.cell-lane')).toContainText('no lane on this Tier')
  await expect(absent.locator('.cell-lane [data-mark="not_applicable"]')).toBeVisible()
  await expect(absent.locator('.cell-volume')).toHaveCount(0)

  // The gateway's logs lane is wired and moving nothing: the same meter
  // reading, and a real finding. It keeps its zero.
  const stopped = page.getByTestId('matrix-data-flow/gateway-logs')
  await expect(stopped).not.toHaveClass(/lane-absent/)
  await expect(stopped.locator('.cell-volume')).toContainText('0 → 0')
  await expect(stopped.locator('.cell-lane')).toHaveCount(0)

  // And a lane nobody could read is the third: last-known-plus-age, with
  // the numbers withheld rather than the lane denied.
  const unread = page.getByTestId('matrix-data-flow/gateway-staging-metrics')
  await expect(unread).not.toHaveClass(/lane-absent/)
  await expect(unread.locator('.cell-volume')).toHaveText('—')
})

test('the three reds stay distinct by band position and mark, not by hue', async ({ page }) => {
  await page.goto(`/estate?${GATEWAY}`)

  // Band order is fixed on every card, so position identifies the kind.
  const bands = page.getByTestId('card-data-flow/gateway').locator('.card-bands .band')
  await expect(bands).toHaveCount(3)
  await expect(bands.nth(0)).toContainText('Delivery')
  await expect(bands.nth(1)).toContainText('Expectation')
  await expect(bands.nth(2)).toContainText('Conformance')

  // Expectation-red beside conformance-red: two findings, each named,
  // neither swallowing the other — and delivery reading ok beside them
  // is the differentiator P4 tested (applied, conforming, expected logs
  // never landed).
  await expect(bands.nth(0)).toContainText('ok')
  await expect(bands.nth(1)).toContainText('finding')
  await expect(bands.nth(2)).toContainText('finding')

  // The mark carries the severity where hue only reinforces it. Marks are
  // drawn, not typed, since ADR-0047 §6: asserting the name the mark
  // renders under holds the mapping without holding its geometry.
  await expect(bands.nth(1).locator('.mark')).toHaveAttribute('data-mark', 'advisory')
  await expect(bands.nth(2).locator('.mark')).toHaveAttribute('data-mark', 'violation')
})

test('the four honest neutrals each render their own mark (ADR-0047 §7)', async ({ page }) => {
  await page.goto('/estate')

  // gateway-staging is the fixture's neutral card: pending settle, then
  // unknown, then not applicable. Before this pass all three drew the same
  // glyph, which said they were one situation. They are three.
  const bands = page.getByTestId('card-data-flow/gateway-staging').locator('.card-bands .band')
  await expect(bands.nth(0).locator('.mark')).toHaveAttribute('data-mark', 'pending_settle')
  await expect(bands.nth(1).locator('.mark')).toHaveAttribute('data-mark', 'unknown')
  await expect(bands.nth(2).locator('.mark')).toHaveAttribute('data-mark', 'not_applicable')

  // And each still says which it is, in words.
  await expect(bands.nth(0)).toContainText('pending settle')
  await expect(bands.nth(1)).toContainText('unknown')
  await expect(bands.nth(2)).toContainText('not applicable')
})

test('the card panel shows the flow readings and the restart rate', async ({ page }) => {
  await page.goto(`/estate?${GATEWAY}`)
  const flow = page.getByTestId('panel-flow')
  await expect(flow).toBeVisible()
  await expect(flow).toContainText('Restarts: 4 incarnations')
})
