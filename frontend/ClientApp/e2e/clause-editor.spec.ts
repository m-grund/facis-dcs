import type { Page } from '@playwright/test'
import { type DcsRole, expect, test } from './dcs-test'

type LoginAs = (role: DcsRole) => Promise<void>

async function gotoAs(page: Page, loginAs: LoginAs, role: DcsRole, url: string): Promise<void> {
  await loginAs(role)
  await page.goto(url)
}

interface Ref {
  '@id': string
}
interface Constraint {
  'odrl:leftOperand': Ref
  'odrl:operator': Ref
  'odrl:rightOperand'?: Ref | { '@value': string; '@type': string }
}
interface Rule {
  '@type': string
  'odrl:action': Ref
  'odrl:constraint'?: Constraint[]
}
interface TemplateData {
  'dcs:policies': { 'odrl:permission'?: Rule[] }
  'dcs:contractData': { 'dcs:fields'?: { '@id': string }[] }[]
  'dcs:documentStructure': { 'dcs:blocks': { '@list': { 'dcs:title'?: string }[] } }
}

// The SRS Appendix C policy is not hard-coded — the DCS produces a reusable
// access-grant *template* whose spatial and temporal boundaries are negotiated
// fields (the permitted country, the access deadline). Filling those fields at
// contract negotiation with "DE" and the deadline yields exactly the Appendix C
// TempoSpatialAccess agreement. The clause carries the human prose beside its
// machine-readable ODRL meaning, terms picked and never typed.
test('an exhaustive access-grant template lets the Appendix C policy be negotiated', async ({ page, loginAs }) => {
  page.setDefaultTimeout(15_000)
  const clauseTitle = `API access grant ${Date.now()}`

  await gotoAs(page, loginAs, 'Template Creator', '/ui/templates/new')
  await page.getByRole('button', { name: /Component/ }).click()
  await page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox').fill(`FV Access ${Date.now()}`)
  await page.getByRole('tab', { name: /Clauses/ }).click()

  const editor = page.getByTestId('split-clause-editor')
  await editor.getByPlaceholder('Clause title').fill(clauseTitle)

  // Two hub objects become the negotiated boundaries of the access grant.
  const fieldPicker = editor.locator('select').first()
  await fieldPicker.selectOption({ label: 'Company Country' })
  await fieldPicker.selectOption({ label: 'Validity End Date' })

  await editor.locator('.clause-editor').first().click()
  await page.keyboard.type('The assignee may access the service within the agreed country until the agreed date.')

  const ruleSelect = (label: string) =>
    editor.locator('label.form-control').filter({ hasText: label }).locator('select')
  await ruleSelect('Rule').selectOption({ label: 'Permission — the assignee MAY' })
  await ruleSelect('Action').selectOption({ label: 'use' })
  await ruleSelect('Applies to').selectOption({ label: 'The counterparty' })

  // Each context constraint's boundary is one of the negotiated fields
  // (option index 0 is "a fixed value"; 1 and 2 are the two picked fields).
  const addContext = async (operand: string, operator: string, boundaryIndex: number) => {
    await editor.getByRole('button', { name: '+ constraint' }).click()
    const row = editor.locator('.flex.flex-wrap.items-center.gap-1').last()
    await row.locator('select').nth(0).selectOption({ label: operand })
    await row.locator('select').nth(1).selectOption({ label: operator })
    await row.locator('select').nth(2).selectOption({ index: boundaryIndex })
  }
  await addContext('access region (spatial)', 'must equal', 1)
  await addContext('access time (dateTime)', 'must be at most', 2)

  await editor.getByRole('button', { name: 'Add clause', exact: true }).click()
  await expect(editor.getByPlaceholder('Clause title')).toHaveValue('')

  const created = page.waitForRequest((r) => r.url().includes('/template/create') && r.method() === 'POST')
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  const doc = (await created).postDataJSON().template_data as TemplateData

  expect(
    doc['dcs:documentStructure']['dcs:blocks']['@list'].some((b) => b['dcs:title'] === clauseTitle),
    'prose block persisted',
  ).toBeTruthy()

  // The two boundaries are declared as fillable requirement fields.
  const fieldIds = new Set(doc['dcs:contractData'].flatMap((r) => (r['dcs:fields'] ?? []).map((f) => f['@id'])))
  expect(fieldIds.size, 'two negotiated fields declared').toBeGreaterThanOrEqual(2)

  // The Permission's spatial + temporal constraints reference those fields as
  // their boundaries — resolved to the filled values at negotiation.
  const permission = (doc['dcs:policies']['odrl:permission'] ?? []).find(
    (r) => r['@type'] === 'odrl:Permission' && r['odrl:action']['@id'] === 'odrl:use',
  )
  expect(permission, 'a Permission to use').toBeTruthy()

  const byOperand = new Map((permission!['odrl:constraint'] ?? []).map((c) => [c['odrl:leftOperand']['@id'], c]))
  const spatial = byOperand.get('odrl:spatial')
  const dateTime = byOperand.get('odrl:dateTime')
  expect(spatial?.['odrl:operator']['@id'], 'spatial eq').toBe('odrl:eq')
  expect(
    fieldIds.has((spatial?.['odrl:rightOperand'] as Ref)?.['@id']),
    'spatial boundary is a negotiated field',
  ).toBeTruthy()
  expect(dateTime?.['odrl:operator']['@id'], 'dateTime lteq').toBe('odrl:lteq')
  expect(
    fieldIds.has((dateTime?.['odrl:rightOperand'] as Ref)?.['@id']),
    'dateTime boundary is a negotiated field',
  ).toBeTruthy()
})
