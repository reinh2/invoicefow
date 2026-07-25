import { defineConfig, devices } from '@playwright/test';

/* End-to-end tests drive a *running* InvoiceFlow demo through a real browser.
   They deliberately start nothing themselves: the Compose smoke already proves
   the API contract, and what these add is the one thing it cannot check — that
   the same flow is actually completable in the interface.

   Run against an isolated demo, never the default persistent one:

     COMPOSE_PROJECT_NAME=invoiceflow-e2e API_HOST_PORT=18082 \
       POSTGRES_HOST_PORT=15434 RECEIVER_HOST_PORT=18092 \
       docker compose up --build --wait
     E2E_BASE_URL=http://127.0.0.1:18082 npm --prefix web run test:e2e

   Requires the Chromium browser once:
     npm --prefix web exec -- playwright install chromium */

export default defineConfig({
  testDir: './e2e',
  // The flow waits on a real worker, so a generous per-test budget is expected.
  timeout: 120_000,
  expect: { timeout: 15_000 },
  // Uploads are deduplicated by SHA-256 server-side, so parallel workers racing
  // on the same fixture would collide by design rather than by accident.
  workers: 1,
  fullyParallel: false,
  retries: 0,
  reporter: [['list']],
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://127.0.0.1:8080',
    trace: 'retain-on-failure',
    acceptDownloads: true,
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
