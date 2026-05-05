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

check_json_version() {
  local file="$1"
  [ -f "$file" ] || return 0
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
  [ -f "$file" ] || return 0
  local mismatches
  mismatches=$(EXPECTED="$expected" perl -ne '
    if (/^\s*"\@mdsmith\/[^"]+"\s*:\s*"([^"]+)"/ && $1 ne $ENV{EXPECTED}) {
      print "$1\n";
    }
  ' "$file")
  if [ -n "$mismatches" ]; then
    while IFS= read -r v; do
      echo "$file: optionalDependencies pin '$v', want '$expected'" >&2
    done <<< "$mismatches"
    fail=1
  fi
}

check_pyproject_version() {
  local file="$1"
  [ -f "$file" ] || return 0
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
