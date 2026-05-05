#!/usr/bin/env bash
# build-npm-platforms.sh — assemble one publishable npm package per
# supported platform from the prebuilt binaries the release workflow
# downloaded as GitHub artifacts.
#
# The script reads binaries from <artifacts-dir> (laid out as the
# `merge-multiple: true` artifact download produces them) and writes
# one ready-to-publish directory under <out-dir>/<node-platform>-<arch>/
# containing a generated package.json and bin/mdsmith[.exe].
#
# Run scripts/set-version.sh BEFORE this script so the source
# manifests already pin the published version.
#
# Usage:
#   scripts/build-npm-platforms.sh <artifacts-dir> <out-dir>

set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <artifacts-dir> <out-dir>" >&2
  exit 2
fi

artifacts="$1"
out="$2"

repo_root="$(cd "$(dirname "$0")/.." && pwd)"

# (release-asset basename) -> (node-platform-arch, executable name)
# kept in lock-step with the matrix in .github/workflows/release.yml
# and the optionalDependencies block in npm/mdsmith/package.json.
build_one() {
  local asset="$1"
  local node_target="$2"
  local exe="$3"

  local src="$artifacts/$asset"
  if [ ! -f "$src" ]; then
    echo "missing release asset: $src" >&2
    exit 1
  fi

  local pkg_dir="$out/$node_target"
  mkdir -p "$pkg_dir/bin"
  install -m 0755 "$src" "$pkg_dir/bin/$exe"

  # The platform sub-package's package.json is a strict subset of the
  # root: it carries the binary, declares os/cpu so npm only installs
  # it on the matching host, and pins the same version the root
  # advertises in its optionalDependencies block.
  local node_os="${node_target%%-*}"
  local node_arch="${node_target#*-}"
  local version
  version=$(perl -ne '
    if (/^\s*"version"\s*:\s*"([^"]+)"/) { print $1; exit }
  ' "$repo_root/npm/mdsmith/package.json")
  if [ -z "$version" ]; then
    echo "could not read version from npm/mdsmith/package.json" >&2
    exit 1
  fi

  cat > "$pkg_dir/package.json" <<JSON
{
  "name": "@mdsmith/$node_target",
  "version": "$version",
  "description": "Prebuilt mdsmith binary for $node_os $node_arch.",
  "license": "MIT",
  "homepage": "https://github.com/jeduden/mdsmith",
  "repository": {
    "type": "git",
    "url": "https://github.com/jeduden/mdsmith"
  },
  "os": ["$node_os"],
  "cpu": ["$node_arch"],
  "files": ["bin/"]
}
JSON

  if [ -f "$repo_root/LICENSE" ]; then
    cp "$repo_root/LICENSE" "$pkg_dir/LICENSE"
  fi
}

mkdir -p "$out"

build_one "mdsmith-linux-amd64"        "linux-x64"     "mdsmith"
build_one "mdsmith-linux-arm64"        "linux-arm64"   "mdsmith"
build_one "mdsmith-darwin-amd64"       "darwin-x64"    "mdsmith"
build_one "mdsmith-darwin-arm64"       "darwin-arm64"  "mdsmith"
build_one "mdsmith-windows-amd64.exe"  "win32-x64"     "mdsmith.exe"

echo "built 5 platform packages under $out"
