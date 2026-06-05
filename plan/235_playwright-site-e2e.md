---
id: 235
title: Playwright end-to-end tests for the website, runnable by CI and agents
status: "🔲"
summary: >-
  Add a Playwright e2e suite for the Hugo site under
  `website/e2e/`, covering the interactive JavaScript the Go
  render probes cannot — install-picker chip filtering, the
  Windows command swap, copy-to-clipboard, and the no-JS
  fallback. One build-and-serve entrypoint is shared by
  Playwright's webServer, a new CI job, and a `site-e2e`
  agent skill. A new `mdsmith-site` plugin bundles that skill
  with a Playwright MCP server so agents can run the suite and
  drive the live site headless.
model: sonnet
depends-on: []
---
# Playwright end-to-end tests for the website

## Goal

Test the site's interactive JavaScript — the parts the Go
render probes cannot see. A Playwright suite covers it. CI
gates it on every PR. An agent can run and drive the same
suite through one shared entrypoint.

## Background

The site has client-side behavior with no automated
coverage. The install picker filters rows by platform chip,
swaps the GitHub row to a concrete `.exe` command under the
Windows chip, and copies the current command. The top nav
toggles a scrolled state; the docs sidebar toggles open.

The Go probes
([`verifypicker.go`](../internal/release/verifypicker.go),
[`verifylinks.go`](../internal/release/verifylinks.go)) read
the rendered HTML, so they assert the *markup* contract — the
`data-cmd-*` attributes and the `<noscript>` fallback exist.
They cannot run the JavaScript, so the swap, the copy, and
the chip filtering ship untested. This plan adds the
behavioral layer the [test pyramid](../docs/development/architecture/tests.md)
calls e2e, which the website currently has none of.

The agent angle matters too. Contributors and coding agents
should be able to drive the live site — click a chip, read
the swapped command, screenshot a page — not just run a
fixed suite. The repo already ships a Claude Code plugin
marketplace ([`marketplace.json`](../.claude-plugin/marketplace.json)),
so a site plugin is the natural home for an agent skill plus
a browser MCP server.

## Architecture: one shared entrypoint

CI, Playwright, and the agent skill must not each reinvent
how to build and serve the site. A single script owns it:

- `website/e2e/scripts/serve.sh` builds the docs content
  tree (`mdsmith-release build-website --no-fix`), renders
  with the pinned Hugo, and serves `website/public/` on a
  fixed port. The Hugo version is read from the same source
  [`pages.yml`](../.github/workflows/pages.yml) uses, so the
  three callers render byte-identical output.
- Playwright's `webServer` config invokes the script, so
  `npx playwright test` starts and stops the server itself.
- The CI job and the agent skill both call `npx playwright
  test`, inheriting the same server.

This keeps the render path DRY: a Hugo or build-website
change updates one script, not three callers.

## Test layout

- `website/e2e/package.json` — pins `@playwright/test` and
  the `site-e2e` scripts; `node_modules` stays gitignored.
- `website/e2e/playwright.config.ts` — `baseURL` to the
  local server, a single `chromium` project to start, trace
  on first retry, screenshot on failure, and both `list` and
  `html` reporters.
- `website/e2e/tests/*.spec.ts` — one spec per surface.

## Test scope (first specs)

**Install picker** (`install-picker.spec.ts`) — the JS the
Go probe cannot reach:

- A platform chip hides rows whose `data-platforms` lacks
  the tag and shows the rest.
- The **Windows** chip swaps the GitHub row's visible
  command to the `Invoke-WebRequest …​.exe` line; **All**
  restores the `curl` default.
- **Copy** writes the *currently shown* command. Grant
  `clipboard-read`/`clipboard-write` to read it back, and
  assert the button flips to `copied` then restores.
- With `javaScriptEnabled: false`, the `<noscript>` Windows
  line is visible — the no-JS guarantee, paired with the
  static check in `verifypicker.go`.

**Chrome** (`chrome.spec.ts`) — scrolling adds `is-scrolled`
to `.topnav`; the docs sidebar toggle flips `is-open` and
`aria-expanded`.

## Agent skill and browser MCP

A new plugin `editors/claude-code-site/`, registered in
[`marketplace.json`](../.claude-plugin/marketplace.json) as
`mdsmith-site`, carries both halves of "agents can run and
interact":

- `skills/site-e2e/SKILL.md` — a `user-invocable` skill in
  the same frontmatter shape as an existing
  [skill](../editors/claude-code-skills/skills/) that
  documents the headless workflow: build and serve via
  the shared script, run the suite, screenshot a page, and
  open the trace or HTML report. Trigger phrases: "test the
  site", "run playwright", "drive the install picker",
  "screenshot the homepage".
- A `playwright` MCP server (`@playwright/mcp`) declared in
  the plugin manifest — like the `lspServers` block in
  [`claude-code-dev`](../editors/claude-code-dev/.claude-plugin/plugin.json),
  but `mcpServers`. It gives an agent accessibility-tree
  snapshots and click/type against the running site with no
  display, which suits this headless environment where
  `codegen` and headed mode do not run.

The skill and CI invoke the identical `npx playwright test`,
so an agent reproduces a CI failure with one command.

## CI

A new job (in [`pages.yml`](../.github/workflows/pages.yml)
or a sibling `e2e.yml`):

- `actions/setup-node` (Node 22, matching the existing wasm
  job), `npm ci` in `website/e2e/`.
- Cache the Playwright browsers; `npx playwright install
  --with-deps chromium` only, to bound download time.
- Install the pinned Hugo (`go install …@v<HUGO_VERSION>`),
  then `npx playwright test`.
- On failure, upload `playwright-report/` and traces as a
  job artifact.

Pin `@playwright/test`, the browser, and Hugo so a render or
browser bump is a deliberate, reviewed change.

## Out of scope

- Headed/`codegen` flows — the container has no display;
  authoring uses local dev or the MCP snapshots.
- Cross-browser (firefox/webkit) and visual-regression
  snapshots — chromium-only first; widen once the suite is
  stable.
- Replacing the Go probes. They stay as the fast static
  markup contract; Playwright adds behavior on top.

## Tasks

1. Add `website/e2e/scripts/serve.sh` (build-website +
   pinned Hugo render + static serve on a fixed port); read
   the Hugo version from the same source as `pages.yml`.
2. Scaffold `website/e2e/` — `package.json` (pinned
   `@playwright/test`), `playwright.config.ts` with a
   `webServer` that runs the script, and gitignore entries.
3. Write `install-picker.spec.ts`: chip filtering, the
   Windows swap and **All** reset, copy of the shown command
   with the `copied` feedback, and the `javaScriptEnabled:
   false` noscript assertion.
4. Write `chrome.spec.ts`: `is-scrolled` on scroll and the
   docs-sidebar toggle.
5. Prove the suite guards behavior: temporarily break the
   swap JS in
   [`baseof.html`](../website/layouts/_default/baseof.html)
   and confirm the spec goes red, then revert.
6. Add the CI job: node + `npm ci`, cached
   `playwright install --with-deps chromium`, pinned Hugo,
   `npx playwright test`, report/trace artifact on failure.
7. Create the `editors/claude-code-site/` plugin:
   `plugin.json` (the `site-e2e` skill + the `playwright`
   `mcpServers` entry), `skills/site-e2e/SKILL.md`, and a
   `README.md`; register it in `marketplace.json`.
8. Document the workflow: a `website/e2e/README.md`, a note
   in [`website/README.md`](../website/README.md), and an
   e2e entry in the
   [test pyramid](../docs/development/architecture/tests.md).

## Acceptance Criteria

- [ ] `npx playwright test` passes locally and in CI against
      the shared-script server.
- [ ] Breaking the install-picker swap JS turns the suite
      red (it guards the behavior, not just the markup).
- [ ] The `javaScriptEnabled: false` spec confirms the
      `<noscript>` Windows fallback is visible.
- [ ] CI runs chromium-only with cached browsers and uploads
      a report/trace artifact on failure.
- [ ] `mdsmith-site` is in `marketplace.json`; the
      `site-e2e` skill builds, serves, runs the suite, and
      screenshots a page; the Playwright MCP can click the
      Windows chip and read the swapped command.
- [ ] `@playwright/test`, the browser, and Hugo are version-
      pinned.
- [ ] `mdsmith check .` passes (new Markdown lints clean).
- [ ] `go test ./...` and `go tool golangci-lint run` stay
      green.
