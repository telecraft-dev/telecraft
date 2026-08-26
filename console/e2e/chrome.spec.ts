import { expect, test } from '@playwright/test'

// The two controls ADR-0047 adds: the theme, resolved in three states and
// offered from the profile menu since the chrome compaction (issue #182),
// and the panel width, which belongs to the reader.

test('the theme resolves in three states and survives a reload', async ({ page }) => {
  await page.goto('/estate')

  // The select sits in the profile menu, still three states as three words.
  await page.getByTestId('profile-trigger').click()

  // Dark and light are stamped on the root element, which is what every
  // colour in tokens.css is selected by.
  const theme = page.getByTestId('theme-control')
  await theme.selectOption('dark')
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await theme.selectOption('light')
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')

  // The choice is a device preference, so it is remembered, and it is not
  // in the URL, which is the documented exception to ADR-0042 §3.5.
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  expect(page.url()).not.toContain('theme')

  // `system` is a third state, not the absence of a choice: it follows the
  // machine, which this context reports as light.
  await page.getByTestId('profile-trigger').click()
  await theme.selectOption('system')
  await expect(theme).toHaveValue('system')
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

test('nothing in the chrome wraps, overlaps, or is printed over', async ({ page }) => {
  // Two regressions live here, one after the other. First the theme
  // control pushed the demo's chrome to 1749px inside 1600px and
  // "Catalogue & Governance" wrapped onto three lines. Then the fix for
  // that let the Workspace navigation shrink past its own content, so the
  // names ran underneath the controls beside them: a worse failure, and
  // one the height assertion could not see, because an overlapping row is
  // still one row.
  //
  // So this asserts the property rather than either symptom: every item in
  // the chrome sits on one line, and no two of them occupy the same space.
  // 800 and 720 are in the list deliberately. The regression was found on
  // the demo, whose chrome carries a provenance banner this build does not,
  // so at ordinary widths this harness cannot reproduce the crowding that
  // caused it. Narrow enough and it can: the same shrink-below-content
  // fault wrapped this chrome to 85px at 800 and 109px at 720.
  for (const width of [720, 800, 1024, 1280, 1600, 1920]) {
    await page.setViewportSize({ width, height: 900 })
    await page.goto('/estate')
    await expect(page.locator('.workspace-link').first()).toBeVisible()

    const chrome = page.locator('.chrome')
    expect((await chrome.boundingBox())!.height, `chrome wraps at ${width}px`).toBeLessThan(64)

    // One line each. One line is ~33px at the base size and two would be
    // ~57px, so 40px separates them without pinning the exact leading.
    for (const link of await page.locator('.workspace-link').all()) {
      const height = (await link.boundingBox())!.height
      expect(height, `a Workspace name wraps at ${width}px`).toBeLessThan(40)
    }

    // Nothing overlaps anything. Measured over the chrome's own children
    // and the controls inside them, which is where the collision was.
    const boxes = await page.evaluate(() => {
      const items = [
        ...document.querySelectorAll('.chrome > *, .chrome-controls > *'),
      ].filter((el) => !el.contains(document.querySelector('.chrome-controls')))
      return items.map((el) => {
        const r = el.getBoundingClientRect()
        return { name: el.className || el.tagName, left: r.left, right: r.right }
      })
    })
    for (let i = 0; i < boxes.length; i++) {
      for (let j = i + 1; j < boxes.length; j++) {
        const a = boxes[i]!
        const b = boxes[j]!
        const overlap = Math.min(a.right, b.right) - Math.max(a.left, b.left)
        expect(
          overlap,
          `"${a.name}" and "${b.name}" overlap by ${Math.round(overlap)}px at ${width}px`,
        ).toBeLessThanOrEqual(0)
      }
    }
  }
})
