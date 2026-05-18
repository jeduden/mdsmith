#!/usr/bin/env bash
# Reproducible cross-tool Markdown-linter benchmark.
#
# Thin wrapper over `mdsmith-release bench`. The harness logic
# (build mdsmith, fetch + SHA-256-verify the pinned comparison
# binaries, `npm ci` markdownlint-cli2 from the committed
# lockfile, build the corpora, drive hyperfine, promote the JSON
# into data/, regenerate the fragments via gen_fragments.py) now
# lives in Go so the local hand-refresh and the post-merge
# benchmark.yml workflow run byte-identical logic — see
# docs/development/release-tooling.md and
# internal/release/bench.go.
#
# Usage:  docs/research/benchmarks/run.sh [workdir]
#
# Requires network on first run (downloads the pinned tool
# tarballs and the npm tree) plus a Go toolchain, python3, npm,
# and git on PATH. workdir caches built/fetched binaries and the
# corpora (default /tmp/mdsmith-bench) so a rerun is cheap.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WORK="${1:-/tmp/mdsmith-bench}"

cd "$REPO_ROOT"
go run ./cmd/mdsmith-release bench "$WORK"

echo "Now run: mdsmith fix . (refresh <?include?> bodies)"
