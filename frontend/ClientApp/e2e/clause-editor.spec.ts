import type { Page } from '@playwright/test'
import { type DcsRole, expect, test } from './dcs-test'

type LoginAs = (role: DcsRole) => Promise<void>

async function gotoAs(page: Page, loginAs: LoginAs, role: DcsRole, url: string): Promise<void> {
  await loginAs(role)
  await page.goto(url)
}

// The SRS split-view clause editor: a data field picked from the Semantic Hub
// is wired into BOTH the human prose (as a placeholder) and the machine-
// readable ODRL rule (as a constraint operand) — no IRIs, one clause, both
// readings — and saved into the document.
test('split clause editor wires a hub field into prose and an ODRL rule', async ({ page, loginAs }) => {
  page.setDefaultTimeout(15_000)
  const clauseTitle = `Payment terms ${Date.now()}`

  await gotoAs(page, loginAs, 'Template Creator', '/ui/templates/new')
  await page.getByRole('button', { name: /Component/ }).click()
  await page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox').fill(`FV Clause ${Date.now()}`)

  await page.getByRole('tab', { name: /Clauses/ }).click()

  const editor = page.getByTestId('split-clause-editor')
  await expect(editor).toBeVisible()
  await editor.getByPlaceholder('Clause title').fill(clauseTitle)

  // Pick a registered hub domain object — it becomes a chip usable in both panes.
  await editor.locator('select').first().selectOption({ label: 'Payment Amount' })
  await expect(editor.locator('.badge').filter({ hasText: 'Payment Amount' })).toBeVisible()

  // Human prose (contenteditable).
  await editor.locator('.clause-editor').first().click()
  await page.keyboard.type('The counterparty shall pay the agreed amount.')

  // Machine-readable meaning: constrain the same field.
  await editor.getByRole('button', { name: '+ constraint' }).click()
  const constraintRow = editor.locator('.flex.flex-wrap.items-center.gap-1').last()
  await constraintRow.locator('select').first().selectOption({ label: 'Payment Amount' })
  await constraintRow.locator('input[placeholder="value"]').fill('500')

  await editor.getByRole('button', { name: 'Add clause', exact: true }).click()
  await expect(editor.getByPlaceholder('Clause title')).toHaveValue('')

  // Create the template and assert the document carries BOTH readings of the
  // clause: the prose block and the ODRL rule constraining the same field.
  const created = page.waitForRequest((r) => r.url().includes('/template/create') && r.method() === 'POST')
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  const doc = (await created).postDataJSON().template_data as {
    'dcs:policies': { 'odrl:obligation'?: { 'odrl:constraint'?: { 'odrl:leftOperand': { '@id': string } } }[] }
    'dcs:contractData': { 'dcs:fields': { '@id': string; 'dcs:parameterName': string }[] }[]
    'dcs:documentStructure': { 'dcs:blocks': { '@list': { 'dcs:title'?: string }[] } }
  }

  const blocks = doc['dcs:documentStructure']['dcs:blocks']['@list']
  expect(
    blocks.some((b) => b['dcs:title'] === clauseTitle),
    'prose block persisted',
  ).toBeTruthy()

  const fieldIds = new Set(doc['dcs:contractData'].flatMap((r) => r['dcs:fields']).map((f) => f['@id']))
  expect(fieldIds.size, 'the picked hub field was declared as a requirement field').toBeGreaterThan(0)

  const obligations = doc['dcs:policies']['odrl:obligation'] ?? []
  const constrainsField = obligations.some((rule) => {
    const leftOp = rule['odrl:constraint']?.['odrl:leftOperand']?.['@id']
    return !!leftOp && fieldIds.has(leftOp)
  })
  expect(constrainsField, 'the ODRL rule constrains the declared field').toBeTruthy()
})
