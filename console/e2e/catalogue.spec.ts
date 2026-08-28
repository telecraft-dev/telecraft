import { expect, test } from '@playwright/test'

// The issue #30 acceptance criteria, end to end: catalogue browsing by
// version with stability and signal filters (ADR-0020), the effective
// palette with provenance resolving to the Grant or ancestor Allow-list
// (ADR-0021), Allow-list and Grant edits exiting as PRs via the forge
// adapter (ADR-0042 §6), and composer palette items deep-linking to their
// Catalogue entries.

test('catalogue browsing consults the retained version asked for', async ({ page }) => {
  await page.goto('/catalogue')
  const rows = page.getByTestId('entries-table').locator('tbody tr')
  // The active catalogue leads: v0.158.0, where the connector carries its
  // renamed key with the resolving alias visible.
  await expect(page.getByTestId('version-select')).toHaveValue('v0.158.0')
  await expect(rows).toHaveCount(13)
  await expect(page.getByTestId('entry-connector/span_metrics')).toContainText('was spanmetrics')
  // Installed catalogues are retained, never replaced (ADR-0020 §9): the
  // earlier version still answers, under its old reality.
  await page.getByTestId('version-select').selectOption('v0.155.0')
  await expect(page).toHaveURL(/version=v0\.155\.0/)
  await expect(rows).toHaveCount(12)
  await expect(page.getByTestId('entry-connector/spanmetrics')).toBeVisible()
  await expect(page.getByTestId('entry-connector/span_metrics')).toHaveCount(0)
})

test('stability and signal filters narrow the browse, alone and together', async ({ page }) => {
  await page.goto('/catalogue')
  const rows = page.getByTestId('entries-table').locator('tbody tr')
  await page.getByTestId('filter-stability').selectOption('deprecated')
  await expect(rows).toHaveCount(1)
  await expect(page.getByTestId('entry-exporter/opencensus')).toBeVisible()
  await page.getByTestId('filter-stability').selectOption('')
  await page.getByTestId('filter-signal').selectOption('logs')
  await expect(page.getByTestId('entry-receiver/filelog')).toBeVisible()
  await expect(page.getByTestId('entry-connector/span_metrics')).toHaveCount(0)
  // Together: the named signal at the named level.
  await page.getByTestId('filter-stability').selectOption('stable')
  await expect(rows).toHaveCount(1)
  await expect(page.getByTestId('entry-exporter/otlphttp')).toBeVisible()
  // The filtered state is URL-addressable (ADR-0042 §3.5).
  await expect(page).toHaveURL(/stability=stable/)
  await expect(page).toHaveURL(/signal=logs/)
})

test('the name filter narrows by type or display name and lands in the URL', async ({ page }) => {
  await page.goto('/catalogue')
  const rows = page.getByTestId('entries-table').locator('tbody tr')
  await page.getByTestId('filter-name').fill('kafka')
  await expect(rows).toHaveCount(2)
  await expect(page.getByTestId('entry-receiver/kafka')).toBeVisible()
  await expect(page.getByTestId('entry-exporter/kafka')).toBeVisible()
  // The filtered state is URL-addressable (ADR-0042 §3.5).
  await expect(page).toHaveURL(/name=kafka/)
  // Case falls away, and the display name matches as well as the type:
  // "file log" is the display name of the filelog receiver.
  await page.getByTestId('filter-name').fill('File log')
  await expect(rows).toHaveCount(1)
  await expect(page.getByTestId('entry-receiver/filelog')).toBeVisible()
  await page.getByTestId('filter-name').fill('')
  await expect(rows).toHaveCount(13)
})

test('uniform stability collapses to one chip; mixed and single-signal rows keep their own', async ({
  page,
}) => {
  await page.goto('/catalogue')
  // Every signal at one level reads as one chip.
  await expect(page.getByTestId('entry-processor/batch')).toContainText('all signals: beta')
  await expect(page.getByTestId('entry-processor/batch')).not.toContainText('logs:')
  // Mixed levels keep the chip per signal.
  const otlp = page.getByTestId('entry-receiver/otlp')
  await expect(otlp).toContainText('logs: beta')
  await expect(otlp).toContainText('metrics: stable')
  await expect(otlp).toContainText('traces: stable')
  // A single-signal entry keeps its named chip.
  const filelog = page.getByTestId('entry-receiver/filelog')
  await expect(filelog).toContainText('logs: beta')
  await expect(filelog).not.toContainText('all signals')
})

test('an entry panel carries per-signal stability and the deprecation remediation', async ({
  page,
}) => {
  await page.goto('/catalogue')
  await page.getByTestId('entry-exporter/opencensus').click()
  await expect(page).toHaveURL(/object=entry%3Aexporter%2Fopencensus/)
  const panel = page.getByTestId('entry-panel')
  await expect(panel).toBeVisible()
  await expect(page.getByTestId('entry-panel-title')).toHaveText('OpenCensus')
  // The upstream notice is the remediation text (ADR-0020 §1).
  await expect(page.getByTestId('deprecation-traces')).toContainText('Export OTLP instead')
})

test('governed Components and Catalogue entries deep-link both ways', async ({ page }) => {
  await page.goto('/catalogue')
  await page.getByTestId('component-entry-data-flow/gateway-exporter').click()
  await expect(page).toHaveURL(/object=entry%3Aexporter%2Fotlphttp/)
  await expect(page.getByTestId('entry-panel-title')).toHaveText('OTLP/HTTP')
  // ...and the entry lists its governed instances, linking back.
  await page.getByTestId('entry-instance-data-flow/gateway-exporter').click()
  await expect(page).toHaveURL(/object=component%3Adata-flow%2Fgateway-exporter/)
  await expect(page.getByTestId('component-data-flow/gateway-exporter')).toHaveClass(/selected/)
})

test('the effective palette resolves provenance to the Grant or ancestor Allow-list', async ({
  page,
}) => {
  // The resting team is the signed-in user's: data-flow (ADR-0042 §2).
  await page.goto('/catalogue?view=palette')
  await expect(page.getByTestId('palette-summary')).toContainText('8 of 13')
  // A Grant admits the Kafka exporter that both lists exclude: union
  // after intersection, overriding the target's own declared list.
  await expect(page.getByTestId('origin-exporter/kafka')).toHaveText('Grant')
  await page.getByTestId('palette-why-exporter/kafka').click()
  const popover = page.getByTestId('palette-popover')
  await expect(popover).toContainText('kafka-egress-for-data-flow')
  await expect(popover).toContainText('Granted by platform to data-flow')
  // A list-admitted component names the ancestor chain it survived.
  await expect(page.getByTestId('origin-processor/batch')).toHaveText('Allow-list')
  await page.getByTestId('palette-why-processor/batch').click()
  await expect(popover).toContainText('Allowed by every Allow-list declared above this team')
  await expect(popover).toContainText('platform')
  // A narrowed-out component says which list removed it.
  await expect(page.getByTestId('origin-exporter/debug')).toHaveText('not allowed')
  await page.getByTestId('palette-why-exporter/debug').click()
  await expect(popover).toContainText('Narrowed out by the data-flow Allow-list')
  // A team with no list on its chain gets the default posture (§4).
  await page.getByTestId('palette-team').selectOption('product')
  await expect(page).toHaveURL(/team=product/)
  await expect(page.getByTestId('origin-receiver/otlp')).toHaveText('default allow')
  await expect(page.getByTestId('palette-summary')).toContainText('13 of 13')
})

test('a non-allowed palette row requests a Grant and the edit exits as a PR', async ({
  page,
}) => {
  await page.goto('/catalogue?view=palette')
  await page.getByTestId('request-grant-exporter/debug').click()
  // The request lands in the governance view, draft prefilled, state in
  // the URL (ADR-0042 §3.5).
  await expect(page).toHaveURL(/view=governance/)
  await expect(page).toHaveURL(/request=exporter%2Fdebug/)
  await expect(page.getByTestId('grant-adds')).toHaveValue('exporter/debug')
  await expect(page.getByTestId('grant-team')).toHaveValue('data-flow')
  await expect(page.getByTestId('grant-id')).toHaveValue('exporter-debug-for-data-flow')
  // A Grant is parent-authored: the platform lead signs it (ADR-0021 §3).
  await page.getByTestId('grant-owner').selectOption('platform-lead')
  await page.getByTestId('propose').click()
  await expect(page.getByTestId('proposal-opened')).toBeVisible()
  await expect(page.getByTestId('proposal-url')).toHaveAttribute('href', /\/pull\/\d+/)
})

test('an Allow-list edit exits as a PR via the forge adapter', async ({ page }) => {
  await page.goto('/catalogue?view=governance')
  const entries = page.getByTestId('allowlist-entries-data-flow')
  const current = await entries.inputValue()
  await entries.fill(`${current}\nexporter/debug`)
  await page.getByTestId('propose').click()
  await expect(page.getByTestId('proposal-opened')).toBeVisible()
  await expect(page.getByTestId('proposal-url')).toHaveAttribute('href', /\/pull\/\d+/)
})

test('a refused governance edit comes back with the load problems named', async ({ page }) => {
  await page.goto('/catalogue?view=governance')
  // An emptied list would ban everything: refused fail-closed, exactly as
  // loading refuses it (ADR-0021 §4), so no proposal opens.
  await page.getByTestId('allowlist-entries-data-flow').fill('')
  await page.getByTestId('propose').click()
  await expect(page.getByTestId('proposal-problems')).toBeVisible()
  await expect(page.getByTestId('proposal-problems')).toContainText('declares no entries')
  await expect(page.getByTestId('proposal-opened')).toHaveCount(0)
})

test('composer palette items deep-link to their Catalogue entries', async ({ page }) => {
  await page.goto('/compose?object=blueprint%3Adata-flow%2Fgateway-standard')
  await page.getByTestId('lane-entry-traces-data-flow/gateway-exporter@2').click()
  await expect(page).toHaveURL(/\/catalogue\?.*object=entry%3Aexporter%2Fotlphttp/)
  await expect(page.getByTestId('entry-panel-title')).toHaveText('OTLP/HTTP')
  await expect(page.getByTestId('entry-exporter/otlphttp')).toHaveClass(/selected/)
})

test('jump-to-object reaches a Catalogue entry', async ({ page }) => {
  await page.goto('/estate')
  await page.getByTestId('jump-trigger').click()
  await page.getByTestId('jump-input').fill('span metrics')
  await page.getByTestId('jump-result-entry-connector/span_metrics').click()
  await expect(page).toHaveURL(/\/catalogue\?.*object=entry%3Aconnector%2Fspan_metrics/)
  await expect(page.getByTestId('entry-panel')).toBeVisible()
})
