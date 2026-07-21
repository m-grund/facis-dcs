import { execFileSync } from 'node:child_process'
import { homedir, tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import type { Browser, BrowserContext, Page } from '@playwright/test'
import {
  E2E_API_BASE,
  E2E_API_BASE_B,
  E2E_DSS_URL,
  E2E_FRONTEND_B_ORIGIN,
  E2E_STATUSLIST_URL,
} from '../playwright.config'
import { applySession, type DcsRole, expect, mintSession } from './dcs-test'

const here = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(here, '../../..')
const python = process.env.E2E_BDD_PYTHON || path.join(homedir(), '.dcs-bdd-venv', 'bin', 'python3')

/**
 * A single DCS instance the two-instance vertical drives from its own UI: its
 * browser context/page bound to that DCS's frontend origin, its API base, and a
 * per-navigation session minter. Hydra rotates refresh tokens single-use, so
 * each top-level navigation re-mints a fresh role session for that instance.
 */
export interface Instance {
  readonly page: Page
  readonly context: BrowserContext
  readonly origin: string
  readonly apiBase: string
  gotoAs(role: DcsRole, url: string): Promise<void>
}

function makeInstance(page: Page, context: BrowserContext, origin: string, apiBase: string): Instance {
  return {
    page,
    context,
    origin,
    apiBase,
    async gotoAs(role, url) {
      await applySession(context, page, origin, mintSession(role, apiBase))
      await page.goto(url)
    },
  }
}

/** Wraps the test's own fixture page/context as instance A (the originator). */
export function instanceA(page: Page, context: BrowserContext, origin: string): Instance {
  return makeInstance(page, context, origin, E2E_API_BASE)
}

/** Opens a second browser context/page for instance B (the counterparty), on
 *  B's own frontend origin and API base — the DCS-to-DCS peer. */
export async function openInstanceB(browser: Browser): Promise<Instance> {
  const context = await browser.newContext({ baseURL: E2E_FRONTEND_B_ORIGIN })
  const page = await context.newPage()
  return makeInstance(page, context, E2E_FRONTEND_B_ORIGIN, E2E_API_BASE_B)
}

/**
 * Signs an APPROVED contract on a given instance through that instance's Secure
 * Contract Viewer, exactly as a real signer would (ADR-12): open from the
 * signing list, verify, run the wallet PID+PoA ceremony (the wallet leg arrives
 * over the wallet's own webhook channel against this instance's API base),
 * download the to-be-signed PDF, sign it externally with the test wallet's key
 * via the DSS SCA, upload it, and confirm SIGNED. The signature field is the
 * signing party's own DCS DID slot; the wallet discovers it from the PDF.
 */
export async function signOnInstance(inst: Instance, contractDid: string, signatory: string): Promise<void> {
  await inst.gotoAs('Contract Signer', '/ui/signing')
  const row = inst.page.getByRole('row').filter({ hasText: contractDid })
  await expect(row).toBeVisible()
  await row.getByRole('link', { name: /Open/ }).click()
  await expect(inst.page).toHaveURL(/\/signing\/.+/)

  await inst.page.getByRole('button', { name: 'Verify', exact: true }).click()
  await expect(inst.page.getByText('Verified', { exact: true })).toBeVisible()

  const ceremonyStarted = inst.page.waitForResponse(
    (r) => r.url().includes('/signature/request') && r.request().method() === 'POST' && r.ok(),
  )
  const preparedDownload = inst.page.waitForEvent('download')
  await inst.page.getByRole('button', { name: /download document to sign/ }).click()
  const ceremony = (await (await ceremonyStarted).json()) as { ceremony_id: string }
  expect(ceremony.ceremony_id).toBeTruthy()

  execFileSync(python, [path.join(here, 'complete_signing_webhook.py'), ceremony.ceremony_id], {
    cwd: repoRoot,
    env: { ...process.env, STATUSLIST_SERVICE_URL: E2E_STATUSLIST_URL, BDD_DCS_BASE_URL: inst.apiBase },
    stdio: 'pipe',
  })

  const preparedPath = (await (await preparedDownload).path())!
  const signedPath = path.join(tmpdir(), `signed-${ceremony.ceremony_id}.pdf`)
  execFileSync(python, [path.join(here, 'sign_prepared_pdf.py'), preparedPath, signedPath], {
    cwd: repoRoot,
    env: { ...process.env, DSS_URL: E2E_DSS_URL, E2E_SIGNATORY: signatory },
    stdio: 'pipe',
  })

  await inst.page.locator('input[type="file"]').setInputFiles(signedPath)
  await expect(inst.page.getByText('SIGNED', { exact: true })).toBeVisible({ timeout: 120_000 })
}
