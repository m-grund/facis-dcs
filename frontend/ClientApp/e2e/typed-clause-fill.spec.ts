import { expect, seededFixtures, test } from './dcs-test'

/**
 * Contract-time typed clause fill (DCS-FR-TR-03/TR-12, DCS-FR-CWE-04): a
 * contract derived from a template with a hub typed clause exposes its
 * values for shape-driven editing in the content tab; saving regenerates
 * the human-readable summary from the machine-readable instance, and the
 * persisted document carries both in sync.
 */

const DCS_AMOUNT = 'https://w3id.org/facis/dcs/ontology/v1#amount'

interface TypedClauseBlock {
  '@type': string
  'dcs:content'?: { '@list': unknown[] }
  'dcs:typedClause'?: Record<string, unknown>
}

test('editing a typed clause updates instance and summary together', async ({ page, loginAs }) => {
  const { typedContractDid } = seededFixtures()
  await loginAs('Contract Creator')
  await page.goto(`/ui/contracts/edit/${typedContractDid}`)

  await page
    .getByRole('tab', { name: /content/i })
    .or(page.getByText('Contract Content', { exact: true }))
    .first()
    .click()

  await page.getByRole('button', { name: 'Edit typed values' }).click()

  // The shacl-form renders the PaymentClause shape's widgets from the hub's
  // Turtle; the amount input carries the sh:path's local name as its label.
  const shaclForm = page.locator('shacl-form')
  await expect(shaclForm).toBeVisible()
  const amountInput = shaclForm.locator('input[type="number"]').first()
  await expect(amountInput).toBeVisible()
  await amountInput.fill('250')

  await page.getByRole('button', { name: 'Save values' }).click()

  // The regenerated human-readable summary shows the new value inline.
  await expect(page.getByText(/amount: 250/).first()).toBeVisible()

  const updateRequest = page.waitForRequest((r) => r.url().includes('/contract/update') && r.method() === 'PUT')
  await page.getByRole('button', { name: 'Update', exact: true }).click()
  const payload = (await updateRequest).postDataJSON() as {
    contract_data: {
      'dcs:documentStructure': { 'dcs:blocks': { '@list': TypedClauseBlock[] } }
    }
  }

  const clause = payload.contract_data['dcs:documentStructure']['dcs:blocks']['@list'].find(
    (block) => !!block['dcs:typedClause'],
  )
  expect(clause, 'the typed clause survives the round-trip').toBeTruthy()
  const amount = clause?.['dcs:typedClause']?.[DCS_AMOUNT] as { '@value'?: string } | string | undefined
  const amountValue = typeof amount === 'object' ? amount?.['@value'] : amount
  expect(String(amountValue), 'machine-readable instance carries the edited value').toBe('250')
  expect(JSON.stringify(clause?.['dcs:content']), 'human-readable summary is regenerated from the instance').toContain(
    'amount: 250',
  )
})
