import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { homedir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { test as base } from '@playwright/test'
import { E2E_API_BASE } from '../playwright.config'

type DcsRole =
  | 'Template Creator'
  | 'Template Manager'
  | 'Contract Creator'
  | 'Contract Manager'
  | 'Contract Signer'
  | 'Auditor'

const here = path.dirname(fileURLToPath(import.meta.url))

export interface SeededFixtures {
  templateDid: string
  contractDid: string
  contractName: string
}

export function seededFixtures(): SeededFixtures {
  return JSON.parse(readFileSync(path.join(here, '.auth', 'fixtures.json'), 'utf-8'))
}

interface RoleSession {
  token: string
  cookies: { name: string; value: string; domain: string; path: string }[]
}

/** Mints a FRESH OID4VP session for the role — Hydra rotates refresh tokens
 *  (single use), so sessions cannot be shared across tests: the app's router
 *  guard consumes one refresh per navigation chain. */
function mintSession(role: DcsRole): RoleSession {
  const repoRoot = path.resolve(here, '..', '..', '..')
  const python = process.env.E2E_BDD_PYTHON || path.join(homedir(), '.dcs-bdd-venv', 'bin', 'python3')
  const script = `
import json, sys
import requests
sys.path.insert(0, ${JSON.stringify(path.resolve(here, '..', '..', '..'))})
from steps.support import localhost_resolver
localhost_resolver.install()
from steps.support.services.auth_service import AuthService
role, api_base = sys.argv[1], sys.argv[2]
credentials = AuthService.parse_auth_credentials([role], None)
session = requests.Session()
session.headers.update({"User-Agent": "dcs-e2e-auth", "Accept": "application/json"})
initiation = AuthService.initiate_login(session, api_base, timeout=60)
AuthService.bind_hydra_login_challenge(session, api_base, state=initiation.state, authorize_url=initiation.authorize_url, timeout=60)
auth_request = AuthService.fetch_authorization_request(session, initiation.request_uri, timeout=60)
vp_token = AuthService.build_vp_token(credentials, nonce=auth_request.nonce, client_id=auth_request.client_id)
redirect_uri = AuthService.submit_presentation(session, api_base=api_base, response_uri=auth_request.response_uri, state=auth_request.state, query_id=auth_request.query_id, vp_token=vp_token, timeout=60)
access_token, _ = AuthService.complete_session(session, api_base, redirect_uri, timeout=60)
print(json.dumps({"token": access_token, "cookies": [
    {"name": c.name, "value": c.value, "domain": c.domain, "path": c.path or "/"} for c in session.cookies
]}))
`
  const stdout = execFileSync(python, ['-c', script, role, E2E_API_BASE], {
    cwd: repoRoot,
    encoding: 'utf-8',
    timeout: 120_000,
    env: {
      ...process.env,
      STATUSLIST_SERVICE_URL: `${new URL(E2E_API_BASE).origin}/statuslist`,
      BDD_DCS_BASE_URL: E2E_API_BASE,
    },
  })
  return JSON.parse(stdout.trim().split('\n').pop() ?? '{}')
}

interface DcsFixtures {
  /** Injects a fresh role session: cookies (the router guard re-mints its
   *  token via POST /auth/refresh, which authenticates by cookie) plus the
   *  access token in localStorage. */
  loginAs: (role: DcsRole) => Promise<void>
}

export const test = base.extend<DcsFixtures>({
  loginAs: async ({ page, context, baseURL }, use) => {
    await use(async (role: DcsRole) => {
      const session = mintSession(role)
      await context.addCookies(
        session.cookies.map((cookie) => ({
          name: cookie.name,
          value: cookie.value,
          // Host-wide on the dev origin: cookies are per-host (not per-port),
          // but their ingress paths (/digital-contracting-service/…) would
          // never match the dev server's rewritten /api/* paths.
          url: new URL(baseURL ?? 'http://localhost:5199').origin + '/',
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
