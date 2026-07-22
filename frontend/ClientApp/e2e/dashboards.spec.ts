import { expect, test } from './dcs-test'
import { buildDraftContractFixture, type DraftContractFixture } from './lifecycle-helpers'

/**
 * The workspace dashboards render their backing APIs' data for the roles the
 * SRS assigns them (DCS-FR-TR-28 template overview, DCS-FR-CWE-24 contract
 * overview, DCS-FR-SM-22 signer dashboard, DCS-FR-CSA-21/-24 archive/audit
 * views) — the UI half of requirements whose API half the BDD suite covers.
 *
 * The template/contract lists and the human-readable render are asserted
 * against a fixture authored by clicking a registered contract template and a
 * DRAFT contract into existence through the real UI.
 */

test.describe('dashboards backed by an authored fixture', () => {
  let fixture: DraftContractFixture

  test.beforeAll(async ({ browser }) => {
    test.setTimeout(600_000)
    fixture = await buildDraftContractFixture(browser)
  })

  test('template dashboard lists registered templates', async ({ page, loginAs }) => {
    await loginAs('Template Creator')
    await page.goto('/ui/templates')

    await expect(page.getByText(fixture.contractTemplateName).first()).toBeVisible()
  })

  test('contract dashboard lists contracts with their lifecycle state', async ({ page, loginAs }) => {
    await loginAs('Contract Manager')
    await page.goto('/ui/contracts')

    // The derived contract inherits its template's name at creation; the state
    // chip is asserted among VISIBLE matches (a bare .first() can land on
    // hidden filter options).
    await expect(page.getByText(fixture.contractTemplateName).first()).toBeVisible()
    await expect(page.getByText('DRAFT', { exact: true }).locator('visible=true').first()).toBeVisible()
  })

  test('contract view renders the human-readable document', async ({ page, loginAs }) => {
    await loginAs('Contract Manager')
    await page.goto(`/ui/contracts/view/${fixture.contractDid}`)

    // The human-readable rendering lives under the Contract Content tab.
    await page
      .getByRole('tab', { name: /content/i })
      .or(page.getByText('Contract Content', { exact: true }))
      .first()
      .click()
    // The authored clause's prose, rendered from the machine-readable JSON-LD —
    // the human-readable representation the SRS demands alongside it.
    await expect(page.getByText(fixture.clauseProse).first()).toBeVisible()
  })
})

test('signing list renders for the signer role', async ({ page, loginAs }) => {
  await loginAs('Contract Signer')
  await page.goto('/ui/signing')

  await expect(page.getByRole('heading', { name: 'Signing', exact: true })).toBeVisible()
})

test('audit view renders scoped audits for the auditor role', async ({ page, loginAs }) => {
  await loginAs('Auditor')
  await page.goto('/ui/audit')

  await expect(page.getByRole('heading', { level: 2 }).first()).toBeVisible()
  // The scoped-audit workstation the /pac endpoints back.
  await expect(page.getByText('Execute Audit').first()).toBeVisible()
  await expect(page.getByText('Scope', { exact: true }).first()).toBeVisible()
})
