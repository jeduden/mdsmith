#!/usr/bin/env bash
# check-versions.sh — assert every tracked manifest pins its version
# (and platform-package optionalDependencies) at 0.0.0-dev. The CI
# version-guard job runs this on non-tag builds; the release workflow
# rewrites the manifests with set-version.sh before publishing.
#
# Usage:
#   scripts/check-versions.sh [--root <path>]

set -euo pipefail

expected="0.0.0-dev"
repo_root=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --root)
      [ "$#" -ge 2 ] || { echo "usage: $0 [--root <path>]" >&2; exit 2; }
      repo_root="$2"
      shift 2
      ;;
    *)
      echo "usage: $0 [--root <path>]" >&2
      exit 2
      ;;
  esac
done

if [ -z "$repo_root" ]; then
  repo_root="$(cd "$(dirname "$0")/.." && pwd)"
fi

fail=0

# A required manifest going missing is a hard failure — the same
# "fail fast" stance set-version.sh takes. The CI version-guard job
# would otherwise silently pass when a tracked manifest was deleted
# or renamed and only blow up later in the tag/release workflow.
require_file() {
  local file="$1"
  if [ ! -f "$file" ]; then
    echo "check-versions.sh: required manifest missing: $file" >&2
    fail=1
    return 1
  fi
  return 0
}

check_json_version() {
  local file="$1"
  require_file "$file" || return 0
  local actual
  actual=$(perl -ne '
    if (/^\s*"version"\s*:\s*"([^"]+)"/) { print $1; exit }
  ' "$file")
  if [ "$actual" != "$expected" ]; then
    echo "$file: version is '$actual', want '$expected'" >&2
    fail=1
  fi
}

check_optional_deps() {
  local file="$1"
  require_file "$file" || return 0
  # Emit "<pkg>\t<actual>" per drifted line so the failure message
  # below can name the offending key — multiple drifted pins
  # otherwise blur together into anonymous version strings.
  local mismatches
  mismatches=$(EXPECTED="$expected" perl -ne '
    if (/^\s*"(\@mdsmith\/[^"]+)"\s*:\s*"([^"]+)"/ && $2 ne $ENV{EXPECTED}) {
      print "$1\t$2\n";
    }
  ' "$file")
  if [ -n "$mismatches" ]; then
    while IFS=$'\t' read -r pkg actual; do
      echo "$file: $pkg pin '$actual', want '$expected'" >&2
    done <<< "$mismatches"
    fail=1
  fi

  # The npm root's optionalDependencies block must list the full set
  # of platform sub-packages — a missing key would silently disable
  # one platform. Compare the keys actually present against the
  # canonical set in lock-step with .github/workflows/release.yml.
  local missing="" key
  for key in "@mdsmith/linux-x64" "@mdsmith/linux-arm64" \
             "@mdsmith/darwin-x64" "@mdsmith/darwin-arm64" \
             "@mdsmith/win32-x64"; do
    if ! grep -Eq "\"$(echo "$key" | sed 's,/,\\/,g')\"[[:space:]]*:" "$file"; then
      missing="$missing $key"
    fi
  done
  if [ -n "$missing" ]; then
    for k in $missing; do
      echo "$file: optionalDependencies missing key $k" >&2
    done
    fail=1
  fi
}

check_pyproject_version() {
  local file="$1"
  require_file "$file" || return 0
  local actual
  actual=$(perl -ne '
    if (/^\s*version\s*=\s*"([^"]+)"/) { print $1; exit }
  ' "$file")
  if [ "$actual" != "$expected" ]; then
    echo "$file: version is '$actual', want '$expected'" >&2
    fail=1
  fi
}

check_json_version "$repo_root/editors/vscode/package.json"
check_json_version "$repo_root/npm/mdsmith/package.json"
check_optional_deps "$repo_root/npm/mdsmith/package.json"

if [ -d "$repo_root/npm/platforms" ]; then
  for pkg in "$repo_root"/npm/platforms/*/package.json; do
    [ -f "$pkg" ] || continue
    check_json_version "$pkg"
  done
fi

check_pyproject_version "$repo_root/python/pyproject.toml"

if [ "$fail" -ne 0 ]; then
  exit 1
fi

echo "all manifests pinned at $expected"
