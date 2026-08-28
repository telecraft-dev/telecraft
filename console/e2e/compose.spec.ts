import { expect, test } from '@playwright/test'

// The issue #29 acceptance criteria, end to end: the palette offers only
// the effective Allow-list (hidden, not greyed traps; ADR-0022 §5), edits
// produce Blueprint documents with ordering findings inline (ADR-0024 §6),
// the resident YAML flyout is read-only and click-off closes it (REQ-035),
// Save exits to a PR through the forge adapter with the one hard block
// different in kind (ADR-0028, ADR-0022 §3), and `satisfies` renders as a
// claim linking to the verdict, never a status (REQ-031).

const EDGE = 'object=blueprint%3Adata-flow%2Fedge-standard'
const GATEWAY = 'object=blueprint%3Adata-flow%2Fgateway-standard'

test('the palette shows only the effective palette; disallowed types are absent', async ({
  page,
}) => {
  await page.goto(`/compose?${EDGE}&lens=production`)
  // Allowed entries are present; the Grant-admitted one names its Grant:
  // the audit chain is total (ADR-0021 §3).
  await expect(page.getByTestId('palette-type:processor/batch')).toBeVisible()
  await expect(page.getByTestId('palette-grant-type:receiver/kafka')).toContainText(
    'kafka-egress-for-data-flow',
  )
  // Non-allowed types are hidden entirely (absent, not greyed traps)
  // with the honest admitted count (ADR-0022 §5).
  await expect(page.getByTestId('palette-type:exporter/debug')).toHaveCount(0)
  await expect(page.getByTestId('palette-type:processor/pii_scrub')).toHaveCount(0)
  await expect(page.getByTestId('palette-hidden')).toHaveText(
    '5 components hidden by your allow-list',
  )
})

test('floor greying follows the environment lens as evaluation context', async ({ page }) => {
  await page.goto(`/compose?${EDGE}&lens=production`)
  const filter = page.getByTestId('palette-type:processor/filter')
  // Greyed with the reason in production (ADR-0022 §5, ADR-0023)...
  await expect(filter).toHaveClass(/greyed/)
  await expect(filter.locator('.palette-reason')).toContainText("below this Service's C1 floor")
  // ...and clear in staging, where alpha components belong (ADR-0023 §3).
  await page.getByTestId('lens-control').selectOption('staging')
  await expect(filter).not.toHaveClass(/greyed/)
})

test('edits produce Blueprint documents; ordering findings appear inline', async ({ page }) => {
  await page.goto(`/compose?${EDGE}&lens=production`)
  // Click-add adds to every supported signal the Blueprint routes
  // (ADR-0043 §4): memory_limiter lands as a local at both lane tails.
  await page.getByTestId('palette-type:processor/memory_limiter').click()
  await expect(page.getByTestId('lane-traces')).toContainText('memory-limiter')
  await expect(page.getByTestId('lane-logs')).toContainText('memory-limiter')
  // The engine re-judges the edit at once: back-pressure belongs first,
  // surfaced as a finding while the user works, never a re-sort or a
  // dedicated ordering UI (ADR-0024 §6).
  const strip = page.getByTestId('findings-strip')
  await expect(strip.locator('.compose-finding', { hasText: 'memory_limiter belongs first' }).first())
    .toBeVisible()
  // The fix is an authored edit: removing the misplaced entry clears it.
  await page.getByTestId('remove-traces-3').click()
  await page.getByTestId('remove-logs-3').click()
  await expect(strip.locator('.compose-finding', { hasText: 'memory_limiter belongs first' }))
    .toHaveCount(0)
})

test('a per-lane targeted add lands on the named lane only', async ({ page }) => {
  await page.goto(`/compose?${EDGE}&lens=production`)
  await page.getByTestId('lane-add-logs').selectOption('shared:infosec/pii-redaction')
  await expect(page.getByTestId('lane-logs')).toContainText('infosec/pii-redaction@3')
  await expect(page.getByTestId('lane-traces')).not.toContainText('infosec/pii-redaction@3')
})

test('the YAML flyout shows the rendered artefact read-only; click-off closes it', async ({
  page,
}) => {
  await page.goto(`/compose?${EDGE}&lens=production`)
  await page.getByTestId('yaml-toggle').click()
  const flyout = page.getByTestId('yaml-flyout')
  await expect(flyout).toBeVisible()
  await expect(page).toHaveURL(/yaml=true/)
  // The rendered otelcol shape with provenance-carrying ids (ADR-0024 §5).
  await expect(flyout.locator('.yaml-pre')).toContainText('batch/batcher: {}')
  await expect(flyout.locator('.yaml-pre')).toContainText('otlphttp/data-flow.gateway-exporter')
  await expect(flyout.locator('.yaml-pre')).toContainText('pipelines:')
  // Read-only: nothing editable lives in the flyout (REQ-035).
  await expect(flyout.locator('textarea, input, [contenteditable]')).toHaveCount(0)
  // Click-off closes: the flyout pushes the surface aside, it never covers
  // it, and clicking the surface dismisses it (P1 verdict).
  await page.getByTestId('lane-traces').locator('h3').click()
  await expect(flyout).toHaveCount(0)
})

test('drag-authoring onto the canvas: the drop target names the signal', async ({ page }) => {
  await page.goto(`/compose?${EDGE}&surface=canvas&lens=production`)
  await expect(page.getByTestId('compose-canvas')).toBeVisible()
  const dataTransfer = await page.evaluateHandle(() => new DataTransfer())
  await page.getByTestId('palette-shared:infosec/pii-redaction').dispatchEvent('dragstart', {
    dataTransfer,
  })
  await page.getByTestId('canvas-band-traces').dispatchEvent('drop', { dataTransfer })
  // Drag is a model edit (ADR-0044 §3): the traces lane grew, logs did not.
  await page.getByTestId('view-composer').click()
  await expect(page.getByTestId('lane-traces')).toContainText('infosec/pii-redaction@3')
  await expect(page.getByTestId('lane-logs')).not.toContainText('infosec/pii-redaction@3')
})

test('the canvas is authoring-capable: remove is a model edit', async ({ page }) => {
  await page.goto(`/compose?${EDGE}&surface=canvas&lens=production`)
  await page.getByTestId('canvas-remove-traces-1').click()
  await page.getByTestId('view-composer').click()
  await expect(page.getByTestId('lane-traces')).not.toContainText('batcher')
  await expect(page.getByTestId('lane-logs')).toContainText('batcher')
})

test('Save exits to a PR through the forge adapter, and the PR decides', async ({ page }) => {
  await page.goto(`/compose?${EDGE}&lens=production`)
  await page.getByTestId('save-button').click()
  const proposal = page.getByTestId('proposal')
  await expect(proposal).toBeVisible()
  // Branch-per-draft (ADR-0028), user-attributed (ADR-0014).
  await expect(page.getByTestId('proposal-branch')).toHaveText('compose/data-flow/edge-standard')
  await expect(page.getByTestId('proposal-url')).toHaveAttribute('href', /\/pull\/\d+$/)
  await expect(page.getByTestId('proposal-attribution')).toContainText('Demo user')
})

test('the one hard block: Save disabled, different in kind, a Grant the way out', async ({
  page,
}) => {
  await page.goto(`/compose?${GATEWAY}&lens=production`)
  // The not-allowed exporter already present blocks Save (ADR-0022 §3).
  await expect(page.getByTestId('save-button')).toBeDisabled()
  const blocked = page.getByTestId('save-blocked')
  await expect(blocked).toContainText('debug-tap')
  await expect(page.getByTestId('request-grant')).toBeVisible()
  // The allow-list finding renders as blocking, different in kind from
  // the advisory strip entries (P1 verdict).
  await expect(page.locator('.compose-finding.blocking')).toHaveCount(1)
  // The environment toggle clears floors, never the allow-list finding.
  await page.getByTestId('lens-control').selectOption('staging')
  await expect(page.getByTestId('save-button')).toBeDisabled()
})

test('satisfies renders as a claim linking to the verdict, never a status', async ({ page }) => {
  await page.goto(`/compose?${EDGE}&lens=production`)
  // The chip says "claims" and carries no verdict of its own (REQ-031).
  const chip = page.getByTestId('claim-req-pii-redaction@3')
  await expect(chip).toContainText('claims req-pii-redaction@3')
  await expect(chip).not.toContainText('met')
  // The link lands on the engine's verdict: claimed, and judged not met:
  // intent and fact side by side, never blended.
  await page.getByTestId('claim-verdict-req-pii-redaction@3').click()
  await expect(page).toHaveURL(/surface=requirements/)
  await expect(page.getByTestId('claimed-req-pii-redaction')).toHaveText('claimed @3')
  await expect(page.getByTestId('verdict-req-pii-redaction')).toHaveText('not met')
})

test('a one-click suggestion add discharges a requirement and stamps the claim', async ({
  page,
}) => {
  await page.goto(`/compose?${EDGE}&surface=requirements&lens=production`)
  await expect(page.getByTestId('coverage')).toHaveText('0/2 requirements met by the current draft')
  await page.getByTestId('suggest-req-pii-redaction').click()
  await expect(page.getByTestId('verdict-req-pii-redaction')).toHaveText('met')
  await expect(page.getByTestId('claimed-req-pii-redaction')).toHaveText('claimed @3')
  await expect(page.getByTestId('coverage')).toHaveText('1/2 requirements met by the current draft')
  // The surfaces are projections of one draft (ADR-0043 §1): the composer
  // shows the added reference on the lanes the requirement named.
  await page.getByTestId('view-composer').click()
  await expect(page.getByTestId('lane-traces')).toContainText('infosec/pii-redaction@3')
  await expect(page.getByTestId('lane-logs')).toContainText('infosec/pii-redaction@3')
})

// The Compose landing (ADR-0064): the reader's Blueprints with their
// governance-relevant facts, the Requirement claims they stamp, and a rail
// carrying the palette and Catalogue readings, every reading a door. The
// one verdict-shaped column is the Allow-list standing, derived through
// the same module the Effective palette view reads, because only an
// Allow-list violation blocks a save (ADR-0022 §3).

test('the landing table reports each Blueprint: lanes, claims, Tier, Allow-list standing', async ({
  page,
}) => {
  await page.goto('/compose')
  await expect(page.getByTestId('landing-table')).toBeVisible()

  const gateway = page.getByTestId('landing-row-data-flow/gateway-standard')
  await expect(gateway).toContainText('traces · logs · metrics')
  await expect(gateway).toContainText('req-pii-redaction@3')
  await expect(gateway).toContainText('req-payment-completeness@2')
  // The save gate, read from the Allow-list alone: the same judgement
  // that disables Save in the composer, with the fact beside it.
  await expect(page.getByTestId('landing-standing-data-flow/gateway-standard')).toHaveText(
    'save disabled',
  )
  await expect(gateway.locator('.landing-fact')).toHaveText(
    "debug-tap (exporter/debug) is outside this team's effective Allow-list",
  )

  await expect(page.getByTestId('landing-standing-data-flow/edge-standard')).toHaveText('clear')

  // A Blueprint bound to no Tier says so plainly; its Allow-list is
  // default-allow on a chain with no declared list, so it reads clear.
  const audit = page.getByTestId('landing-row-infosec/audit-standard')
  await expect(audit).toContainText('not used by a Tier')
  await expect(page.getByTestId('landing-standing-infosec/audit-standard')).toHaveText('clear')
})

test('a landing row opens the composer exactly as the cards did', async ({ page }) => {
  await page.goto('/compose')
  await page.getByTestId('blueprint-data-flow/gateway-standard').click()
  await expect(page.getByTestId('compose-detail')).toBeVisible()
  await expect(page.getByTestId('compose-detail')).toContainText('gateway-standard')
  expect(decodeURIComponent(page.url())).toContain('object=blueprint:data-flow/gateway-standard')
})

test('the used-by door lands on the Estate shelf with that card selected', async ({ page }) => {
  await page.goto('/compose')
  await page.getByTestId('landing-used-by-data-flow/gateway-standard').click()
  await page.waitForURL(/\/estate/)
  expect(decodeURIComponent(page.url())).toContain('object=tier:data-flow/gateway')
  await expect(page.getByTestId('card-data-flow/gateway')).toHaveClass(/selected/)
})

test('the Requirements section lists each claim per claiming Blueprint, doors to the verdict', async ({
  page,
}) => {
  await page.goto('/compose')
  await expect(page.locator('.landing-claim')).toHaveCount(3)
  const row = page.getByTestId('landing-claim-data-flow/edge-standard-req-pii-redaction@3')
  await expect(row).toContainText('claimed by edge-standard')
  // The door lands on the engine's verdict for that Blueprint: intent and
  // fact side by side, never blended (REQ-031).
  await page
    .getByTestId('landing-claim-verdict-data-flow/edge-standard-req-pii-redaction@3')
    .click()
  await expect(page).toHaveURL(/surface=requirements/)
  await expect(page.getByTestId('claimed-req-pii-redaction')).toHaveText('claimed @3')
  await expect(page.getByTestId('verdict-req-pii-redaction')).toHaveText('not met')
})

test('the rail reads the effective palette with its Grants, and doors to the palette view', async ({
  page,
}) => {
  await page.goto('/compose')
  await expect(page.getByTestId('landing-palette-summary')).toHaveText(
    'Data Flow: 8 of 13 Catalogue entries allowed · judged against v0.158.0 (active)',
  )
  await expect(page.getByTestId('landing-grant-kafka-egress-for-data-flow')).toHaveText(
    'Grant kafka-egress-for-data-flow allows receiver/kafka, exporter/kafka.',
  )
  await page.getByTestId('landing-open-palette').click()
  await page.waitForURL(/view=palette/)
  await expect(page.getByTestId('palette-summary')).toContainText('8 of 13')
})

test('the rail repeats the impact report behind the version on offer, and doors to Activation', async ({
  page,
}) => {
  await page.goto('/compose')
  await expect(page.getByTestId('landing-catalogue-active')).toHaveText(
    'Active: v0.158.0 · v0.155.0 on offer',
  )
  const lines = page.getByTestId('landing-offer-line')
  await expect(lines).toHaveCount(2)
  await expect(lines.first()).toContainText('processor/transform is removed')
  await page.getByTestId('landing-open-activation').click()
  await page.waitForURL(/view=activation/)
  await expect(page.getByTestId('candidate-v0.155.0')).toBeVisible()
})
