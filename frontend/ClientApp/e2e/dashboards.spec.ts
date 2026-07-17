import { expect, seededFixtures, test } from './dcs-test'

/**
 * The workspace dashboards render their backing APIs' data for the roles the
 * SRS assigns them (DCS-FR-TR-28 template overview, DCS-FR-CWE-24 contract
 * overview, DCS-FR-SM-22 signer dashboard, DCS-FR-CSA-21/-24 archive/audit
 * views) — the UI half of requirements whose API half the BDD suite covers.
 */

test('template dashboard lists registered templates', async ({ page, loginAs }) => {
  await loginAs('Template Creator')
  await page.goto('/ui/templates')

  await expect(page.getByText('BDD Contract Source Template').first()).toBeVisible()
})

test('contract dashboard lists contracts with their lifecycle state', async ({ page, loginAs }) => {
  await loginAs('Contract Manager')
  await page.goto('/ui/contracts')

  // The seeded contract inherits its template's name at creation; the state
  // chip is asserted among VISIBLE matches (a bare .first() can land on
  // hidden filter options).
  await expect(page.getByText('BDD Contract Source Template').first()).toBeVisible()
  await expect(page.getByText('DRAFT', { exact: true }).locator('visible=true').first()).toBeVisible()
})

test('contract view renders the human-readable document', async ({ page, loginAs }) => {
  const { contractDid } = seededFixtures()
  await loginAs('Contract Manager')
  await page.goto(`/ui/contracts/view/${contractDid}`)

  // The human-readable rendering lives under the Contract Content tab.
  await page
    .getByRole('tab', { name: /content/i })
    .or(page.getByText('Contract Content', { exact: true }))
    .first()
    .click()
  // The seeded ODRL fixture document's clause text, rendered from the
  // machine-readable JSON-LD — the human-readable representation the SRS
  // demands alongside it.
  await expect(page.getByText('Provider coverage:').first()).toBeVisible()
})

test('signing dashboard renders for the signer role', async ({ page, loginAs }) => {
  await loginAs('Contract Signer')
  await page.goto('/ui/signing')

  await expect(page.getByRole('heading', { name: 'Signing Dashboard' })).toBeVisible()
})

test('audit view renders scoped audits for the auditor role', async ({ page, loginAs }) => {
  await loginAs('Auditor')
  await page.goto('/ui/audit')

  await expect(page.getByRole('heading', { level: 2 }).first()).toBeVisible()
  // The scoped-audit workstation the /pac endpoints back.
  await expect(page.getByText('Execute Audit').first()).toBeVisible()
  await expect(page.getByText('Scope', { exact: true }).first()).toBeVisible()
})
