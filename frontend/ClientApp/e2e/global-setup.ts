import { execFileSync } from 'node:child_process'
import { mkdirSync, writeFileSync } from 'node:fs'
import { homedir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { E2E_API_BASE } from '../playwright.config'

/**
 * Mints one access token per role through the instance's real OID4VP
 * headless login by reusing the BDD suite's AuthService (steps/support),
 * then stores them for the specs' localStorage injection (the app keeps
 * `token_type` + `access_token` in localStorage; see
 * src/stores/auth-token-store.ts).
 */

const ROLES = [
  'Template Creator',
  'Template Manager',
  'Contract Creator',
  'Contract Manager',
  'Contract Signer',
  'Auditor',
]

const here = path.dirname(fileURLToPath(import.meta.url))

function apiOrigin(): string {
  return new URL(E2E_API_BASE).origin
}

export default function globalSetup(): void {
  const repoRoot = path.resolve(here, '..', '..', '..')
  const python = process.env.E2E_BDD_PYTHON || path.join(homedir(), '.dcs-bdd-venv', 'bin', 'python3')

  // Replicates AuthService.exchange_roles_for_access_token leg by leg with
  // an owned requests.Session, capturing BOTH the access token and the
  // session cookies: the app's router guard re-mints its token through
  // POST /auth/refresh, which authenticates by session cookie — a bare
  // localStorage token never survives the first guarded navigation.
  const script = `
import json, sys
import requests
sys.path.insert(0, ${JSON.stringify(repoRoot)})
from steps.support.services.auth_service import AuthService

roles = json.loads(sys.argv[1])
api_base = sys.argv[2]
result = {}
for role in roles:
    credentials = AuthService.parse_auth_credentials([role], None)
    session = requests.Session()
    session.headers.update({"User-Agent": "dcs-e2e-auth", "Accept": "application/json"})
    initiation = AuthService.initiate_login(session, api_base, timeout=60)
    AuthService.bind_hydra_login_challenge(session, api_base, state=initiation.state, authorize_url=initiation.authorize_url, timeout=60)
    auth_request = AuthService.fetch_authorization_request(session, initiation.request_uri, timeout=60)
    vp_token = AuthService.build_vp_token(credentials, nonce=auth_request.nonce, client_id=auth_request.client_id)
    redirect_uri = AuthService.submit_presentation(session, api_base=api_base, response_uri=auth_request.response_uri, state=auth_request.state, query_id=auth_request.query_id, vp_token=vp_token, timeout=60)
    access_token, _ = AuthService.complete_session(session, api_base, redirect_uri, timeout=60)
    cookies = [
        {"name": c.name, "value": c.value, "domain": c.domain, "path": c.path or "/"}
        for c in session.cookies
    ]
    result[role] = {"token": access_token, "cookies": cookies}
print(json.dumps(result))
`
  // Credentials issued by the headless login embed the status-list URL the
  // BACKEND must be able to dereference — the ingress-served one, exactly as
  // the BDD harness exports it (run_bdd_helm.sh), never the dev NodePort.
  const env = {
    ...process.env,
    STATUSLIST_SERVICE_URL: `${apiOrigin()}/statuslist`,
    BDD_DCS_BASE_URL: E2E_API_BASE,
  }
  const stdout = execFileSync(python, ['-c', script, JSON.stringify(ROLES), E2E_API_BASE], {
    cwd: repoRoot,
    encoding: 'utf-8',
    timeout: 180_000,
    env,
  })
  const tokens = JSON.parse(stdout.trim().split('\n').pop() ?? '{}')

  const authDir = path.join(here, '.auth')
  mkdirSync(authDir, { recursive: true })
  writeFileSync(path.join(authDir, 'tokens.json'), JSON.stringify(tokens, null, 2))

  const seeded = execFileSync(python, [path.join(here, 'seed_fixtures.py'), E2E_API_BASE], {
    cwd: repoRoot,
    encoding: 'utf-8',
    timeout: 180_000,
    env,
  })
  writeFileSync(path.join(authDir, 'fixtures.json'), seeded.trim().split('\n').pop() ?? '{}')
}
