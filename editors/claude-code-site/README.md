---
title: mdsmith site plugin
summary: >-
  Marketplace plugin that ships the `site-e2e` skill
  for running Playwright end-to-end tests against the
  mdsmith.dev website, plus a Playwright MCP server for
  interactive browser-driven exploration.
---
# mdsmith site plugin

A Claude Code plugin for testing and driving the
[mdsmith.dev](https://mdsmith.dev) website.

## Install

```text
/plugin marketplace add jeduden/mdsmith
/plugin install mdsmith-site@mdsmith
/reload-plugins
```

## What it includes

| Component        | What it does                                       |
| ---------------- | -------------------------------------------------- |
| `site-e2e` skill | Build, serve, and run the Playwright e2e suite.    |
| Playwright MCP   | Browser snapshots and click/type on the live site. |

## Run the e2e suite

```text
/site-e2e test
```

This builds the Hugo site via the shared
`website/e2e/scripts/serve.sh` entrypoint, runs
`npx playwright test`, and reports results inline.

## Screenshot a page

Start the server, then ask the agent to screenshot:

```text
/site-e2e screenshot http://localhost:3001/
```

The Playwright MCP server drives a headless Chromium
session. It returns an accessibility snapshot or
screenshot.

## Drive the install picker

```text
/site-e2e drive the Windows chip on the install picker
```

The agent clicks the Windows chip via the Playwright
MCP, reads the swapped command, and reports what the
picker shows.

## Prerequisites

- Node 22+ with `npm` and `npx`
- Go 1.25+

Run once per clone to install the browser:

```bash
cd website/e2e && npm ci
npx playwright install chromium
```

## Relationship to CI

The `site-e2e` skill and the
[CI e2e job](../../.github/workflows/e2e.yml) both
call `website/e2e/scripts/serve.sh`. Running the
skill locally reproduces any CI failure with one
command.
