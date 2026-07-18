import { execFileSync } from 'node:child_process'
import { homedir, tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { E2E_API_BASE, E2E_DSS_URL, E2E_STATUSLIST_URL } from '../playwright.config'
import { expect, test } from './dcs-test'
import { buildApprovedContract, gotoAs } from './lifecycle-helpers'

/**
 * Secure Contract Viewer (SRS DCS-IR-SM-01..04): an approved contract is
 * driven through the per-contract split-view signing wizard against the real
 * backend — Retrieve → Verify (integrity + envelope) → Apply Signature
 * (wallet-driven prepare/sign/submit, ADR-12) → Submit → Validate → executed.
 * The wallet leg (PID ceremony + external AES) is played by the same test
 * helpers the full-vertical signing stage uses.
 */

const here = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(here, '../../..')
const python = process.env.E2E_BDD_PYTHON || path.join(homedir(), '.dcs-bdd-venv', 'bin', 'python3')

test('secure contract viewer drives the guided signing wizard', async ({ page, loginAs }) => {
  test.setTimeout(600_000)
  page.setDefaultTimeout(15_000)

  const contractDid = await buildApprovedContract(page, loginAs)

  await test.step('open the approved contract in the Secure Contract Viewer', async () => {
    await gotoAs(page, loginAs, 'Contract Signer', '/ui/signing')
    const row = page.getByRole('row').filter({ hasText: contractDid })
    await expect(row).toBeVisible()
    await row.getByRole('link', { name: /Open/ }).click()

    await expect(page).toHaveURL(/\/signing\/.+/)
    // Split-view: contract content on the left, the wizard on the right.
    await expect(page.getByRole('heading', { name: 'Contract document' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Verify', exact: true })).toBeVisible()
  })

  await test.step('verify integrity and signature envelope', async () => {
    await page.getByRole('button', { name: 'Verify', exact: true }).click()
    // The "Verified" badge only renders once verify + PDF integrity resolved,
    // which is also the gate that enables Apply Signature.
    await expect(page.getByText('Verified', { exact: true })).toBeVisible()
  })

  await test.step('apply signature and submit the externally-signed PDF', async () => {
    const ceremonyStarted = page.waitForResponse(
      (r) => r.url().includes('/signature/request') && r.request().method() === 'POST' && r.ok(),
    )
    // Apply Signature opens the ceremony dialog; once the wallet presents its
    // PID over the webhook, the viewer fetches the to-be-signed PDF
    // (/signature/prepare) and downloads it (ADR-12: the DCS holds no key).
    const preparedDownload = page.waitForEvent('download')
    await page.getByRole('button', { name: 'Apply Signature', exact: true }).click()
    const ceremony = (await (await ceremonyStarted).json()) as { ceremony_id: string }
    expect(ceremony.ceremony_id).toBeTruthy()

    execFileSync(python, [path.join(here, 'complete_signing_webhook.py'), ceremony.ceremony_id], {
      cwd: repoRoot,
      env: { ...process.env, STATUSLIST_SERVICE_URL: E2E_STATUSLIST_URL, BDD_DCS_BASE_URL: E2E_API_BASE },
      stdio: 'pipe',
    })

    // The signatory signs the prepared PDF externally (test wallet drives the
    // DSS SCA with its own key) and uploads it; the DCS validates and records it.
    const preparedPath = (await (await preparedDownload).path())!
    const signedPath = path.join(tmpdir(), `scv-signed-${ceremony.ceremony_id}.pdf`)
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
    await page.locator('input[type="file"]').setInputFiles(signedPath)

    await expect(page.getByText('Submitted', { exact: true })).toBeVisible({ timeout: 120_000 })
  })

  await test.step('validate the applied signature and confirm execution', async () => {
    await page.getByRole('button', { name: 'Validate', exact: true }).click()
    await expect(page.getByText('Validated', { exact: true })).toBeVisible()
    await expect(page.getByText('Executed contract submitted.')).toBeVisible()
  })
})
