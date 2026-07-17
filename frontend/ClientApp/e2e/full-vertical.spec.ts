import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { homedir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import type { Page } from '@playwright/test'
import { E2E_API_BASE, E2E_STATUSLIST_URL } from '../playwright.config'
import { type DcsRole, expect, test } from './dcs-test'

/**
 * Full vertical: a component template with a hub typed clause travels the
 * whole product surface through the real UI — build, review, approval,
 * composition into a contract template, registration, contract derivation,
 * negotiation, review/approval, signing (the wallet leg arrives over the
 * wallet's own webhook channel), PDF/bundle export, and audit.
 */

const here = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(here, '../../..')
const python = process.env.E2E_BDD_PYTHON || path.join(homedir(), '.dcs-bdd-venv', 'bin', 'python3')

type LoginAs = (role: DcsRole) => Promise<void>

/**
 * Navigate with a freshly minted role session. Hydra rotates refresh tokens
 * single-use, so the router guard's on-boot refresh can only be spent once
 * per session; a long many-navigation test therefore re-mints before every
 * top-level navigation, so each page boots with an unexpired access token
 * and the guard never has to refresh (nor bounce to the login challenge).
 */
async function gotoAs(page: Page, loginAs: LoginAs, role: DcsRole, url: string): Promise<void> {
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
  // One entry per list (reviewers/approvers/negotiators) once the DID landed.
  await expect(dialog.getByText(/^did:/).first()).toBeVisible()
  await dialog.getByRole('button', { name: 'Apply', exact: true }).click()
}

/** Waits until the template detail view finished loading (name populated). */
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
    // The backend accepts the reviewer recommendation only after a
    // verification run — the Verify dialog is part of the review flow.
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

test('full vertical through the real UI', async ({ page, loginAs }) => {
  test.setTimeout(600_000)
  page.setDefaultTimeout(15_000)

  const unique = Date.now()
  const componentName = `FV Component ${unique}`
  const contractTemplateName = `FV Contract ${unique}`

  // ---- Stage 1: Template Creator builds a Component template ----
  let componentDid = ''
  await test.step('create component template with typed clause + prose', async () => {
    await gotoAs(page, loginAs, 'Template Creator', '/ui/templates/new')

    // Component templates carry typed clauses (contract templates compose components).
    await page.getByRole('button', { name: /Component/ }).click()
    await page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox').fill(componentName)
    // An empty description is a verification finding at review time.
    await page
      .getByRole('group')
      .filter({ hasText: 'Base Description' })
      .getByRole('textbox')
      .fill('Payment component for the full vertical.')

    await page.getByRole('tab', { name: /Builder/ }).click()
    await page
      .getByRole('button', { name: /add.*block/i })
      .first()
      .click()

    const modal = page.getByRole('dialog')
    await expect(modal.getByRole('heading', { name: 'Add block' })).toBeVisible()
    await expect(modal.getByText('Typed clauses (Semantic Hub):')).toBeVisible()

    // The palette label comes from the hub's clause catalog — look the
    // PaymentClause entry up the same way the palette does (metadata read
    // only; every app action stays in the UI).
    const catalog = (await (await page.request.get('/api/semantic/clauses')).json()) as {
      clauses?: { type: string; label: string }[]
    }
    const paymentClause = (catalog.clauses ?? []).find(
      (c) => c.type === 'dcs:PaymentClause' || c.type.endsWith('#PaymentClause'),
    )
    expect(paymentClause, 'the hub catalog serves the PaymentClause').toBeTruthy()
    await modal.getByRole('button', { name: paymentClause!.label, exact: true }).click()

    const shaclForm = modal.locator('shacl-form')
    await expect(shaclForm).toBeVisible()
    // amount (xsd:integer, first property of the shape) and currency
    // (sh:in EUR/USD renders as shacl-form's filterable combobox).
    await shaclForm.locator('input[type="number"]').first().fill('100')
    await shaclForm.getByPlaceholder('Type to filter list...').first().click()
    await modal.getByRole('option', { name: 'EUR', exact: true }).click()

    await modal.getByRole('button', { name: 'Add to document' }).click()
    await expect(page.getByRole('dialog')).toBeHidden()

    // A prose text block alongside the typed clause (a populated outline
    // offers per-block insert buttons instead of the empty-state add).
    await page.getByRole('button', { name: 'Insert block below' }).first().click()
    await modal.getByRole('button', { name: 'Text', exact: true }).click()
    await expect(page.getByRole('dialog')).toBeHidden()
    await page.locator('textarea').last().fill('Prose block for the full vertical.')

    const created = page.waitForResponse(
      (r) => r.url().includes('/template/create') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Create', exact: true }).click()
    componentDid = ((await (await created).json()) as { did: string }).did
    expect(componentDid).toBeTruthy()
  })

  // ---- Stage 2: submit → review → approve the component ----
  await submitReviewApproveTemplate(page, loginAs, componentDid, componentName)

  // ---- Stage 3: Contract Template composing the approved component ----
  let contractTemplateDid = ''
  await test.step('create contract template from approved component', async () => {
    await gotoAs(page, loginAs, 'Template Creator', '/ui/templates/new')
    await page.getByRole('button', { name: /parent for other contracts/ }).click()
    await page.getByRole('group').filter({ hasText: 'Global Name' }).getByRole('textbox').fill(contractTemplateName)
    await page
      .getByRole('group')
      .filter({ hasText: 'Base Description' })
      .getByRole('textbox')
      .fill('Contract template composed for the full vertical.')

    // Pin the approved component as a sub-template snapshot (Details tab picker).
    await page.getByText('Component Templates', { exact: true }).click()
    await page.getByPlaceholder('Search templates…').fill(componentName)
    await page.getByRole('button', { name: componentName }).click()
    await expect(page.getByText('No component templates selected yet.')).toBeHidden()

    // Reference it in the document outline (Builder tab).
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

  // ---- Stage 4: Template Manager registers the contract template ----
  await test.step('register approved contract template', async () => {
    await gotoAs(page, loginAs, 'Template Manager', `/ui/templates/view/${contractTemplateDid}`)
    await waitForTemplateLoaded(page, contractTemplateName)
    const registered = page.waitForResponse(
      (r) => r.url().includes('/template/register') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Register', exact: true }).click()
    await registered
  })

  // ---- Stage 5: Contract Creator derives a contract ----
  let contractDid = ''
  await test.step('create contract from registered template', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', '/ui/contracts/new')
    // The template picker is a plain <select> with "Version {n} - {name}" options.
    const picker = page.locator('select').first()
    const option = picker.locator('option', { hasText: contractTemplateName })
    await expect(option).toHaveCount(1)
    await picker.selectOption({ label: (await option.textContent())!.trim() })

    // The create trigger is the ParticipantSelectionDialog instance.
    await page.getByRole('button', { name: 'Create', exact: true }).click()
    const created = page.waitForResponse((r) => r.url().includes('/contract/create') && r.request().method() === 'POST')
    await completeParticipantDialog(page)
    const createResp = await created
    expect(createResp.ok(), `contract create ${createResp.status()}: ${await createResp.text()}`).toBeTruthy()
    contractDid = ((await createResp.json()) as { did: string }).did
    expect(contractDid).toBeTruthy()
  })

  // ---- Stage 6: the SHACL-typed clause travelled hub → component →
  //      contract template → contract, and persists on the contract doc ----
  await test.step('typed clause travels into the contract document', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/edit/${contractDid}`)
    await page
      .getByRole('tab', { name: /content/i })
      .or(page.getByText('Contract Content', { exact: true }))
      .first()
      .click()
    // The component's typed clause renders through its regenerated
    // human-readable summary (composed sub-template clauses are immutable at
    // contract time, so there is no in-place editor here — the machine
    // instance rode along from the hub shape).
    await expect(page.getByText(/amount: 100 · currency: EUR/).first()).toBeVisible()

    const updated = page.waitForRequest((r) => r.url().includes('/contract/update') && r.method() === 'PUT')
    await page.getByRole('button', { name: 'Update', exact: true }).click()
    const payload = JSON.stringify((await updated).postDataJSON())
    expect(payload, 'the machine-readable typed instance rides along').toContain('PaymentClause')
  })

  // ---- Stage 7: DRAFT → NEGOTIATION → SUBMITTED → REVIEWED → APPROVED ----
  await test.step('submit contract into negotiation', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/edit/${contractDid}`)
    // The submit trigger is the second ParticipantSelectionDialog instance —
    // its trigger button is labeled "Create" (component-fixed label). Wait for
    // the contract to load so the DRAFT-only submit control has rendered.
    await expect(page.getByRole('button', { name: 'Update', exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Create', exact: true }).click()
    const submitted = page.waitForResponse(
      (r) => r.url().includes('/contract/submit') && r.request().method() === 'POST',
    )
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

  // ---- Stage 8: signing ceremony, wallet leg over its own channel ----
  await test.step('sign contract', async () => {
    await gotoAs(page, loginAs, 'Contract Signer', '/ui/signing')
    const row = page.getByRole('row').filter({ hasText: contractDid })
    await expect(row).toBeVisible()
    await expect(row.getByText('UNSIGNED')).toBeVisible()

    const ceremonyStarted = page.waitForResponse(
      (r) => r.url().includes('/signature/request') && r.request().method() === 'POST' && r.ok(),
    )
    await row.getByRole('button', { name: 'Sign', exact: true }).click()
    const ceremony = (await (await ceremonyStarted).json()) as { ceremony_id: string }
    expect(ceremony.ceremony_id).toBeTruthy()

    // The wallet presents its PID over the webhook channel; the ceremony
    // dialog's poll then sees "verified" and the view applies the signature.
    execFileSync(python, [path.join(here, 'complete_signing_webhook.py'), ceremony.ceremony_id], {
      cwd: repoRoot,
      env: { ...process.env, STATUSLIST_SERVICE_URL: E2E_STATUSLIST_URL, BDD_DCS_BASE_URL: E2E_API_BASE },
      stdio: 'pipe',
    })

    await expect(row.getByText('SIGNED', { exact: true })).toBeVisible({ timeout: 120_000 })
  })

  // ---- Stage 9: Contract Manager exports PDF + evidence bundle ----
  await test.step('export PDF and bundle', async () => {
    await gotoAs(page, loginAs, 'Contract Manager', `/ui/contracts/view/${contractDid}`)

    const pdfDownload = page.waitForEvent('download')
    await page.getByRole('button', { name: 'Export PDF' }).click()
    const pdfBytes = readFileSync((await (await pdfDownload).path())!)
    expect(pdfBytes.subarray(0, 5).toString('latin1')).toBe('%PDF-')

    const bundleDownload = page.waitForEvent('download')
    await page.getByRole('button', { name: 'Export bundle' }).click()
    const bundleBytes = readFileSync((await (await bundleDownload).path())!)
    expect(bundleBytes.subarray(0, 2).toString('latin1')).toBe('PK')
  })

  // ---- Stage 10: Auditor runs a scoped audit over the contract ----
  await test.step('scoped audit references the contract', async () => {
    await gotoAs(page, loginAs, 'Auditor', '/ui/audit')
    await page.getByLabel('Scope').selectOption('contracts')
    // Scope to this contract's own trail (a whole-corpus audit walks every
    // contract's IPFS trail — far slower and unnecessary here).
    await page.getByLabel('DID (optional)').fill(contractDid)
    await page.getByLabel('Audit justification').fill('Full-vertical E2E audit')
    // The audit reads the contract's lifecycle trail from IPFS; right after a
    // fresh sign+export the document manager can briefly lag before the trail
    // is retrievable, so retry the run until it comes back ok.
    await expect(async () => {
      const audited = page.waitForResponse((r) => r.url().includes('/pac/audit') && r.request().method() === 'POST', {
        timeout: 90_000,
      })
      await page.getByRole('button', { name: 'Execute Audit' }).click()
      const auditResp = await audited
      expect(auditResp.ok(), `audit ${auditResp.status()}: ${await auditResp.text()}`).toBeTruthy()
    }).toPass({ timeout: 150_000, intervals: [5_000, 10_000, 20_000] })
    // The freshly signed contract's lifecycle events are in the audit trail.
    await expect(page.getByRole('cell', { name: contractDid }).first()).toBeVisible({ timeout: 60_000 })
  })
})
