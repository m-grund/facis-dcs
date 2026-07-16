import { readFileSync } from 'node:fs'
import path from 'node:path'
import { test as base } from '@playwright/test'

type DcsRole =
  | 'Template Creator'
  | 'Template Manager'
  | 'Contract Creator'
  | 'Contract Manager'
  | 'Contract Signer'
  | 'Auditor'

export interface SeededFixtures {
  templateDid: string
  contractDid: string
  contractName: string
}

export function seededFixtures(): SeededFixtures {
  return JSON.parse(readFileSync(path.join(__dirname, '.auth', 'fixtures.json'), 'utf-8'))
}

interface DcsFixtures {
  /** Navigates with the given role's token injected into localStorage. */
  loginAs: (role: DcsRole) => Promise<void>
}

export const test = base.extend<DcsFixtures>({
  loginAs: async ({ page }, use) => {
    const tokens: Record<string, string> = JSON.parse(
      readFileSync(path.join(__dirname, '.auth', 'tokens.json'), 'utf-8'),
    )
    await use(async (role: DcsRole) => {
      const token = tokens[role]
      if (!token) throw new Error(`No token minted for role "${role}" — check e2e/global-setup.ts`)
      await page.addInitScript(
        ([accessToken]) => {
          window.localStorage.setItem('token_type', 'Bearer')
          window.localStorage.setItem('access_token', accessToken)
        },
        [token],
      )
    })
  },
})

export const expect = test.expect
