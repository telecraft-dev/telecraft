import { expect, test } from '@playwright/test'

// The issue #36 acceptance criteria, end to end: an active Rollout renders
// per-cohort progress computed from stamps and the membership function,
// both delivery paths appear in one view with foreign members advisory,
// halt and abort states are visible with provenance and deep-link to the
// Rollout panel, and every state is URL-addressable (ADR-0029, ADR-0042).

const CANARY = 'data-flow/gateway-canary'
const TRIAL = 'data-flow/gateway-staging-trial'

test('the rollout ledger renders per-cohort progress across both paths', async ({ page }) => {
  await page.goto('/topology?view=rollout')
  await expect(page.getByTestId('rollout-ledger')).toBeVisible()

  // Two active Rollouts, one per Tier (ADR-0029 §2).
  await expect(page.getByTestId(`rollout-${CANARY}`)).toBeVisible()
  await expect(page.getByTestId(`rollout-${TRIAL}`)).toBeVisible()

  // The canary's three cohorts: entered, active, pending: cumulative
  // membership, the union that only ever widens (§4).
  await expect(page.getByTestId(`cohort-${CANARY}-0`)).toContainText('1 member')
  await expect(page.getByTestId(`cohort-${CANARY}-1`)).toContainText('3 members')
  await expect(page.getByTestId(`cohort-${CANARY}-1`)).toContainText('active')
  await expect(page.getByTestId(`cohort-${CANARY}-2`)).toContainText('pending')

  // Both delivery paths in one view: the active cohort splits 2 served,
  // 1 foreign, the foreign member marked advisory, lag never failure (§7).
  await expect(page.getByTestId(`cohort-served-${CANARY}-1`)).toContainText('1 of 2 on to')
  await expect(page.getByTestId(`cohort-foreign-${CANARY}-1`)).toContainText('0 of 1 on to')
  await expect(page.getByTestId(`cohort-foreign-${CANARY}-1`)).toContainText('1 from')
  await expect(page.getByTestId(`advisory-${CANARY}-1`)).toHaveText('advisory')
})

test('halt and abort states are visible and deep-link to the Rollout panel', async ({ page }) => {
  await page.goto('/topology?view=rollout')

  // The halt state: blocked below the abort threshold, so the advance is
  // never proposed (§6).
  await expect(page.getByTestId(`rollout-decision-${CANARY}`)).toContainText('halted')
  // The abort state: the staging trial's whole cohort went dark.
  await expect(page.getByTestId(`rollout-decision-${TRIAL}`)).toContainText('abort proposed')

  // The halted member deep-links to the Rollout panel, where the reason
  // and provenance live, and the state lands in the URL (ADR-0042 §3.5).
  await page.getByTestId(`rollout-halt-${CANARY}-gw-1`).click()
  await expect(page).toHaveURL(/object=rollout%3Adata-flow%2Fgateway-canary/)
  await expect(page.getByTestId('rollout-panel')).toBeVisible()
  await expect(page.getByTestId('rollout-panel-decision')).toContainText('halted')
  await expect(page.getByTestId(`halt-${CANARY}-gw-1`)).toContainText(
    'reported FAILED for the new version',
  )

  // The "why?" provenance popover: the authored rollout file lines and the
  // judged SHA (ADR-0041 §3, ADR-0042 §5).
  await page.getByTestId('why-stage').click()
  await expect(page.getByTestId('why-popover')).toContainText(
    'teams/data-flow/rollouts/gateway-canary.yaml',
  )
  await expect(page.getByTestId('why-popover')).toContainText('9c2f4e1')
})

test('the abort verdict carries the went-dark evidence on the foreign path', async ({ page }) => {
  await page.goto(`/topology?view=rollout&object=rollout%3Adata-flow%2Fgateway-staging-trial`)
  await expect(page.getByTestId('rollout-panel-title')).toHaveText('gateway-staging-trial')
  await expect(page.getByTestId('rollout-panel-decision')).toContainText('abort proposed')
  await expect(page.getByTestId(`halt-${TRIAL}-gws-0`)).toContainText('went silent')
  // The foreign path never reports FAILED; the halt is honest about where
  // it came from (§7).
  await expect(page.getByTestId(`halt-${TRIAL}-gws-0`)).toContainText('foreign')
})

test('every ledger state is URL-addressable and restores fresh', async ({ page }) => {
  // A Rollout deep link needs no explicit view: the object implies the
  // ledger (ADR-0042 §3.5).
  await page.goto('/topology?object=rollout%3Adata-flow%2Fgateway-canary')
  await expect(page.getByTestId('rollout-ledger')).toBeVisible()
  await expect(page.getByTestId('rollout-panel-title')).toHaveText('gateway-canary')

  // The view-switcher travels in place, preserving the lens (rule 3.1);
  // leaving the ledger drops the Rollout selection it cannot show.
  await page.goto('/topology?view=rollout&lens=staging')
  await page.getByTestId('view-flow').click()
  await expect(page).toHaveURL(/view=flow/)
  await expect(page).toHaveURL(/lens=staging/)
  await expect(page.getByTestId('topology-canvas')).toBeVisible()
  await page.getByTestId('view-rollout').click()
  await expect(page).toHaveURL(/view=rollout/)
  await expect(page.getByTestId('rollout-ledger')).toBeVisible()
})

test('the ledger links into the one model: Tier card and flat-list doors', async ({ page }) => {
  await page.goto('/topology?view=rollout')

  // The target Tier summons the universal card panel in place: the same
  // component as everywhere a Tier appears (ADR-0042 §3.2).
  await page.getByTestId(`rollout-tier-${CANARY}`).click()
  await expect(page.getByTestId('card-panel')).toBeVisible()
  await expect(page.getByTestId('panel-title')).toHaveText('gateway')

  // A member count is a door to the flat list, pre-filtered: per-collector
  // detail lives there only (ADR-0042 §3.4).
  await page.getByTestId(`cohort-members-${CANARY}-1`).click()
  await expect(page).toHaveURL(/\/estate\?.*view=list/)
  await expect(page).toHaveURL(/tier=data-flow%2Fgateway/)
  await expect(page.getByTestId('collector-table')).toBeVisible()
})

test('jump-to-object reaches a Rollout and lands on its ledger panel', async ({ page }) => {
  await page.goto('/estate')
  await page.getByTestId('jump-trigger').click()
  await page.getByTestId('jump-input').fill('canary')
  await page.getByTestId(`jump-result-rollout-${CANARY}`).click()
  await expect(page).toHaveURL(/\/topology\?.*object=rollout%3Adata-flow%2Fgateway-canary/)
  await expect(page.getByTestId('rollout-panel')).toBeVisible()
})
