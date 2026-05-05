#!/usr/bin/env bash
# set-version.sh — rewrite the version field in every tracked
# manifest to the supplied semver. The release workflow runs this
# before each `package` or `publish` step. The script is idempotent:
# running it twice with the same version produces no further change.
#
# Usage:
#   scripts/set-version.sh <version> [--root <path>]
#
# <version> is the cleaned tag (no leading "v"), e.g. "1.2.3". The
# optional --root flag overrides the repository root for tests; by
# default the script resolves the repo via its own location.

set -euo pipefail

usage() {
  echo "usage: $0 <version> [--root <path>]" >&2
  exit 2
}

if [ "$#" -lt 1 ]; then
  usage
fi

ver="$1"
shift

repo_root=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --root)
      [ "$#" -ge 2 ] || usage
      repo_root="$2"
      shift 2
      ;;
    *)
      usage
      ;;
  esac
done

if [ -z "$repo_root" ]; then
  repo_root="$(cd "$(dirname "$0")/.." && pwd)"
fi

if [ -z "$ver" ]; then
  echo "version must be non-empty" >&2
  exit 2
fi

case "$ver" in
  v*)
    echo "version must not start with 'v' (got '$ver')" >&2
    exit 2
    ;;
esac

# MAJOR.MINOR.PATCH plus optional pre-release / build metadata.
# Both npm and PyPI reject anything weaker, so refuse early.
if ! [[ "$ver" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.+-]+)?$ ]]; then
  echo "version '$ver' is not valid semver" >&2
  exit 2
fi

export V="$ver"

# Rewrite the first top-level "version": "..." entry in a JSON
# manifest. Avoids jq so the script stays dependency-free; perl
# in-place edits are portable across BSD and GNU userlands.
rewrite_json_version() {
  local file="$1"
  [ -f "$file" ] || return 0
  perl -i -pe '
    BEGIN { $done = 0 }
    if (!$done && s/^(\s*"version"\s*:\s*")[^"]+(")/$1.$ENV{V}.$2/e) {
      $done = 1;
    }
  ' "$file"
}

# Rewrite every "@mdsmith/<platform>": "..." pin so the npm root
# advertises matching platform-package versions. The pin is always
# equal to the root version, so a single regex covers every line.
rewrite_json_optional_deps() {
  local file="$1"
  [ -f "$file" ] || return 0
  perl -i -pe '
    s/^(\s*"\@mdsmith\/[^"]+"\s*:\s*")[^"]+(")/$1.$ENV{V}.$2/e;
  ' "$file"
}

rewrite_pyproject_version() {
  local file="$1"
  [ -f "$file" ] || return 0
  perl -i -pe '
    BEGIN { $done = 0 }
    if (!$done && s/^(\s*version\s*=\s*")[^"]+(")/$1.$ENV{V}.$2/e) {
      $done = 1;
    }
  ' "$file"
}

rewrite_json_version "$repo_root/editors/vscode/package.json"

rewrite_json_version "$repo_root/npm/mdsmith/package.json"
rewrite_json_optional_deps "$repo_root/npm/mdsmith/package.json"

if [ -d "$repo_root/npm/platforms" ]; then
  for pkg in "$repo_root"/npm/platforms/*/package.json; do
    [ -f "$pkg" ] || continue
    rewrite_json_version "$pkg"
  done
fi

rewrite_pyproject_version "$repo_root/python/pyproject.toml"
