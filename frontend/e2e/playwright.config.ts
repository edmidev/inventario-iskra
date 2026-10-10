import { defineConfig, devices } from "@playwright/test";
import { API_URL, WEB_PORT, WEB_URL } from "./env";

// Real stack: throwaway Postgres (docker) + Go API on :18080 (global setup) + the production
// build served by `vite preview` with /api proxied to the API.
export default defineConfig({
  testDir: ".",
  testMatch: /.*\.spec\.ts/,
  outputDir: "./.tmp/results",
  globalSetup: "./global-setup.ts",
  // Specs share one seeded database and each owns its items, but run them one at a time anyway.
  workers: 1,
  fullyParallel: false,
  retries: 0,
  timeout: 45_000,
  expect: { timeout: 8_000 },
  reporter: [["list"]],
  use: {
    baseURL: WEB_URL,
    locale: "es-MX",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    ...devices["Pixel 5"],
    viewport: { width: 390, height: 844 },
    // Service workers stay out of the way of ordinary specs; pwa.spec.ts opts in explicitly.
    serviceWorkers: "block",
  },
  projects: [{ name: "mobile-chromium", use: { browserName: "chromium" } }],
  webServer: {
    command: `npm run build && npx vite preview --host 127.0.0.1 --port ${WEB_PORT} --strictPort`,
    cwd: "..",
    url: WEB_URL,
    reuseExistingServer: false,
    timeout: 180_000,
    env: { API_PROXY_TARGET: API_URL },
  },
});
