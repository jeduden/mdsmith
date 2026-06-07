---
title: website end-to-end tests
summary: >-
  Playwright e2e suite for the mdsmith.dev website. Covers
  the interactive JavaScript the Go render probes cannot
  reach: install-picker chip filtering, the Windows command
  swap, copy-to-clipboard feedback, and the no-JS noscript
  fallback. One shared `serve.sh` entrypoint is used by
  Playwright's webServer, the CI job, and the `site-e2e`
  agent skill.
---
# Website end-to-end tests

A Playwright suite for the mdsmith.dev site. It covers
the interactive JavaScript that the Go render probes in
`internal/release/` cannot reach.

## What is tested

| Spec                     | Coverage                                                                              |
| ------------------------ | ------------------------------------------------------------------------------------- |
| `install-picker.spec.ts` | Chip filtering, Windows command swap, copy-to-clipboard, no-JS noscript fallback      |
| `chrome.spec.ts`         | Scroll-triggered `is-scrolled` on `.topnav`, docs sidebar toggle                      |
| `search.spec.ts`         | ⌘K search: JSON index fetch, dialog lifecycle, querying, keyboard nav, no-JS fallback |

## Run locally

Requires Node 22+ and Go 1.24+.

```bash
# Install Playwright and its chromium browser (once per clone).
cd website/e2e
npm ci
npx playwright install chromium

# Run the suite from the repo root.
cd website/e2e && npx playwright test
```

Playwright's `webServer` invokes `scripts/serve.sh`. That
script builds the docs tree, renders with the pinned Hugo,
and serves `website/public/` on port 3001. The CI job and
the `site-e2e` skill use the same script, so all three
callers render byte-identical output.

## Shared entrypoint

`scripts/serve.sh` is the single source of truth for
building and serving the site:

```bash
PORT=3001 bash website/e2e/scripts/serve.sh
```

The Hugo version is pinned once in `.hugo-version` at the
repo root. That file is read by serve.sh, the e2e CI job,
and the pages deploy, so a bump there updates every
renderer.

## CI

`.github/workflows/e2e.yml` runs on every PR and push to
main that touches the website, docs, or the e2e suite
itself. It:

1. Installs Node 22 and `npm ci` in `website/e2e/`.
2. Caches Playwright browsers keyed on `package-lock.json`.
3. Pre-warms the Go build cache with the pinned Hugo from
   `.hugo-version`; serve.sh renders via `go run`.
4. Runs `npx playwright test` with `CI=true`.
5. Uploads `playwright-report/` and traces as a job
   artifact on every run (`if: always()`).

## Agent skill

The `mdsmith-site` Claude Code plugin (in
`editors/claude-code-site/`) ships the `site-e2e` skill
and a Playwright MCP server. See
[`editors/claude-code-site/README.md`](../../editors/claude-code-site/README.md).

## Reproducing a CI failure

```bash
cd website/e2e
CI=true npx playwright test
```

`CI=true` enables retries and writes traces on first retry,
matching the CI configuration.
