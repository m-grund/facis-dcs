import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
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
  return JSON.parse(readFileSync(path.join(path.dirname(fileURLToPath(import.meta.url)), '.auth', 'fixtures.json'), 'utf-8'))
}

interface RoleSession {
  token: string
  cookies: { name: string; value: string; domain: string; path: string }[]
}

interface DcsFixtures {
  /** Injects the role's session cookies + token: the router guard re-mints
   *  its token via POST /auth/refresh, which authenticates by cookie. */
  loginAs: (role: DcsRole) => Promise<void>
}

export const test = base.extend<DcsFixtures>({
  loginAs: async ({ page, context, baseURL }, use) => {
    const sessions: Record<string, RoleSession> = JSON.parse(
      readFileSync(path.join(path.dirname(fileURLToPath(import.meta.url)), '.auth', 'tokens.json'), 'utf-8'),
    )
    await use(async (role: DcsRole) => {
      const session = sessions[role]
      if (!session) throw new Error(`No session minted for role "${role}" — check e2e/global-setup.ts`)
      await context.addCookies(
        session.cookies.map((cookie) => ({
          name: cookie.name,
          value: cookie.value,
          // Cookies are per-host, not per-port: the login ran against the
          // ingress origin, the app runs on the dev server — same host.
          url: new URL(baseURL ?? 'http://localhost:5199').origin + cookie.path,
        })),
      )
      await page.addInitScript(
        ([accessToken]) => {
          window.localStorage.setItem('token_type', 'Bearer')
          window.localStorage.setItem('access_token', accessToken)
        },
        [session.token],
      )
    })
  },
})

export const expect = test.expect
