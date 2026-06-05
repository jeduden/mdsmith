#!/usr/bin/env bash
# serve.sh — build the Hugo site content tree and serve it on a fixed port.
#
# Used by:
#   - Playwright's webServer config (website/e2e/playwright.config.ts)
#   - The CI e2e job (.github/workflows/e2e.yml)
#   - The site-e2e agent skill (editors/claude-code-site/)
#
# The Hugo version comes from the HUGO_VERSION env var. Its default
# below must be kept in sync with the pin in .github/workflows/pages.yml
# so all three callers render byte-identical output.
#
# Usage:
#   PORT=3001 ./website/e2e/scripts/serve.sh
#
# Env vars:
#   PORT          Port to listen on (default: 3001)
#   HUGO_VERSION  Hugo version to use (default: 0.161.1 — must match pages.yml)
#
# The script runs from the repository root.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
PORT="${PORT:-3001}"
HUGO_VERSION="${HUGO_VERSION:-0.161.1}"

echo "serve.sh: repo root = $REPO_ROOT"
echo "serve.sh: Hugo version = $HUGO_VERSION"
echo "serve.sh: port = $PORT"

cd "$REPO_ROOT"

# Build the content tree from docs/ -> website/content/docs/
# --no-fix: docs/ is already lint-clean; CI must not mutate the tree.
echo "serve.sh: building content tree..."
go run ./cmd/mdsmith-release build-website --no-fix ./docs ./website/content/docs

# Render the site into website/public/.
# Use `go run github.com/gohugoio/hugo` so the pinned version is
# resolved through the module proxy with sumdb checksum verification —
# the same mechanism that pages.yml's `go install` step uses — without
# requiring the binary to be on PATH ahead of time.
echo "serve.sh: rendering site..."
(cd "$REPO_ROOT/website" && go run "github.com/gohugoio/hugo@v${HUGO_VERSION}" --minify --baseURL "/")

# Serve the rendered output with the version-pinned `serve` from
# website/e2e/node_modules (a package.json devDependency), so CI and
# locked-down environments need no runtime network fetch.
# Do NOT use -s (single-SPA mode) — the Hugo site is multi-page.
echo "serve.sh: serving website/public/ on port $PORT"
SERVE_BIN="$REPO_ROOT/website/e2e/node_modules/.bin/serve"
if [ ! -x "$SERVE_BIN" ]; then
  echo "serve.sh: $SERVE_BIN missing — run 'npm ci' in website/e2e first" >&2
  exit 1
fi
exec "$SERVE_BIN" -p "$PORT" "$REPO_ROOT/website/public"
