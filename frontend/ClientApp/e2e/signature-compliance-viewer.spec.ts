import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { homedir, tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import type { Page } from '@playwright/test'
import { E2E_API_BASE, E2E_DSS_URL, E2E_STATUSLIST_URL } from '../playwright.config'
import { type DcsRole, expect, seededFixtures, test } from './dcs-test'

/**
 * Signature Compliance Viewer (DCS-FR-SM-05/-07/-08, DCS-FR-SM-18/-21/-26).
 *
 * Drives the seeded draft contract to SIGNED through the real signing flow (the
 * wallet leg over the webhook + the external SCA sign, exactly as
 * full-vertical.spec.ts does), then opens the dedicated tabbed viewer and
 * exercises every tab — asserting the DSS-report + embedded-VC metadata this
 * feature surfaces (signer identity, signature level, timestamp, cryptographic
 * integrity, QES/AES + credential-status flags) actually renders, plus the
 * PDF + JSON report export.
 *
 * NOTE: this asserts the enriched validate/compliance responses and the new
 * /signature/view fields — it must be run against a build carrying those
 * backend changes (merge + redeploy first).
 */

const here = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(here, '../../..')
const python = process.env.E2E_BDD_PYTHON || path.join(homedir(), '.dcs-bdd-venv', 'bin', 'python3')

type LoginAs = (role: DcsRole) => Promise<void>

/** Navigate with a freshly minted role session (Hydra rotates refresh tokens
 *  single-use, so each top-level navigation re-mints — mirrors full-vertical). */
async function gotoAs(page: Page, loginAs: LoginAs, role: DcsRole, url: string): Promise<void> {
  await loginAs(role)
  await page.goto(url)
}

async function confirmModal(page: Page, buttonName: 'Submit' | 'Confirm'): Promise<void> {
  const dialog = page.getByRole('dialog').filter({ hasText: 'Confirmation' })
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: buttonName, exact: true }).click()
}

/** Drives the seeded DRAFT contract through NEGOTIATION → REVIEWED → APPROVED →
 *  SIGNED via the UI, and returns its DID. Reuses the seeded fixture and the
 *  Python wallet/SCA helpers rather than rebuilding templates from scratch. */
async function signSeededContract(page: Page, loginAs: LoginAs): Promise<string> {
  const { contractDid } = seededFixtures()

  await test.step('submit contract into negotiation', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/edit/${contractDid}`)
    await expect(page.getByRole('button', { name: 'Update', exact: true })).toBeVisible()
    // The submit trigger is the ParticipantSelectionDialog instance labeled "Create".
    await page.getByRole('button', { name: 'Create', exact: true }).click()
    const submitted = page.waitForResponse((r) => r.url().includes('/contract/submit') && r.request().method() === 'POST')
    const dialog = page.getByRole('dialog').filter({ hasText: 'Contract Participants' })
    await expect(dialog).toBeVisible()
    await dialog.getByRole('button', { name: 'Add local DID' }).click()
    await expect(dialog.getByText(/^did:/).first()).toBeVisible()
    await dialog.getByRole('button', { name: 'Apply', exact: true }).click()
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

  await test.step('sign contract', async () => {
    await gotoAs(page, loginAs, 'Contract Signer', '/ui/signing')
    const row = page.getByRole('row').filter({ hasText: contractDid })
    await expect(row).toBeVisible()

    const ceremonyStarted = page.waitForResponse(
      (r) => r.url().includes('/signature/request') && r.request().method() === 'POST' && r.ok(),
    )
    const preparedDownload = page.waitForEvent('download')
    await row.getByRole('button', { name: 'Sign', exact: true }).click()
    const ceremony = (await (await ceremonyStarted).json()) as { ceremony_id: string }
    expect(ceremony.ceremony_id).toBeTruthy()

    // The wallet presents its PID over its own webhook channel.
    execFileSync(python, [path.join(here, 'complete_signing_webhook.py'), ceremony.ceremony_id], {
      cwd: repoRoot,
      env: { ...process.env, STATUSLIST_SERVICE_URL: E2E_STATUSLIST_URL, BDD_DCS_BASE_URL: E2E_API_BASE },
      stdio: 'pipe',
    })

    // The signatory signs the prepared PDF externally (test wallet drives the
    // DSS SCA with its own key) and uploads it; the DCS validates and records it.
    const preparedPath = (await (await preparedDownload).path())!
    const signedPath = path.join(tmpdir(), `signed-compliance-${ceremony.ceremony_id}.pdf`)
    execFileSync(python, [path.join(here, 'sign_prepared_pdf.py'), preparedPath, signedPath], {
      cwd: repoRoot,
      env: {
        ...process.env,
        DSS_URL: E2E_DSS_URL,
        E2E_SIGNATORY: 'E2E Vertical Signer',
        E2E_SIGN_FIELD: 'Signature1',
      },
      stdio: 'pipe',
    })
    await row.locator('input[type="file"]').setInputFiles(signedPath)
    await expect(row.getByText('SIGNED', { exact: true })).toBeVisible({ timeout: 120_000 })
  })

  return contractDid
}

/** Opens the viewer for the given role and selects the contract in the list. */
async function openViewer(page: Page, loginAs: LoginAs, role: DcsRole, contractDid: string): Promise<void> {
  await gotoAs(page, loginAs, role, '/ui/compliance')
  await page.getByPlaceholder('Search DID or name…').fill(contractDid)
  const item = page.getByRole('button').filter({ hasText: contractDid })
  await expect(item.first()).toBeVisible()
  const viewLoaded = page.waitForResponse(
    (r) => r.url().includes('/signature/view') && r.request().method() === 'GET' && r.ok(),
  )
  await item.first().click()
  await viewLoaded
}

test('signature compliance viewer surfaces DSS + embedded-VC metadata', async ({ page, loginAs }) => {
  test.setTimeout(600_000)
  page.setDefaultTimeout(15_000)

  const contractDid = await signSeededContract(page, loginAs)

  // ---- Contract Manager: Validation, Compliance Checks, export, Revocation ----
  await test.step('validation tab surfaces DSS report + cryptographic integrity', async () => {
    await openViewer(page, loginAs, 'Contract Manager', contractDid)

    const validated = page.waitForResponse(
      (r) => r.url().includes('/signature/validate') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Validate', exact: true }).click()
    await validated

    // The DSS report (ETSI EN 319 102-1) — signer identity, signature level,
    // timestamp — is exactly the metadata FR-SM-26 requires.
    await expect(page.getByText('EU DSS Validation (ETSI EN 319 102-1)')).toBeVisible()
    await expect(page.getByText('Signer identity:')).toBeVisible()
    await expect(page.getByText('Signature level:')).toBeVisible()
    await expect(page.getByText('Timestamp:')).toBeVisible()

    // The cryptographic-integrity findings section renders with pass/fail rows.
    await expect(page.getByRole('heading', { name: 'Cryptographic Integrity' })).toBeVisible()
    await expect(page.locator('.badge').filter({ hasText: /PASS|FAIL/ }).first()).toBeVisible()
  })

  await test.step('compliance checks tab flags signature level + credential status', async () => {
    await page.getByRole('tab', { name: 'Compliance Checks' }).click()

    const checked = page.waitForResponse(
      (r) => r.url().includes('/signature/compliance') && r.request().method() === 'POST' && r.ok(),
    )
    await page.getByRole('button', { name: 'Run Compliance', exact: true }).click()
    await checked

    // FR-SM-21: signature level (AES) and credential status (ACTIVE) per signature.
    await expect(page.locator('.badge').filter({ hasText: 'AES' }).first()).toBeVisible()
    await expect(page.locator('.badge').filter({ hasText: 'ACTIVE' }).first()).toBeVisible()
  })

  await test.step('export compliance report as JSON and PDF', async () => {
    const jsonDownload = page.waitForEvent('download')
    await page.getByRole('button', { name: 'Export JSON' }).click()
    const jsonPath = (await (await jsonDownload).path())!
    const report = JSON.parse(readFileSync(jsonPath, 'utf-8')) as {
      contract_did: string
      signatures: unknown[]
    }
    expect(report.contract_did).toBe(contractDid)
    expect(report.signatures.length).toBeGreaterThan(0)

    // PDF export opens a print window (dependency-free, browser "Save as PDF").
    const popup = page.waitForEvent('popup')
    await page.getByRole('button', { name: 'Export PDF' }).click()
    expect(await popup).toBeTruthy()
  })

  // ---- Auditor: Audit Reports tab ----
  await test.step('audit reports tab loads the audit trail', async () => {
    await openViewer(page, loginAs, 'Auditor', contractDid)
    await page.getByRole('tab', { name: 'Audit Reports' }).click()

    const audited = page.waitForResponse(
      (r) => r.url().includes('/signature/audit') && r.request().method() === 'GET',
      { timeout: 90_000 },
    )
    await page.getByRole('button', { name: 'Load Audit Report', exact: true }).click()
    const resp = await audited
    // The audit trail lives only in IPFS and intermittently loses a just-written
    // entry ("DataIdentifier not found") — tolerate that one infra flake, stay
    // strict on any other failure (mirrors full-vertical's audit stage).
    if (resp.ok()) {
      await expect(page.getByRole('table')).toBeVisible()
    } else {
      const body = await resp.text()
      const ipfsTrailMiss = body.includes('ipfs could not find') || body.includes('DataIdentifier not found')
      expect(ipfsTrailMiss, `audit ${resp.status()}: ${body}`).toBeTruthy()
      test.info().annotations.push({ type: 'known-flake', description: `audit tolerated an IPFS trail miss: ${body}` })
    }
  })

  // ---- Revocation (destructive — last) ----
  await test.step('revocation tab revokes the signature', async () => {
    await openViewer(page, loginAs, 'Contract Manager', contractDid)
    await page.getByRole('tab', { name: 'Revocation' }).click()

    const sigRow = page.getByRole('row').filter({ hasText: 'ACTIVE' }).first()
    await expect(sigRow).toBeVisible()
    const revoked = page.waitForResponse(
      (r) => r.url().includes('/signature/revoke') && r.request().method() === 'POST' && r.ok(),
    )
    await sigRow.getByRole('button', { name: 'Revoke', exact: true }).click()
    await revoked
    await expect(page.locator('.badge').filter({ hasText: 'REVOKED' }).first()).toBeVisible()
  })
})
