import { expect, test } from './dcs-test'

/**
 * The template builder's Semantic Hub integration, end to end through the
 * real UI: the typed-clause palette comes from the hub's clause catalog,
 * the form is generated from the hub's SHACL shapes (shacl-form), and the
 * saved document is the canonical JSON-LD envelope — one enclosing
 * odrl:Offer whose machine rules are backed by human-readable prose
 * (dcs:prose), with no counterparty bound before signing.
 */

test('typed clause from the hub palette lands as a prose-backed ODRL rule in an odrl:Offer', async ({
  page,
  loginAs,
}) => {
  await loginAs('Template Creator')
  await page.goto('/ui/templates/new')

  // Component templates carry typed clauses (contract templates compose components).
  await page.getByRole('button', { name: /Component/ }).click()

  await page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox').fill('E2E Typed Clause Template')

  // Builder tab → add a block at the root.
  await page.getByRole('tab', { name: /Builder/ }).click()
  await page.getByRole('button', { name: /add.*block/i }).first().click()

  const modal = page.getByRole('dialog')
  await expect(modal.getByRole('heading', { name: 'Add block' })).toBeVisible()
  await expect(modal.getByText('Typed clauses (Semantic Hub):')).toBeVisible()

  // Pick the first hub-served typed clause; the form below is rendered by
  // shacl-form from the hub's SHACL shapes — no hardcoded fields.
  const paletteButtons = modal.locator('button.btn-xs')
  await expect(paletteButtons.first()).toBeVisible()
  await paletteButtons.first().click()

  const shaclForm = modal.locator('shacl-form')
  await expect(shaclForm).toBeVisible()

  // Fill every text/number input the shape generated with a plausible value.
  const inputs = shaclForm.locator('input[type="text"], input[type="number"], input:not([type])')
  const count = await inputs.count()
  for (let i = 0; i < count; i++) {
    const input = inputs.nth(i)
    const type = await input.getAttribute('type')
    await input.fill(type === 'number' ? '42' : 'E2E value')
  }

  await modal.getByRole('button', { name: 'Add to document' }).click()
  await expect(page.getByRole('dialog')).toBeHidden()

  // Save and capture the canonical envelope the UI emits.
  const createRequest = page.waitForRequest((r) => r.url().includes('/template/create') && r.method() === 'POST')
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  const payload = (await createRequest).postDataJSON() as {
    template_data: {
      '@type': string
      'dcs:policies': Record<string, unknown>
      'dcs:documentStructure': { 'dcs:blocks': { '@list': Record<string, unknown>[] } }
    }
  }

  const doc = payload.template_data
  expect(doc['@type']).toBe('dcs:ContractTemplate')

  const policies = doc['dcs:policies'] as {
    '@type': string
    'odrl:profile': { '@id': string }
    'odrl:obligation'?: Record<string, unknown>[]
    'odrl:permission'?: Record<string, unknown>[]
    'odrl:prohibition'?: Record<string, unknown>[]
  }
  expect(policies['@type'], 'a template offers, it does not bind parties').toBe('odrl:Offer')
  expect(policies['odrl:profile']['@id']).toContain('odrl-profile')
  expect(policies, 'no uid key — the policy identity is its @id').not.toHaveProperty('uid')

  const rules = [
    ...(policies['odrl:obligation'] ?? []),
    ...(policies['odrl:permission'] ?? []),
    ...(policies['odrl:prohibition'] ?? []),
  ]
  expect(rules.length, 'the typed clause produced at least one machine rule').toBeGreaterThan(0)
  for (const rule of rules) {
    expect(rule['odrl:action'], 'every rule declares its action').toBeTruthy()
    expect(rule['dcs:prose'], 'every machine rule is backed by human-readable prose').toBeTruthy()
    expect(rule['odrl:assigner'], 'rule parties are open role references on an Offer').toBeTruthy()
    expect(rule['odrl:assignee']).toBeTruthy()
    expect(rule['odrl:target']).toBeTruthy()
  }

  // The prose backing points at a block that exists in the document.
  const blockIds = new Set(doc['dcs:documentStructure']['dcs:blocks']['@list'].map((b) => b['@id']))
  for (const rule of rules) {
    const prose = rule['dcs:prose'] as { '@id': string }
    expect(blockIds, 'dcs:prose dereferences within the document').toContain(prose['@id'])
  }
})
