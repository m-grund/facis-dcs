import { expect, seededFixtures, test } from './dcs-test'

/**
 * The contract fill flow's semantic boundary, through the real edit UI: a
 * value typed into a placeholder input is emitted as a canonical
 * dcs:semanticConditionValues entry binding to its requirement field by IRI
 * (forField — the same IRI the ODRL constraint names as odrl:leftOperand),
 * with the editor-internal (blockId, conditionId, parameterName) tuple never
 * leaking into the document, and the unsigned contract's policy set staying
 * an odrl:Offer.
 */

test('filling a placeholder emits a forField-bound value in an odrl:Offer document', async ({ page, loginAs }) => {
  const { contractDid } = seededFixtures()
  await loginAs('Contract Creator')
  await page.goto(`/ui/contracts/edit/${contractDid}`)

  // The fill inputs live under the Contract Content tab; the seeded fixture
  // binds one placeholder to the coverage requirement field, and the input
  // is labeled with the field's parameter name.
  await page.getByRole('tab', { name: /content/i }).or(page.getByText('Contract Content', { exact: true })).first().click()
  const input = page.getByRole('textbox', { name: 'coverage' }).first()
  await expect(input).toBeVisible()
  await input.fill('97')

  const updateRequest = page.waitForRequest((r) => r.url().includes('/contract/update') && r.method() === 'PUT')
  await page.getByRole('button', { name: 'Update', exact: true }).click()
  const payload = (await updateRequest).postDataJSON() as {
    contract_data: {
      semanticConditionValues?: Record<string, unknown>[]
      'dcs:policies': { '@type': string }
    }
  }

  const values = payload.contract_data.semanticConditionValues ?? []
  expect(values.length, 'the filled value is in the document').toBeGreaterThan(0)
  for (const value of values) {
    expect(value.forField, 'values bind to their requirement field by IRI').toBeTruthy()
    expect(value, 'editor-internal keys never leak into the document').not.toHaveProperty('conditionId')
    expect(value, 'editor-internal keys never leak into the document').not.toHaveProperty('parameterName')
  }
  const coverage = values.find((v) => String(v.forField).includes('field-provider-coverage'))
  expect(coverage?.parameterValue).toBe('97')

  expect(payload.contract_data['dcs:policies']['@type'], 'unsigned contracts stay an odrl:Offer').toBe('odrl:Offer')
})
