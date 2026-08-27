import { expect, test } from '@playwright/test'

// Setup guidance on the never_seen card (ADR-0060 §3 to §6): the waiting
// room carries what to run, where the artefact and endpoints are, and the
// identity the collector must report. The snippet content is held by
// tests/setup.test.ts; what only a browser can answer is here: the panel
// section appears on the right card and on no other, and the toggles and
// the image field drive what the blocks show.

const PAYMENTS = 'object=tier%3Adata-flow%2Fpayments-gateway'

test('the never_seen card opens on Served for Kubernetes, image still a placeholder', async ({
  page,
}) => {
  await page.goto(`/estate?${PAYMENTS}`)
  await expect(page.getByTestId('panel-setup')).toBeVisible()

  // Served on Kubernetes is the resting choice, and the pressed controls
  // say so.
  await expect(page.getByTestId('setup-served')).toHaveClass(/selected/)
  await expect(page.getByTestId('setup-kubernetes')).toHaveClass(/selected/)

  // The identity block carries the selector's attributes, filled in for
  // real; the manifest keeps the adopter's placeholder until an image is
  // named.
  await expect(page.getByTestId('block-identity')).toContainText('service.namespace: payments')
  await expect(page.getByTestId('block-supervisor')).toContainText(
    'https://opamp.estate.internal:4320',
  )
  await expect(page.getByTestId('block-manifest')).toContainText('YOUR_IMAGE')
})

test('a named image lands in the manifest in place of the placeholder', async ({ page }) => {
  await page.goto(`/estate?${PAYMENTS}`)
  await page
    .getByTestId('setup-image')
    .fill('registry.internal/payments/otelcol-supervised:0.158.0')
  await expect(page.getByTestId('block-manifest')).toContainText(
    'image: registry.internal/payments/otelcol-supervised:0.158.0',
  )
  await expect(page.getByTestId('block-manifest')).not.toContainText('YOUR_IMAGE')
})

test('a Linux server shows packages and units, and no image field', async ({ page }) => {
  await page.goto(`/estate?${PAYMENTS}`)
  await page.getByTestId('setup-linux').click()

  await expect(page.getByTestId('block-packages')).toBeVisible()
  await expect(page.getByTestId('block-packages')).toContainText('systemd')
  await expect(page.getByTestId('block-manifest')).toHaveCount(0)
  // Packages, not images: there is nothing for an image field to name.
  await expect(page.getByTestId('setup-image')).toHaveCount(0)
  // supervisor.yaml stays: every served collector boots beside one.
  await expect(page.getByTestId('block-supervisor')).toBeVisible()
})

test('the Foreign path shows the rendered artefact beside the upstream image', async ({
  page,
}) => {
  await page.goto(`/estate?${PAYMENTS}`)
  await page.getByTestId('setup-foreign').click()

  const artefact = page.getByTestId('block-artefact')
  await expect(artefact).toBeVisible()
  await expect(artefact).toContainText('rendered/data-flow/payments-gateway.yaml')
  await expect(artefact).toContainText('otel/opentelemetry-collector-contrib:0.158.0')

  // The upstream image pre-fills from the activated Catalogue's release
  // and stays editable.
  await expect(page.getByTestId('setup-image')).toHaveValue(
    'otel/opentelemetry-collector-contrib:0.158.0',
  )
  await expect(page.getByTestId('block-supervisor')).toHaveCount(0)
  await expect(page.getByTestId('block-identity')).toBeVisible()
})

test('the face says the Tier is waiting, in neutral words', async ({ page }) => {
  await page.goto(`/estate?${PAYMENTS}`)
  const card = page.getByTestId('card-data-flow/payments-gateway')
  await expect(card).toContainText('no collectors yet')
  // One quiet line where the matrix would be: no reading has ever been
  // taken, and a grid of empty cells would say it worse. Scoped to the
  // face: the open panel renders the same quiet line in its flow section.
  await expect(card.getByTestId('matrix-data-flow/payments-gateway-quiet')).toHaveText(
    'no readings yet',
  )
})

test('a governed card carries no setup guidance', async ({ page }) => {
  await page.goto('/estate?object=tier%3Adata-flow%2Fgateway')
  await expect(page.getByTestId('card-panel')).toBeVisible()
  await expect(page.getByTestId('panel-setup')).toHaveCount(0)
})
