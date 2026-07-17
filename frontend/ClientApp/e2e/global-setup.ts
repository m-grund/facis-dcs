import { execFileSync } from 'node:child_process'
import { mkdirSync, writeFileSync } from 'node:fs'
import { homedir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { E2E_API_BASE, E2E_STATUSLIST_URL } from '../playwright.config'

/**
 * Seeds the shared E2E fixtures (an approved template and a draft contract
 * carrying an ODRL-constrained requirement field) through the instance's
 * public API. Role sessions are minted per test in e2e/dcs-test.ts —
 * Hydra's refresh tokens are single-use, so sessions cannot be shared.
 */

const here = path.dirname(fileURLToPath(import.meta.url))

export default function globalSetup(): void {
  const repoRoot = path.resolve(here, '..', '..', '..')
  const python = process.env.E2E_BDD_PYTHON || path.join(homedir(), '.dcs-bdd-venv', 'bin', 'python3')

  const seeded = execFileSync(python, [path.join(here, 'seed_fixtures.py'), E2E_API_BASE], {
    cwd: repoRoot,
    encoding: 'utf-8',
    timeout: 180_000,
    env: {
      ...process.env,
      STATUSLIST_SERVICE_URL: E2E_STATUSLIST_URL,
      BDD_DCS_BASE_URL: E2E_API_BASE,
    },
  })
  const authDir = path.join(here, '.auth')
  mkdirSync(authDir, { recursive: true })
  writeFileSync(path.join(authDir, 'fixtures.json'), seeded.trim().split('\n').pop() ?? '{}')
}
