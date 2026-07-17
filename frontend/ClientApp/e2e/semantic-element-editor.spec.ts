import type { Page } from '@playwright/test'
import { type DcsRole, expect, test } from './dcs-test'

type LoginAs = (role: DcsRole) => Promise<void>

async function gotoAs(page: Page, loginAs: LoginAs, role: DcsRole, url: string): Promise<void> {
  await loginAs(role)
  await page.goto(url)
}

// The Data Requirements tab authors a domain field through the SAME shacl-form
// that renders typed clauses (SemanticElementEditor) — the field's value shape
// is generated from the hub SLA ontology, and the field lands in the document's
// data requirements.
test('unified semantic editor adds a domain field via shacl-form', async ({ page, loginAs }) => {
  page.setDefaultTimeout(15_000)
  await gotoAs(page, loginAs, 'Template Creator', '/ui/templates/new')

  await page.getByRole('button', { name: /Component/ }).click()
  await page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox').fill(`FV Field ${Date.now()}`)

  await page.getByRole('tab', { name: /Data Requirements/ }).click()

  await page.getByRole('button', { name: 'Payment Amount', exact: true }).click()

  const shaclForm = page.locator('shacl-form')
  await expect(shaclForm).toBeVisible()
  await shaclForm.locator('input').first().fill('5000')

  await page.getByRole('button', { name: 'Add field', exact: true }).click()

  const currentRequirements = page.locator('section').filter({ hasText: 'Current data requirements' })
  await expect(currentRequirements.getByText('No data requirements yet')).toBeHidden()
  await expect(currentRequirements.locator('span.font-medium', { hasText: 'Payment Amount' })).toBeVisible()
})
