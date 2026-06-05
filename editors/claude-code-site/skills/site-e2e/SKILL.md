---
name: site-e2e
description: >-
  Build the mdsmith.dev Hugo site, serve it locally,
  and run the Playwright end-to-end suite or drive it
  interactively. The same `serve.sh` entrypoint is used
  by Playwright's webServer, the CI job, and this skill,
  so the site rendered here is byte-identical to what CI
  and production render. Trigger when the user asks to
  "test the site", "run playwright", "drive the install
  picker", "screenshot the homepage", "check the Windows
  command swap", or "run the e2e suite".
user-invocable: true
argument-hint: "[test | screenshot <url> | serve]"
allowed-tools: >-
  Bash(git rev-parse:*),
  Bash(cd website/e2e && npm ci),
  Bash(npx playwright install:*),
  Bash(cd website/e2e && npx playwright:*),
  Bash(bash website/e2e/scripts/serve.sh:*)
---
# site-e2e

Build the mdsmith.dev Hugo site and serve it locally.
Run the Playwright end-to-end suite. Or drive the live
site interactively using the Playwright MCP server
bundled with this plugin.

## Prerequisites

- Node 22+ with `npm` and `npx` on `$PATH`
- Go 1.24+ (the repo already requires this)
- The `@playwright/test` package installed:
  `cd website/e2e && npm ci`
- Chromium browser:
  `cd website/e2e && npx playwright install chromium`

## Steps

### Run the full suite

1. Locate the repository root:

   ```bash
   git rev-parse --show-toplevel
   ```

   Run every subsequent command from that path.

2. Install dependencies if needed:

   ```bash
   cd website/e2e && npm ci
   npx playwright install chromium
   ```

3. Run the Playwright suite:

   ```bash
   cd website/e2e && npx playwright test
   ```

   Playwright's `webServer` config invokes
   `website/e2e/scripts/serve.sh`, which:

  - Runs `go run ./cmd/mdsmith-release build-website`
     to sync `docs/` → `website/content/docs/`.
  - Renders the site with the pinned Hugo version.
  - Serves `website/public/` on port 3001 (or the
     `PORT` env var).

4. If tests fail, open the HTML report:

   ```bash
   cd website/e2e && npx playwright show-report
   ```

   Traces and screenshots are in `test-results/`.
   In CI the report is uploaded as a job artifact.

### Screenshot a page

Use the Playwright MCP server bundled with this plugin
to navigate and screenshot a page without writing a
test file.

1. Start the site server in one terminal:

   ```bash
   bash website/e2e/scripts/serve.sh
   ```

2. Ask the agent to screenshot a URL:
   "Screenshot <http://localhost:3001/>"

   The Playwright MCP server handles the browser
   session. It works headless with no display.

### Serve only

To start the site and leave it running (useful for
manual inspection or MCP interaction):

```bash
bash website/e2e/scripts/serve.sh
```

## Architecture note

All three entry points — `npx playwright test`,
the CI job (`.github/workflows/e2e.yml`), and this
skill — call the same `website/e2e/scripts/serve.sh`.
A change to the build or Hugo render path updates one
file and all three callers stay in sync.

## Reproducing a CI failure

The CI job sets `CI=true`, which enables retries and
forbids `test.only`. Reproduce locally with:

```bash
CI=true npx playwright test
```

Traces are written on the first retry so a CI failure
with a trace can be reproduced by adding `--trace on`
locally.
