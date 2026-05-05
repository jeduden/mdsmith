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

# Validate against the semver.org grammar: each of MAJOR/MINOR/PATCH
# must be either "0" or a digit string with no leading zero
# (`[1-9][0-9]*`), then an optional pre-release identifier (-FOO)
# and an optional build metadata identifier (+BAR), independently.
# npm and PyPI both reject leading-zero numeric identifiers, so
# refuse them here rather than only at publish time.
semver_num='(0|[1-9][0-9]*)'
if ! [[ "$ver" =~ ^${semver_num}\.${semver_num}\.${semver_num}(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]; then
  echo "version '$ver' is not valid semver" >&2
  exit 2
fi

export V="$ver"

# A required manifest going missing means the publish would ship
# stale data, so abort instead of silently skipping.
require_file() {
  local file="$1"
  if [ ! -f "$file" ]; then
    echo "set-version.sh: required manifest missing: $file" >&2
    exit 1
  fi
}

# Rewrite the first top-level "version": "..." entry in a JSON
# manifest. Avoids jq so the script stays dependency-free; perl
# in-place edits are portable across BSD and GNU userlands. A
# pre-flight grep ensures the regex actually has something to
# match — otherwise a malformed manifest (renamed key, JSON
# corruption) would no-op silently and we'd ship 0.0.0-dev.
rewrite_json_version() {
  local file="$1"
  require_file "$file"
  if ! grep -Eq '^[[:space:]]*"version"[[:space:]]*:[[:space:]]*"[^"]+"' "$file"; then
    echo "set-version.sh: $file: no top-level \"version\" field found" >&2
    exit 1
  fi
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
# Pre-flight grep ensures the optionalDependencies block exists —
# a deleted key would otherwise pass through silently.
rewrite_json_optional_deps() {
  local file="$1"
  require_file "$file"
  if ! grep -Eq '^[[:space:]]*"@mdsmith/[^"]+"[[:space:]]*:[[:space:]]*"[^"]+"' "$file"; then
    echo "set-version.sh: $file: no @mdsmith/* optionalDependencies pins found" >&2
    exit 1
  fi
  perl -i -pe '
    s/^(\s*"\@mdsmith\/[^"]+"\s*:\s*")[^"]+(")/$1.$ENV{V}.$2/e;
  ' "$file"
}

rewrite_pyproject_version() {
  local file="$1"
  require_file "$file"
  if ! grep -Eq '^[[:space:]]*version[[:space:]]*=[[:space:]]*"[^"]+"' "$file"; then
    echo "set-version.sh: $file: no top-level version = \"...\" field found" >&2
    exit 1
  fi
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
