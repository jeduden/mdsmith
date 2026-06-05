import { defineConfig, devices } from "@playwright/test";

/**
 * Playwright configuration for mdsmith.dev end-to-end tests.
 *
 * The webServer block builds and serves the site via the shared
 * serve.sh script so Playwright, CI, and the site-e2e agent skill
 * all render byte-identical Hugo output.
 *
 * Chromium only to start — cross-browser and visual-regression
 * snapshots are out of scope for this first suite.
 */
export default defineConfig({
  testDir: "./tests",
  /* Run tests in files in parallel */
  fullyParallel: true,
  /* Fail the build on CI if you accidentally left test.only in the source */
  forbidOnly: !!process.env.CI,
  /* Retry on first failure in CI */
  retries: process.env.CI ? 1 : 0,
  /* Reporter: list for console + HTML for CI artifact */
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    /* Base URL for all page.goto("/") calls */
    baseURL: `http://localhost:${process.env.PORT ?? 3001}`,
    /* Collect traces on first retry only — keeps artifacts small */
    trace: "on-first-retry",
    /* Screenshot on failure */
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  /* Start the site server before running tests.
   * Playwright starts this once for the entire run and stops it after.
   * The PORT env var is forwarded so serve.sh and baseURL agree. */
  webServer: {
    command: `PORT=${process.env.PORT ?? 3001} bash scripts/serve.sh`,
    url: `http://localhost:${process.env.PORT ?? 3001}`,
    reuseExistingServer: !process.env.CI,
    /* Allow up to 3 minutes for the content build + Hugo render */
    timeout: 180_000,
    stdout: "pipe",
    stderr: "pipe",
  },
});
