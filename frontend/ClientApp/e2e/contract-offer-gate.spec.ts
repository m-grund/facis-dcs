import { expect, test } from './dcs-test'
import { buildDraftContract, gotoAs } from './lifecycle-helpers'

/**
 * The offer gate on the contract view: "Offer to counterparty" is the first
 * transmission of a draft to its counterparty (DRAFT -> OFFERED), and SRS §1.2
 * defines an offer as a clear and DEFINITE proposal — §2.2.2 requires Contract
 * Generation to end with a filled-out contract "ready to be sent to the
 * Responder". command/offer.go's validateOfferReady therefore enforces contract
 * closedness: a draft still carrying an unfilled required placeholder is
 * rejected (HTTP 400, surfaced via the global error toast) and stays DRAFT;
 * once the Contract Creator fills the placeholder through the real edit UI, the
 * same button performs DRAFT -> OFFERED.
 *
 * The fixture walks the same path a user does — author a Payment component with
 * an inline required placeholder, drive it through its template lifecycle,
 * compose + approve + register a contract template from it, derive the contract
 * — all through real UI buttons (lifecycle-helpers).
 */

test('offer to counterparty is blocked until the draft is filled out', async ({ page, loginAs }) => {
  test.setTimeout(600_000)

  const { contractDid } = await buildDraftContract(page, loginAs)

  await test.step('offering the unfilled draft is rejected and the contract stays DRAFT', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/view/${contractDid}`)
    const rejected = page.waitForResponse((r) => r.url().includes('/contract/offer') && r.request().method() === 'POST')
    await page.getByRole('button', { name: 'Offer to counterparty' }).click()
    const resp = await rejected
    expect(resp.status(), `offer of an unfilled draft must be a client error: ${await resp.text()}`).toBe(400)
    // The backend's closedness message reaches the user via the global toast.
    await expect(page.getByRole('alert').filter({ hasText: 'contract is not closed' })).toBeVisible()
    // Still DRAFT: reloading the view still shows the state and the offer action.
    await page.reload()
    await expect(page.getByText('DRAFT', { exact: true }).first()).toBeVisible({ timeout: 15_000 })
    await expect(page.getByRole('button', { name: 'Offer to counterparty' })).toBeVisible()
  })

  await test.step('fill the required Payment Amount through the edit UI', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/edit/${contractDid}`)
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
    const updated = page.waitForResponse((r) => r.url().includes('/contract/update') && r.request().method() === 'PUT')
    await page.getByRole('button', { name: 'Update', exact: true }).click()
    const resp = await updated
    expect(resp.ok(), `contract update ${resp.status()}: ${await resp.text()}`).toBeTruthy()
  })

  await test.step('offering the filled draft succeeds: DRAFT -> OFFERED', async () => {
    await gotoAs(page, loginAs, 'Contract Creator', `/ui/contracts/view/${contractDid}`)
    const offered = page.waitForResponse(
      (r) => r.url().includes('/contract/offer') && r.request().method() === 'POST' && r.ok(),
      { timeout: 30_000 },
    )
    await page.getByRole('button', { name: 'Offer to counterparty' }).click()
    await offered
    // The view reloads on success; EventOffer is DRAFT-only, so the action is gone.
    await expect(page.getByText('OFFERED', { exact: true }).first()).toBeVisible({ timeout: 15_000 })
    await expect(page.getByRole('button', { name: 'Offer to counterparty' })).toBeHidden()
  })
})
