import { defineConfig, devices } from '@playwright/test'

/**
 * E2E suite against a running DCS instance (default: the BDD kind cluster's
 * instance A). The dev server proxies /api to E2E_DCS_API_TARGET with the
 * instance's API base path, and global-setup mints role tokens through the
 * instance's real OID4VP headless login (reusing the BDD suite's
 * AuthService), so specs drive the UI exactly as an authenticated user.
 *
 * Requirements: the target DCS instance is up (make -C tests/bdd kind_up)
 * and the BDD venv exists (~/.dcs-bdd-venv, created by the BDD Makefile).
 */

const FRONTEND_PORT = Number(process.env.E2E_FRONTEND_PORT || 5199)

export const E2E_API_BASE =
  process.env.E2E_DCS_API_BASE || 'http://dcs-a.localhost:18080/digital-contracting-service/api'

const apiTarget = new URL(E2E_API_BASE)

export default defineConfig({
  testDir: './e2e',
  globalSetup: './e2e/global-setup',
  timeout: 60_000,
  expect: { timeout: 15_000 },
  retries: process.env.CI ? 1 : 0,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: `http://localhost:${FRONTEND_PORT}`,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: 'npx vite --port ' + FRONTEND_PORT,
    url: `http://localhost:${FRONTEND_PORT}`,
    reuseExistingServer: !process.env.CI,
    env: {
      DCS_FRONTEND_PORT: String(FRONTEND_PORT),
      DCS_API_TARGET: apiTarget.origin,
      DCS_API_TARGET_PATH: apiTarget.pathname,
    },
  },
})
