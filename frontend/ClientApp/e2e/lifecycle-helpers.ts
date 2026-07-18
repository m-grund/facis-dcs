import type { Page } from '@playwright/test'
import { type DcsRole, expect, test } from './dcs-test'

/**
 * UI lifecycle helpers shared by e2e specs that need a contract in a given
 * state. Extracted from the full-vertical flow: they drive a component
 * template and a contract template through DRAFT → APPROVED → REGISTERED, then
 * derive a contract and take it DRAFT → APPROVED (approved-for-signing) — the
 * precondition of the Secure Contract Viewer.
 */

export type LoginAs = (role: DcsRole) => Promise<void>

export async function gotoAs(page: Page, loginAs: LoginAs, role: DcsRole, url: string): Promise<void> {
  await loginAs(role)
  await page.goto(url)
}

/** Confirms the shared ConfirmationModal (comment/decision-note dialogs). */
async function confirmModal(page: Page, buttonName: 'Submit' | 'Confirm'): Promise<void> {
  const dialog = page.getByRole('dialog').filter({ hasText: 'Confirmation' })
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: buttonName, exact: true }).click()
}

/** Fills the ParticipantSelectionDialog with the local instance DID. */
async function completeParticipantDialog(page: Page): Promise<void> {
  const dialog = page.getByRole('dialog').filter({ hasText: 'Contract Participants' })
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: 'Add local DID' }).click()
  await expect(dialog.getByText(/^did:/).first()).toBeVisible()
  await dialog.getByRole('button', { name: 'Apply', exact: true }).click()
}

async function waitForTemplateLoaded(page: Page, name: string): Promise<void> {
  await expect(page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox')).toHaveValue(name)
}

/** DRAFT → SUBMITTED → REVIEWED → APPROVED for one template, via the UI. */
async function submitReviewApproveTemplate(page: Page, loginAs: LoginAs, did: string, name: string): Promise<void> {
  await test.step(`submit template ${name} for review`, async () => {
    await gotoAs(page, loginAs, 'Template Creator', `/ui/templates/view/${did}`)
    const submitted = page.waitForResponse(
      (r) => r.url().includes('/template/submit') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Submit', exact: true }).click()
    await submitted
  })

  await test.step(`review template ${name}`, async () => {
    await gotoAs(page, loginAs, 'Template Reviewer', `/ui/templates/review/${did}`)
    await waitForTemplateLoaded(page, name)
    const verified = page.waitForResponse(
      (r) => r.url().includes('/template/verify') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Verify', exact: true }).click()
    await verified
    await page.getByRole('dialog').getByRole('button', { name: 'Close', exact: true }).click()
    const forwarded = page.waitForResponse(
      (r) => r.url().includes('/template/submit') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Approve', exact: true }).click()
    await confirmModal(page, 'Submit')
    await forwarded
  })

  await test.step(`approve template ${name}`, async () => {
    await gotoAs(page, loginAs, 'Template Approver', `/ui/templates/approve/${did}`)
    await waitForTemplateLoaded(page, name)
    const approved = page.waitForResponse(
      (r) => r.url().includes('/template/approve') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Approve', exact: true }).click()
    await confirmModal(page, 'Submit')
    await approved
  })
}

/**
 * Builds a fresh contract and drives it to APPROVED (approved-for-signing):
 * a component template with a semantic clause → a contract template composing
 * it → registration → contract derivation → negotiation, review, approval.
 * Returns the approved contract's DID.
 */
export async function buildApprovedContract(page: Page, loginAs: LoginAs): Promise<string> {
  const unique = Date.now()
  const componentName = `SCV Component ${unique}`
  const contractTemplateName = `SCV Contract ${unique}`

  let componentDid = ''
  await test.step('create component template with a semantic clause', async () => {
    await gotoAs(page, loginAs, 'Template Creator', '/ui/templates/new')
    await page.getByRole('button', { name: /Component/ }).click()
    await page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox').fill(componentName)
    await page
      .getByRole('group')
      .filter({ hasText: 'Base Description' })
      .getByRole('textbox')
      .fill('Payment component for the Secure Contract Viewer e2e.')

    await page.getByRole('tab', { name: /Clauses/ }).click()
    const editor = page.getByTestId('split-clause-editor')
    await editor.getByPlaceholder('Clause title').fill('Payment terms')
    await editor.locator('select').first().selectOption({ label: 'Payment Amount' })
    await editor.locator('.clause-editor').first().click()
    await page.keyboard.type('The provider invoices the agreed payment amount.')

    const ruleSelect = (label: string) =>
      editor.locator('label.form-control').filter({ hasText: label }).locator('select')
    await ruleSelect('Rule').selectOption({ label: 'Permission — the assignee MAY' })
    await ruleSelect('Action').selectOption({ label: 'use' })
    await editor.getByRole('button', { name: '+ constraint' }).click()
    const constraint = editor.locator('.flex.flex-wrap.items-center.gap-1').last()
    await constraint.locator('select').nth(0).selectOption({ label: 'Payment Amount' })
    await constraint.locator('select').nth(1).selectOption({ label: 'must be at most' })
    await constraint.locator('input[placeholder="value"]').fill('500')

    await editor.getByRole('button', { name: 'Add clause', exact: true }).click()
    await expect(editor.getByPlaceholder('Clause title')).toHaveValue('')

    const modal = page.getByRole('dialog')
    await page.getByRole('button', { name: 'Place in document' }).first().click()
    await expect(modal.getByText('Selected clause')).toBeVisible()
    await modal.getByRole('button', { name: /Payment terms/ }).click()
    await expect(page.getByRole('dialog')).toBeHidden()

    const created = page.waitForResponse(
      (r) => r.url().includes('/template/create') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Create', exact: true }).click()
    componentDid = ((await (await created).json()) as { did: string }).did
    expect(componentDid).toBeTruthy()
  })

  await submitReviewApproveTemplate(page, loginAs, componentDid, componentName)

  let contractTemplateDid = ''
  await test.step('create contract template from approved component', async () => {
    await gotoAs(page, loginAs, 'Template Creator', '/ui/templates/new')
    await page.getByRole('button', { name: /parent for other contracts/ }).click()
    await page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox').fill(contractTemplateName)
    await page
      .getByRole('group')
      .filter({ hasText: 'Base Description' })
      .getByRole('textbox')
      .fill('Contract template composed for the Secure Contract Viewer e2e.')

    await page.getByText('Component Templates', { exact: true }).click()
    await page.getByPlaceholder('Search templates…').fill(componentName)
    await page.getByRole('button', { name: componentName }).click()
    await expect(page.getByText('No component templates selected yet.')).toBeHidden()

    await page.getByRole('tab', { name: /Builder/ }).click()
    await page
      .getByRole('button', { name: /add.*block/i })
      .first()
      .click()
    const modal = page.getByRole('dialog')
    await expect(modal.getByText('Approved sub-templates:')).toBeVisible()
    await modal.getByText(componentName).first().click()
    await expect(page.getByRole('dialog')).toBeHidden()

    const created = page.waitForResponse(
      (r) => r.url().includes('/template/create') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Create', exact: true }).click()
    contractTemplateDid = ((await (await created).json()) as { did: string }).did
    expect(contractTemplateDid).toBeTruthy()
  })
  await submitReviewApproveTemplate(page, loginAs, contractTemplateDid, contractTemplateName)

  await test.step('register approved contract template', async () => {
    await gotoAs(page, loginAs, 'Template Manager', `/ui/templates/view/${contractTemplateDid}`)
    await waitForTemplateLoaded(page, contractTemplateName)
    const registered = page.waitForResponse(
      (r) => r.url().includes('/template/register') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Register', exact: true }).click()
    await registered
  })

  let contractDid = ''
  await test.step('create contract from registered template', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', '/ui/contracts/new')
    const picker = page.locator('select').first()
    const option = picker.locator('option', { hasText: contractTemplateName })
    await expect(option).toHaveCount(1)
    await picker.selectOption({ label: (await option.textContent())!.trim() })

    await page.getByRole('button', { name: 'Create', exact: true }).click()
    const created = page.waitForResponse((r) => r.url().includes('/contract/create') && r.request().method() === 'POST')
    await completeParticipantDialog(page)
    const createResp = await created
    expect(createResp.ok(), `contract create ${createResp.status()}: ${await createResp.text()}`).toBeTruthy()
    contractDid = ((await createResp.json()) as { did: string }).did
    expect(contractDid).toBeTruthy()
  })

  await test.step('submit contract into negotiation', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/edit/${contractDid}`)
    await expect(page.getByRole('button', { name: 'Update', exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Create', exact: true }).click()
    const submitted = page.waitForResponse((r) => r.url().includes('/contract/submit') && r.request().method() === 'POST')
    await completeParticipantDialog(page)
    const resp = await submitted
    expect(resp.ok(), `contract submit ${resp.status()}: ${await resp.text()}`).toBeTruthy()
  })

  await test.step('accept negotiation', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/negotiate/${contractDid}`)
    const accepted = page.waitForResponse(
      (r) => r.url().includes('/contract/submit') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Submit', exact: true }).click()
    await accepted
  })

  await test.step('review contract', async () => {
    await gotoAs(page, loginAs, 'Contract Reviewer', `/ui/contracts/review/${contractDid}`)
    const forwarded = page.waitForResponse(
      (r) => r.url().includes('/contract/submit') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Approve', exact: true }).click()
    await confirmModal(page, 'Submit')
    await forwarded
  })

  await test.step('approve contract', async () => {
    await gotoAs(page, loginAs, 'Contract Approver', `/ui/contracts/approve/${contractDid}`)
    const approved = page.waitForResponse(
      (r) => r.url().includes('/contract/approve') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Approve', exact: true }).click()
    await confirmModal(page, 'Confirm')
    await approved
  })

  return contractDid
}
