import { execFileSync } from 'node:child_process'
import { mkdirSync, writeFileSync } from 'node:fs'
import { homedir } from 'node:os'
import path from 'node:path'
import { E2E_API_BASE } from '../playwright.config'

/**
 * Mints one access token per role through the instance's real OID4VP
 * headless login by reusing the BDD suite's AuthService (steps/support),
 * then stores them for the specs' localStorage injection (the app keeps
 * `token_type` + `access_token` in localStorage; see
 * src/stores/auth-token-store.ts).
 */

const ROLES = ['Template Creator', 'Template Manager', 'Contract Creator']

export default function globalSetup(): void {
  const repoRoot = path.resolve(__dirname, '..', '..', '..')
  const python = process.env.E2E_BDD_PYTHON || path.join(homedir(), '.dcs-bdd-venv', 'bin', 'python3')

  const script = `
import json, sys
sys.path.insert(0, ${JSON.stringify(repoRoot)})
from steps.support.services.auth_service import AuthService
roles = json.loads(sys.argv[1])
api_base = sys.argv[2]
tokens = {role: AuthService.exchange_roles_for_access_token(api_base, [role]) for role in roles}
print(json.dumps(tokens))
`
  const stdout = execFileSync(python, ['-c', script, JSON.stringify(ROLES), E2E_API_BASE], {
    cwd: repoRoot,
    encoding: 'utf-8',
    timeout: 180_000,
  })
  const tokens = JSON.parse(stdout.trim().split('\n').pop() ?? '{}')

  const authDir = path.join(__dirname, '.auth')
  mkdirSync(authDir, { recursive: true })
  writeFileSync(path.join(authDir, 'tokens.json'), JSON.stringify(tokens, null, 2))
}
