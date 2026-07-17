import { expect, seededFixtures, test } from './dcs-test'

/**
 * The contract fill flow's semantic boundary, through the real edit UI: a
 * value typed into a placeholder input is carried inline on the requirement
 * field it fills (dcs:parameterValue on the dcs:RequirementField an ODRL
 * constraint names as its odrl:leftOperand), with the editor-internal
 * (blockId, conditionId, parameterName) tuple never leaking into the document,
 * and the unsigned contract's policy set staying an odrl:Offer.
 */

interface RequirementField {
  '@id'?: string
  'dcs:parameterName'?: string
  'dcs:parameterValue'?: unknown
}

test('filling a placeholder writes the value inline on the requirement field of an odrl:Offer', async ({
  page,
  loginAs,
}) => {
  const { contractDid } = seededFixtures()
  await loginAs('Contract Creator')
  await page.goto(`/ui/contracts/edit/${contractDid}`)

  // The fill inputs live under the Contract Content tab; the seeded fixture
  // binds one placeholder to the coverage requirement field, and the input
  // is labeled with the field's parameter name.
  await page
    .getByRole('tab', { name: /content/i })
    .or(page.getByText('Contract Content', { exact: true }))
    .first()
    .click()
  const input = page.getByRole('textbox', { name: 'coverage' }).first()
  await expect(input).toBeVisible()
  await input.fill('97')

  const updateRequest = page.waitForRequest((r) => r.url().includes('/contract/update') && r.method() === 'PUT')
  await page.getByRole('button', { name: 'Update', exact: true }).click()
  const payload = (await updateRequest).postDataJSON() as {
    contract_data: {
      'dcs:contractData'?: { 'dcs:fields'?: RequirementField[] }[]
      'dcs:policies': { '@type': string }
    }
  }

  const fields = (payload.contract_data['dcs:contractData'] ?? []).flatMap(
    (requirement) => requirement['dcs:fields'] ?? [],
  )
  const coverage = fields.find((field) => field['dcs:parameterName'] === 'coverage')
  expect(coverage, 'the coverage requirement field is in the document').toBeTruthy()
  // The value lives inline on the field, not in a separate values array; the
  // decimal-typed field yields a NUMBER, not a string.
  expect(coverage!['dcs:parameterValue'], 'the filled value is carried inline on the field').toBe(97)

  expect(payload.contract_data['dcs:policies']['@type'], 'unsigned contracts stay an odrl:Offer').toBe('odrl:Offer')
})
