#!/usr/bin/env bash
# Sync ../docs/ into content/docs/ for a Hugo build.
#
# Thin convenience wrapper around `mdsmith-release sync-docs`, which
# is the canonical implementation (see
# docs/development/release-tooling.md — every workflow that needs
# runtime logic goes through that binary). The wrapper exists so a
# developer can run one short command from the website/ directory
# during local edits.
#
# Steps:
#   1. (optional, on by default) run `mdsmith fix` against the
#      source docs/ so every <?catalog?> and <?include?> body is
#      current. Skip with --no-fix when you want to preview without
#      mutating the source tree.
#   2. Invoke `mdsmith-release sync-docs` to snapshot docs/ into
#      content/docs/, drop proto.md schema templates, rename
#      index.md to _index.md, prune non-content files, and escape
#      literal Hugo shortcode patterns.
#
# Run from the website/ directory:
#   ./scripts/sync-docs.sh           # mdsmith fix + sync
#   ./scripts/sync-docs.sh --no-fix  # sync only

set -euo pipefail

run_fix=1
for arg in "$@"; do
  case "$arg" in
    --no-fix) run_fix=0 ;;
    -h|--help)
      # Reprint the top-of-file comment block as help text.
      # Keep the upper bound aligned with the comment block's
      # final line so the `--no-fix` usage example below the
      # step list stays visible.
      sed -n '2,23p' "$0" | sed 's/^# \?//'
      exit 0 ;;
    *) echo "unknown flag: $arg" >&2; exit 2 ;;
  esac
done

here="$(cd "$(dirname "$0")/.." && pwd)"
repo="$(cd "$here/.." && pwd)"
src="$repo/docs"
dst="$here/content/docs"

if [[ ! -d "$src" ]]; then
  echo "source not found: $src" >&2
  exit 1
fi

if (( run_fix )); then
  echo "==> go run ./cmd/mdsmith fix $src"
  (cd "$repo" && go run ./cmd/mdsmith fix "$src") || {
    echo "mdsmith fix failed" >&2
    exit 1
  }
fi

echo "==> go run ./cmd/mdsmith-release sync-docs $src -> $dst"
cd "$repo" && go run ./cmd/mdsmith-release sync-docs "$src" "$dst"

echo "==> done"
