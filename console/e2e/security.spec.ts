import { expect, test } from '@playwright/test'

// The console is served under a Content Security Policy, and it still
// works under it. The first is a header assertion; the second is the whole
// suite, and this file adds the part the other specs cannot see, which is
// whether the browser blocked anything on the way.

test('the document is served under a policy that allows nothing external', async ({ page }) => {
  const response = await page.goto('/')
  const policy = response?.headers()['content-security-policy']
  expect(policy, 'the console document is served with no Content-Security-Policy').toBeTruthy()

  const directives = new Map(
    policy!.split(';').map((directive) => {
      const [name, ...sources] = directive.trim().split(/\s+/)
      return [name, sources]
    }),
  )

  expect(directives.get('default-src')).toEqual(["'self'"])
  expect(directives.get('frame-ancestors')).toEqual(["'none'"])
  expect(directives.get('object-src')).toEqual(["'none'"])
  expect(directives.get('base-uri')).toEqual(["'none'"])
  expect(directives.get('form-action')).toEqual(["'self'"])

  // The theme resolver runs before the first paint, so it is in the
  // document rather than in a file, and it is admitted by its own hash.
  // Admitting inline script by keyword instead would admit every other
  // inline script with it.
  const script = directives.get('script-src') ?? []
  expect(script.some((source) => source.startsWith("'sha256-"))).toBe(true)
  expect(script).not.toContain("'unsafe-inline'")
  expect(script).not.toContain("'unsafe-eval'")

  expect(response?.headers()['x-content-type-options']).toBe('nosniff')
  expect(response?.headers()['referrer-policy']).toBe('no-referrer')
})

test('nothing the console does is blocked by the policy', async ({ page }) => {
  const blocked: string[] = []
  page.on('console', (message) => {
    if (message.text().includes('Content Security Policy')) blocked.push(message.text())
  })
  await page.addInitScript(() => {
    document.addEventListener('securitypolicyviolation', (event) => {
      const violation = event as SecurityPolicyViolationEvent
      console.log(`Content Security Policy blocked ${violation.blockedURI} on ${violation.violatedDirective}`)
    })
  })

  // Every Workspace, because a blocked stylesheet or chunk shows up on the
  // surface that needs it and nowhere else.
  await page.goto('/')
  await expect(page.getByTestId('home')).toBeVisible()
  await page.getByTestId('nav-estate').click()
  await expect(page.getByTestId('shelf')).toBeVisible()
  await page.getByTestId('nav-topology').click()
  await expect(page.getByTestId('topology-canvas')).toBeVisible()
  await page.getByTestId('nav-compose').click()
  await expect(page.getByTestId('blueprint-data-flow/gateway-standard')).toBeVisible()
  await page.getByTestId('nav-catalogue').click()

  expect(blocked, blocked.join('\n')).toEqual([])
})

test('the theme is resolved on a document served under the policy', async ({ page }) => {
  await page.goto('/')
  // The shell resolves the theme too, so a blocked resolver would still
  // end up stamped: what this holds is that the surface is themed under
  // the policy at all. The test above is the one that fails on a hash
  // that has drifted from the bytes served.
  await expect(page.locator('html')).toHaveAttribute('data-theme', /^(light|dark)$/)
})
