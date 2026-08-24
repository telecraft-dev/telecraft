import { expect, test } from '@playwright/test'

// Home (ADR-0056): the landing, whose question is "where do I look first?".
// The derivation is held by tests/summary.test.ts. What only a browser can
// answer is here, and it is the two rules that stop a landing page lying:
// every element is a door and every door carries its filter (§4), and no
// number on the page is blended (§3).

test('lands on Home rather than redirecting to a Workspace', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('home')).toBeVisible()
  await expect(page).toHaveURL(/\/(\?|$)/)
  // Home is the pressed entry, and it is pressed nowhere else: `/` prefixes
  // every other route, so this is the ADR-0048 active-link hazard.
  await expect(page.getByTestId('nav-home')).toHaveAttribute('aria-current', 'page')
  await page.getByTestId('nav-estate').click()
  await expect(page.getByTestId('nav-home')).not.toHaveAttribute('aria-current', 'page')
})

test('shows ratio-plus-worst per kind, and no blended score anywhere', async ({ page }) => {
  await page.goto('/')
  const standing = page.getByTestId('home-standing')
  await expect(standing).toBeVisible()

  // One tile per finding kind, plus the neutral count as their sibling
  // rather than a footnote (ADR-0017, ADR-0035).
  await expect(page.getByTestId('standing-delivery')).toBeVisible()
  await expect(page.getByTestId('standing-expectation')).toBeVisible()
  await expect(page.getByTestId('standing-conformance')).toBeVisible()
  await expect(page.getByTestId('standing-neutral')).toBeVisible()

  // The fixture estate has three production Tiers; `data-flow/gateway`
  // carries the violation, so conformance passes two of the three.
  await expect(page.getByTestId('standing-conformance')).toContainText('2/3')

  // ADR-0056 §3: a single health score is the one thing this surface must
  // not grow, and a percentage is how one would arrive.
  await expect(standing).not.toContainText('%')

  // The lens is evaluation context, and the all-Environments count sits
  // beside it so the lens conceals no finding (§6).
  await expect(page.getByTestId('home-lens')).toHaveText('production')
  await expect(page.getByTestId('home-all-environments')).toBeVisible()
})

test('the worst Tier is a door that opens its card in place', async ({ page }) => {
  await page.goto('/')
  await page.getByTestId('home-tier-data-flow/gateway').getByRole('link').click()

  // ADR-0056 §4: the door carries its filter, so the card is already
  // selected rather than merely on screen somewhere.
  await expect(page.getByTestId('card-panel')).toBeVisible()
  await expect(page).toHaveURL(/object=tier%3Adata-flow%2Fgateway/)
  await expect(page.getByTestId('shelf')).toBeVisible()
})

test('a Team is a door to its shelf section, worst Team first', async ({ page }) => {
  await page.goto('/')
  // Platform owns the subtree holding the violation, so it leads.
  const teams = page.locator('[data-testid^="home-team-"]')
  await expect(teams.first()).toHaveAttribute('data-testid', 'home-team-platform')

  await page.getByTestId('home-team-platform').getByRole('link').click()
  await expect(page).toHaveURL(/object=team%3Aplatform/)
  await expect(page.getByTestId('shelf')).toBeVisible()
})

test('the ungoverned count is a door to the flat list, pre-filtered', async ({ page }) => {
  await page.goto('/')
  // Both referents stay distinct and the total is only the label
  // (ADR-0030, ADR-0031): three served and two foreign.
  await expect(page.getByTestId('home-ungoverned-count')).toHaveText('5 collectors')
  await expect(page.getByTestId('home-ungoverned')).toContainText('3 served')
  await expect(page.getByTestId('home-ungoverned')).toContainText('2 foreign')

  await page.getByTestId('home-to-ungoverned').click()
  await expect(page).toHaveURL(/ungoverned=true/)
  await expect(page.getByTestId('flat-list')).toBeVisible()
})

test('Rollouts waiting on a person lead, and each is a door to the ledger', async ({ page }) => {
  await page.goto('/')
  // `abort` outranks `blocked`: both want a person, and one wants them
  // sooner (ADR-0029 §5, §6).
  const rollouts = page.locator('[data-testid^="home-rollout-"]')
  await expect(rollouts.first()).toHaveAttribute(
    'data-testid',
    'home-rollout-data-flow/gateway-staging-trial',
  )
  await expect(rollouts.first()).toContainText('abort')

  await page.getByTestId('home-rollout-data-flow/gateway-canary').getByRole('link').click()
  await expect(page).toHaveURL(/object=rollout%3Adata-flow%2Fgateway-canary/)
  await expect(page.getByTestId('rollout-panel')).toBeVisible()
})

test('the lens re-judges Home and names what it is not judging', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('home-worst')).toContainText('1 Tier in production')

  await page.getByTestId('lens-control').selectOption('staging')
  await expect(page.getByTestId('home-lens')).toHaveText('staging')

  // Nothing in staging carries a finding, and the production violation is
  // counted rather than dropped: the lens is emphasis, never a place to
  // hide (ADR-0042 §4, ADR-0056 §6).
  await expect(page.getByTestId('home-worst')).toContainText('Nothing in staging has a finding')
  await expect(page.getByTestId('home-worst-elsewhere')).toContainText('1 more in other')
})
