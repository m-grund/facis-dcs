import { execFileSync } from 'node:child_process'
import { homedir, tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import type { Browser, Page } from '@playwright/test'
import { E2E_API_BASE, E2E_DSS_URL, E2E_FRONTEND_ORIGIN, E2E_STATUSLIST_URL } from '../playwright.config'
import { applySession, type DcsRole, expect, mintSession, test } from './dcs-test'

const here = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(here, '../../..')
const python = process.env.E2E_BDD_PYTHON || path.join(homedir(), '.dcs-bdd-venv', 'bin', 'python3')

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
  const dialog = page.getByRole('dialog').filter({ hasText: 'Contract Counterparty' })
  await expect(dialog).toBeVisible()
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

/** The clause prose the fixture lifecycle authors into its Payment component —
 *  asserted verbatim by the specs that render the contract's human-readable
 *  document (dashboards, secure-contract-viewer). */
export const FIXTURE_CLAUSE_PROSE = 'The provider invoices the agreed payment amount.'

/**
 * Authors a Component template with a semantic Payment clause through the real
 * split-clause editor: a titled clause carrying human prose beside its
 * machine-readable ODRL meaning (a Permission bounded by the Payment Amount hub
 * field). When `withPlaceholder` is set it also drops an INLINE, fillable
 * placeholder for Payment Amount into the clause (clicking its building block in
 * the "Available requirements" panel) — the negotiable input a contract derived
 * from this component renders (see multi-dcs authorSemanticComponent; only an
 * inline placeholder renders an editable input, a bare constraint boundary does
 * not). Returns the created component's DID.
 */
async function authorPaymentComponent(
  page: Page,
  loginAs: LoginAs,
  name: string,
  withPlaceholder: boolean,
): Promise<string> {
  await gotoAs(page, loginAs, 'Template Creator', '/ui/templates/new')
  await page.getByRole('button', { name: /Component/ }).click()
  await page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox').fill(name)
  await page
    .getByRole('group')
    .filter({ hasText: 'Base Description' })
    .getByRole('textbox')
    .fill('Payment component for the fixture lifecycle e2e.')

  await page.getByRole('tab', { name: /Clauses/ }).click()
  const editor = page.getByTestId('split-clause-editor')
  await editor.getByPlaceholder('Clause title').fill('Payment terms')
  await editor.locator('select').first().selectOption({ label: 'Payment Amount' })
  await editor.locator('.clause-editor').first().click()
  await page.keyboard.type(FIXTURE_CLAUSE_PROSE)

  if (withPlaceholder) {
    await editor.getByRole('listitem').filter({ hasText: 'Payment Amount' }).first().click()
    // Guard: the inline placeholder span must have landed in the clause editor
    // (ClauseTextEditor renders it as a span with data-parameter-name), else the
    // derived contract carries no negotiable value and the fill spec fails silently.
    await expect(editor.locator('[data-parameter-name]')).toHaveCount(1)
  }

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
  const componentDid = ((await (await created).json()) as { did: string }).did
  expect(componentDid).toBeTruthy()
  return componentDid
}

/** Composes a Contract template by inlining an approved component's blocks,
 *  placeholders and policies into the Builder outline (flatten-on-compose).
 *  Returns its DID. */
async function authorContractTemplateFrom(
  page: Page,
  loginAs: LoginAs,
  name: string,
  componentName: string,
): Promise<string> {
  await gotoAs(page, loginAs, 'Template Creator', '/ui/templates/new')
  await page.getByRole('button', { name: /parent for other contracts/ }).click()
  await page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox').fill(name)
  await page
    .getByRole('group')
    .filter({ hasText: 'Base Description' })
    .getByRole('textbox')
    .fill('Contract template composed for the fixture lifecycle e2e.')

  await page.getByRole('tab', { name: /Builder/ }).click()
  await page
    .getByRole('button', { name: /add.*block/i })
    .first()
    .click()
  const modal = page.getByRole('dialog')
  await expect(modal.getByText('Components (inlined on add):')).toBeVisible()
  await modal.getByPlaceholder('Search components').fill(componentName)
  await modal.getByRole('button', { name: new RegExp(componentName) }).click()
  await expect(page.getByRole('dialog')).toBeHidden()

  const created = page.waitForResponse(
    (r) => r.url().includes('/template/create') && r.request().method() === 'POST' && r.ok(),
  )
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  const contractTemplateDid = ((await (await created).json()) as { did: string }).did
  expect(contractTemplateDid).toBeTruthy()
  return contractTemplateDid
}

/** Registers an approved contract template (publishes it to the Federated
 *  Catalogue) so contracts can be derived from it. */
async function registerContractTemplate(page: Page, loginAs: LoginAs, did: string, name: string): Promise<void> {
  await gotoAs(page, loginAs, 'Template Manager', `/ui/templates/view/${did}`)
  await waitForTemplateLoaded(page, name)
  const registered = page.waitForResponse(
    (r) => r.url().includes('/template/register') && r.request().method() === 'POST' && r.ok(),
  )
  await page.getByRole('button', { name: 'Register', exact: true }).click()
  await registered
}

/** Derives a purely local contract from a registered template (no counterparty:
 *  the R6 dialog is applied empty). The contract lands in DRAFT. Returns its DID. */
async function deriveLocalContract(page: Page, loginAs: LoginAs, templateName: string): Promise<string> {
  await gotoAs(page, loginAs, 'Contract Creator', '/ui/contracts/new')
  const picker = page.locator('select').first()
  const option = picker.locator('option', { hasText: templateName })
  await expect(option).toHaveCount(1)
  await picker.selectOption({ label: (await option.textContent())!.trim() })

  await page.getByRole('button', { name: 'Create', exact: true }).click()
  const created = page.waitForResponse((r) => r.url().includes('/contract/create') && r.request().method() === 'POST')
  await completeParticipantDialog(page)
  const createResp = await created
  expect(createResp.ok(), `contract create ${createResp.status()}: ${await createResp.text()}`).toBeTruthy()
  const contractDid = ((await createResp.json()) as { did: string }).did
  expect(contractDid).toBeTruthy()
  return contractDid
}

/** Identifiers of a UI-authored DRAFT contract fixture — the values the specs
 *  that consumed the old raw-HTTP seed assert against. */
export interface DraftContractFixture {
  contractDid: string
  /** The registered contract template's name; the derived contract inherits it. */
  contractTemplateName: string
  /** The clause prose authored into the fixture's Payment component. */
  clauseProse: string
}

/**
 * Builds a fresh DRAFT contract entirely through the real UI — a Payment
 * component carrying an INLINE fillable placeholder, composed into a registered
 * contract template, then derived into a contract (which lands in DRAFT). The
 * UI-authored replacement for the deleted raw-HTTP seed: a fillable draft the
 * fill/dashboard/storytelling specs assert against, guaranteed to match whatever
 * the current authoring emits.
 */
export async function buildDraftContract(page: Page, loginAs: LoginAs): Promise<DraftContractFixture> {
  const unique = Date.now()
  const componentName = `Fixture Component ${unique}`
  const contractTemplateName = `Fixture Contract ${unique}`

  const componentDid = await test.step('author fixture component with a fillable placeholder', () =>
    authorPaymentComponent(page, loginAs, componentName, true))
  await submitReviewApproveTemplate(page, loginAs, componentDid, componentName)

  const contractTemplateDid = await test.step('compose fixture contract template', () =>
    authorContractTemplateFrom(page, loginAs, contractTemplateName, componentName))
  await submitReviewApproveTemplate(page, loginAs, contractTemplateDid, contractTemplateName)
  await test.step('register fixture contract template', () =>
    registerContractTemplate(page, loginAs, contractTemplateDid, contractTemplateName))

  const contractDid = await test.step('derive DRAFT fixture contract', () =>
    deriveLocalContract(page, loginAs, contractTemplateName))
  return { contractDid, contractTemplateName, clauseProse: FIXTURE_CLAUSE_PROSE }
}

/**
 * Authors buildDraftContract in a throwaway browser context bound to the
 * frontend origin, with its own page and role-session minter — so a spec's
 * beforeAll can build the shared fixture without borrowing a test's page
 * fixture. The authored template + contract persist on the shared backend; the
 * context is closed once authoring is done.
 */
export async function buildDraftContractFixture(browser: Browser): Promise<DraftContractFixture> {
  const context = await browser.newContext({ baseURL: E2E_FRONTEND_ORIGIN })
  const page = await context.newPage()
  const loginAs: LoginAs = async (role) => {
    await applySession(context, page, E2E_FRONTEND_ORIGIN, mintSession(role))
  }
  try {
    return await buildDraftContract(page, loginAs)
  } finally {
    await context.close()
  }
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

  const componentDid = await test.step('create component template with a semantic clause', () =>
    authorPaymentComponent(page, loginAs, componentName, true))
  await submitReviewApproveTemplate(page, loginAs, componentDid, componentName)

  const contractTemplateDid = await test.step('create contract template from approved component', () =>
    authorContractTemplateFrom(page, loginAs, contractTemplateName, componentName))
  await submitReviewApproveTemplate(page, loginAs, contractTemplateDid, contractTemplateName)
  await test.step('register approved contract template', () =>
    registerContractTemplate(page, loginAs, contractTemplateDid, contractTemplateName))

  const contractDid = await test.step('create contract from registered template', () =>
    deriveLocalContract(page, loginAs, contractTemplateName))

  await test.step('fill the required Payment Amount and submit into negotiation', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/edit/${contractDid}`)
    await expect(page.getByRole('button', { name: 'Update', exact: true })).toBeVisible()
    // The Payment Amount placeholder is a required top-level field; approve
    // enforces closedness, so fill it (under the Content tab) before submitting.
    await page
      .getByRole('tab', { name: /content/i })
      .or(page.getByText('Contract Content', { exact: true }))
      .first()
      .click()
    const amount = page
      .getByRole('spinbutton', { name: /amount/i })
      .or(page.getByRole('textbox', { name: /amount/i }))
      .first()
    await expect(amount).toBeVisible({ timeout: 15_000 })
    await amount.fill('250')
    await amount.blur()
    // Submit saves the draft then submits (no separate Update — that navigates
    // away); it appears for a draft and only disables while a save is in flight.
    const submit = page.getByRole('button', { name: 'Submit', exact: true })
    await expect(submit).toBeEnabled({ timeout: 15_000 })
    const submitted = page.waitForResponse(
      (r) => r.url().includes('/contract/submit') && r.request().method() === 'POST',
    )
    await submit.click()
    const resp = await submitted
    expect(resp.ok(), `contract submit ${resp.status()}: ${await resp.text()}`).toBeTruthy()
  })

  await test.step('accept negotiation', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/negotiate/${contractDid}`)
    // Resolve any outstanding decision (Show opens a compare view that disables
    // Submit), then reload so that state clears before submitting.
    const showBtn = page.getByRole('button', { name: 'Show' }).first()
    if (await showBtn.isVisible().catch(() => false)) {
      await showBtn.click()
      const responded = page.waitForResponse(
        (r) => r.url().includes('/contract/respond') && r.request().method() === 'POST' && r.ok(),
      )
      await page.getByRole('button', { name: 'Accept', exact: true }).click()
      await confirmModal(page, 'Confirm')
      await responded
      await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/negotiate/${contractDid}`)
    }
    const submit = page.getByRole('button', { name: 'Submit', exact: true })
    await expect(submit).toBeEnabled({ timeout: 30_000 })
    const accepted = page.waitForResponse(
      (r) => r.url().includes('/contract/submit') && r.request().method() === 'POST' && r.ok(),
    )
    await submit.click()
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

/**
 * Builds a fresh contract and drives it only as far as REVIEWED (forwarded
 * to the Contract Approver) — deliberately NOT approved, so it is left with
 * exactly one OPEN approval task. This is the precondition the continuous-
 * monitoring sweep's MISSING_APPROVAL risk detects (see
 * backend/internal/processauditandcompliance/query/querymonitor.go:
 * RiskTypeMissingApproval flags any Submitted/Reviewed contract carrying an
 * OPEN approval task). Kept separate from buildApprovedContract (rather than
 * refactored out of it) so that function's existing, already-relied-upon
 * behaviour is untouched. Returns the contract's DID.
 */
export async function buildContractPendingApproval(page: Page, loginAs: LoginAs): Promise<string> {
  const unique = Date.now()
  const componentName = `NCI Component ${unique}`
  const contractTemplateName = `NCI Contract ${unique}`

  const componentDid = await test.step('create component template with a semantic clause', () =>
    authorPaymentComponent(page, loginAs, componentName, true))
  await submitReviewApproveTemplate(page, loginAs, componentDid, componentName)

  const contractTemplateDid = await test.step('create contract template from approved component', () =>
    authorContractTemplateFrom(page, loginAs, contractTemplateName, componentName))
  await submitReviewApproveTemplate(page, loginAs, contractTemplateDid, contractTemplateName)
  await test.step('register approved contract template', () =>
    registerContractTemplate(page, loginAs, contractTemplateDid, contractTemplateName))

  const contractDid = await test.step('create contract from registered template', () =>
    deriveLocalContract(page, loginAs, contractTemplateName))

  await test.step('fill the required Payment Amount and submit into negotiation', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/edit/${contractDid}`)
    await expect(page.getByRole('button', { name: 'Update', exact: true })).toBeVisible()
    await page
      .getByRole('tab', { name: /content/i })
      .or(page.getByText('Contract Content', { exact: true }))
      .first()
      .click()
    const amount = page
      .getByRole('spinbutton', { name: /amount/i })
      .or(page.getByRole('textbox', { name: /amount/i }))
      .first()
    await expect(amount).toBeVisible({ timeout: 15_000 })
    await amount.fill('250')
    await amount.blur()
    const submit = page.getByRole('button', { name: 'Submit', exact: true })
    await expect(submit).toBeEnabled({ timeout: 15_000 })
    const submitted = page.waitForResponse(
      (r) => r.url().includes('/contract/submit') && r.request().method() === 'POST',
    )
    await submit.click()
    const resp = await submitted
    expect(resp.ok(), `contract submit ${resp.status()}: ${await resp.text()}`).toBeTruthy()
  })

  await test.step('accept negotiation', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/negotiate/${contractDid}`)
    const showBtn = page.getByRole('button', { name: 'Show' }).first()
    if (await showBtn.isVisible().catch(() => false)) {
      await showBtn.click()
      const responded = page.waitForResponse(
        (r) => r.url().includes('/contract/respond') && r.request().method() === 'POST' && r.ok(),
      )
      await page.getByRole('button', { name: 'Accept', exact: true }).click()
      await confirmModal(page, 'Confirm')
      await responded
      await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/negotiate/${contractDid}`)
    }
    const submit = page.getByRole('button', { name: 'Submit', exact: true })
    await expect(submit).toBeEnabled({ timeout: 30_000 })
    const accepted = page.waitForResponse(
      (r) => r.url().includes('/contract/submit') && r.request().method() === 'POST' && r.ok(),
    )
    await submit.click()
    await accepted
  })

  await test.step('review contract, leaving one OPEN approval task', async () => {
    await gotoAs(page, loginAs, 'Contract Reviewer', `/ui/contracts/review/${contractDid}`)
    const forwarded = page.waitForResponse(
      (r) => r.url().includes('/contract/submit') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Approve', exact: true }).click()
    await confirmModal(page, 'Submit')
    await forwarded
  })

  return contractDid
}

/**
 * Signs an APPROVED contract through the Secure Contract Viewer, as a real
 * signer would (ADR-12): open the contract from the signing list, verify it,
 * run the wallet PID ceremony, download the to-be-signed PDF, sign it
 * externally (the test wallet drives the DSS SCA with its own key, discovering
 * the signature field from the PDF), upload it, and confirm SIGNED. Shared by
 * the full-vertical end-to-end and the specs that need a signed contract.
 */
export async function signApprovedContractViaViewer(page: Page, loginAs: LoginAs, contractDid: string): Promise<void> {
  await gotoAs(page, loginAs, 'Contract Signer', '/ui/signing')
  const row = page.getByRole('row').filter({ hasText: contractDid })
  await expect(row).toBeVisible()
  await row.getByRole('link', { name: /Open/ }).click()
  await expect(page).toHaveURL(/\/signing\/.+/)

  await page.getByRole('button', { name: 'Verify', exact: true }).click()
  await expect(page.getByText('Verified', { exact: true })).toBeVisible()

  const ceremonyStarted = page.waitForResponse(
    (r) => r.url().includes('/signature/request') && r.request().method() === 'POST' && r.ok(),
  )
  // Clicking the step-3 button opens the ceremony dialog; once the wallet
  // presents its PID over the webhook, the viewer fetches the to-be-signed PDF
  // (/signature/prepare) and downloads it.
  const preparedDownload = page.waitForEvent('download')
  await page.getByRole('button', { name: /download document to sign/ }).click()
  const ceremony = (await (await ceremonyStarted).json()) as { ceremony_id: string }
  expect(ceremony.ceremony_id).toBeTruthy()

  execFileSync(python, [path.join(here, 'complete_signing_webhook.py'), ceremony.ceremony_id], {
    cwd: repoRoot,
    env: { ...process.env, STATUSLIST_SERVICE_URL: E2E_STATUSLIST_URL, BDD_DCS_BASE_URL: E2E_API_BASE },
    stdio: 'pipe',
  })

  const preparedPath = (await (await preparedDownload).path())!
  const signedPath = path.join(tmpdir(), `signed-${ceremony.ceremony_id}.pdf`)
  // No E2E_SIGN_FIELD: the wallet discovers the pre-placed field from the PDF.
  execFileSync(python, [path.join(here, 'sign_prepared_pdf.py'), preparedPath, signedPath], {
    cwd: repoRoot,
    env: { ...process.env, DSS_URL: E2E_DSS_URL, E2E_SIGNATORY: 'E2E Vertical Signer' },
    stdio: 'pipe',
  })

  await page.locator('input[type="file"]').setInputFiles(signedPath)
  await expect(page.getByText('SIGNED', { exact: true })).toBeVisible({ timeout: 120_000 })
}
