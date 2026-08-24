import { expect, test, type Page } from '@playwright/test'
import { TOURS } from '../src/tours/registry'

// Guided Tours (ADR-0051). The pure rules (placement, clamping, which
// URLs a Tour may open itself on) are held by tests/tours.test.ts. What
// only a browser can answer is here, and the first of those is §4's
// invariant: every anchor a Tour was authored with resolves to something
// on the surface it names.

const PRESENTATION_KEY = 'telecraft.console.presentation.v1.demo-user'

/**
 * A reader who has never been offered anything, which the saved state is
 * not. Cleared once, from a URL that is nobody's bare arrival, so the next
 * navigation is the first thing the Tour sees, and so that what the Tour
 * writes afterwards survives, which is the whole point of the test below.
 */
async function asNewReader(page: Page) {
  await page.goto('/estate?view=list')  // not a bare landing, and never was
  await page.evaluate((key) => localStorage.removeItem(key), PRESENTATION_KEY)
}

test('every Step of every Tour points at something that is really there', async ({ page }) => {
  for (const tour of TOURS) {
    for (const [index, step] of tour.steps.entries()) {
      // Entered by URL, one Step at a time: a Step is addressable, so this
      // walks the Tour the way a reader following a link would.
      await page.goto(`/estate?tour=${tour.id}&step=${index + 1}`)

      const card = page.getByTestId('tour-card')
      await expect(card, `${tour.id}/${step.id} renders`).toBeVisible()
      await expect(card).toHaveAttribute('data-step', step.id)
      await expect(page.getByTestId('tour-progress')).toHaveText(
        `Step ${index + 1} of ${tour.steps.length}`,
      )

      if (step.to !== undefined) {
        await expect(page, `${tour.id}/${step.id} lands on its Workspace`).toHaveURL(
          new RegExp(`^[^?]*${step.to}\\?`),
        )
      }

      if (step.anchor === undefined) {
        // No anchor is the welcome's shape, and the only shape allowed to
        // be centred on purpose (§7).
        await expect(card).toHaveAttribute('data-placement', 'centre')
        continue
      }

      // The anchor exists, is on screen, and the card is beside it rather
      // than centred: a Step degrading to centred in production is
      // survivable (§4), a Step authored against an anchor nobody carries
      // is a defect.
      const anchor = page.locator(`[data-tour="${step.anchor}"]`).first()
      await expect(anchor, `${tour.id}/${step.id} finds its anchor`).toBeVisible()
      await expect(page.getByTestId('tour-spotlight')).toBeVisible()
      await expect(card, `${tour.id}/${step.id} is placed against its anchor`).not.toHaveAttribute(
        'data-placement',
        'centre',
      )
    }
  }
})

test('the welcome opens itself once, for a reader who has not been offered it', async ({
  page,
}) => {
  await asNewReader(page)
  // The bare landing is Home (ADR-0056 §1), and that is where the welcome
  // offers itself.
  await page.goto('/')

  const card = page.getByTestId('tour-card')
  await expect(card).toBeVisible()
  await expect(card).toHaveAttribute('data-step', 'welcome')
  await expect(card).toHaveAttribute('data-placement', 'centre')

  // Being offered it is what is remembered, not finishing it: a reader who
  // closes the tab halfway through is not offered it again.
  await page.getByTestId('tour-end').click()
  await expect(card).toBeHidden()
  await page.goto('/')
  await expect(card).toBeHidden()
})

test('it never lands on top of somebody else’s link', async ({ page }) => {
  await asNewReader(page)

  // A URL carrying context was sent to this reader for the thing in it.
  await page.goto('/estate?object=tier%3Adata-flow%2Fgateway')
  await expect(page.getByTestId('card-panel')).toBeVisible()
  await expect(page.getByTestId('tour-card')).toBeHidden()

  // Their own bare arrival still opens it.
  await page.goto('/')
  await expect(page.getByTestId('tour-card')).toBeVisible()
})

test('the position is in the URL, and the Tour survives a Workspace switch', async ({ page }) => {
  await page.goto('/estate?tour=welcome&step=3')
  await expect(page.getByTestId('tour-card')).toHaveAttribute('data-step', 'shelf')

  // Next is a navigation, so the address bar says where the reader is and
  // the back button walks the Tour backwards.
  await page.getByTestId('tour-next').click()
  await expect(page).toHaveURL(/step=4/)
  await expect(page.getByTestId('tour-card')).toHaveAttribute('data-step', 'bands')
  await page.goBack()
  await expect(page.getByTestId('tour-card')).toHaveAttribute('data-step', 'shelf')

  // A Workspace switched by hand keeps the Tour, because a Tour belongs to
  // the console rather than to a Workspace (§3).
  await page.getByTestId('nav-topology').click()
  await expect(page).toHaveURL(/\/topology\?.*tour=welcome/)
  await expect(page.getByTestId('tour-card')).toBeVisible()
})

test('a Tour nobody authored is no Tour, never an error', async ({ page }) => {
  await page.goto('/estate?tour=nonesuch&step=4')
  await expect(page.getByTestId('shelf')).toBeVisible()
  await expect(page.getByTestId('tour-card')).toBeHidden()

  // And a Step past the end lands on the last one rather than nowhere.
  const last = TOURS[0]!.steps.length
  await page.goto(`/estate?tour=welcome&step=${last + 40}`)
  await expect(page.getByTestId('tour-progress')).toHaveText(`Step ${last} of ${last}`)
})

test('the product stays usable while a Tour points at it', async ({ page }) => {
  await page.goto('/estate?tour=welcome&step=5')
  await expect(page.getByTestId('tour-card')).toHaveAttribute('data-step', 'lens')

  // The spotlight dims the surface and takes nothing from it: the control
  // it is lighting still works, which is what "narrates, never drives"
  // means in the markup (§2).
  await page.getByTestId('lens-control').selectOption('staging')
  await expect(page).toHaveURL(/lens=staging/)
  await expect(page.getByTestId('tour-card')).toBeVisible()

  // Escape leaves from wherever the reader is, and the URL forgets it.
  await page.keyboard.press('Escape')
  await expect(page.getByTestId('tour-card')).toBeHidden()
  expect(page.url()).not.toContain('tour=')
})

test('the chrome offers it again, to the reader who skipped it', async ({ page }) => {
  await page.goto('/estate')
  await expect(page.getByTestId('tour-card')).toBeHidden()

  await page.getByTestId('tour-trigger').click()
  const card = page.getByTestId('tour-card')
  await expect(card).toBeVisible()
  await expect(card).toHaveAttribute('data-step', 'welcome')
  await expect(page).toHaveURL(/tour=welcome&step=1|step=1&tour=welcome/)
})
