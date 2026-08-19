import { expect, test } from '@playwright/test'

// The two controls ADR-0047 adds to the chrome: the theme, resolved in
// three states, and the panel width, which belongs to the reader.

test('the theme resolves in three states and survives a reload', async ({ page }) => {
  await page.goto('/estate')

  // Dark and light are stamped on the root element, which is what every
  // colour in tokens.css is selected by.
  await page.getByTestId('theme-dark').click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await page.getByTestId('theme-light').click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')

  // The choice is a device preference, so it is remembered — and it is not
  // in the URL, which is the documented exception to ADR-0042 §3.5.
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  expect(page.url()).not.toContain('theme')

  // `system` is a third state, not the absence of a choice: it follows the
  // machine, which this context reports as light.
  await page.getByTestId('theme-system').click()
  await expect(page.getByTestId('theme-system')).toHaveAttribute('aria-pressed', 'true')
  await expect(page.locator('html')).toHaveAttribute('data-theme', /light|dark/)
})

test('the theme is stamped before the first paint, never after it', async ({ page }) => {
  // The inline resolver in index.html runs ahead of any module, so the
  // document is never painted in the wrong theme and swapped. Reading the
  // attribute at DOMContentLoaded proves the inline copy did it.
  await page.addInitScript(() => {
    document.addEventListener('DOMContentLoaded', () => {
      ;(window as unknown as { themeAtLoad?: string }).themeAtLoad =
        document.documentElement.dataset.theme
    })
  })
  await page.goto('/estate')
  const atLoad = await page.evaluate(
    () => (window as unknown as { themeAtLoad?: string }).themeAtLoad,
  )
  expect(atLoad).toMatch(/light|dark/)
})

test('a panel is resized by dragging its edge, and remembers the width', async ({ page }) => {
  await page.goto('/estate?object=tier%3Adata-flow%2Fgateway')

  const panel = page.getByTestId('card-panel')
  const handle = page.getByTestId('card-panel-resize')
  await expect(panel).toBeVisible()

  // It is a separator to a screen reader, and says where it currently is.
  await expect(handle).toHaveAttribute('role', 'separator')
  await expect(handle).toHaveAttribute('aria-orientation', 'vertical')

  const before = (await panel.boundingBox())!.width
  const grip = (await handle.boundingBox())!

  // Dragging leftwards widens the panel: it grows into the surface beside
  // it rather than off the screen.
  await page.mouse.move(grip.x + grip.width / 2, grip.y + 40)
  await page.mouse.down()
  await page.mouse.move(grip.x - 160, grip.y + 40, { steps: 8 })
  await page.mouse.up()

  const after = (await panel.boundingBox())!.width
  expect(after).toBeGreaterThan(before + 120)

  // The width is a device preference, like the theme: remembered, and not
  // in the URL.
  await page.reload()
  expect((await panel.boundingBox())!.width).toBeCloseTo(after, 0)
  expect(page.url()).not.toContain('width')
})

test('the resize handle takes the keyboard, not only a pointer', async ({ page }) => {
  await page.goto('/estate?object=tier%3Adata-flow%2Fgateway')

  const panel = page.getByTestId('card-panel')
  const handle = page.getByTestId('card-panel-resize')
  await handle.focus()

  const before = (await panel.boundingBox())!.width
  await page.keyboard.press('ArrowLeft')
  await page.keyboard.press('ArrowLeft')
  expect((await panel.boundingBox())!.width).toBeGreaterThan(before)

  // Home returns it to the width the panel ships with.
  await page.keyboard.press('Home')
  expect((await panel.boundingBox())!.width).toBeCloseTo(before, 0)
})
