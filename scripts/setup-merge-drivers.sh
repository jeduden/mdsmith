#!/usr/bin/env bash
# setup-merge-drivers.sh — register the catalog merge driver
# in the local git config. Run once after cloning.
#
# The .gitattributes file assigns the "catalog" merge driver
# to PLAN.md and README.md. This script tells git which
# command to invoke for that driver.

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"

git config merge.catalog.name "Catalog-aware Markdown merge"
git config merge.catalog.driver \
  "${REPO_ROOT}/scripts/merge-driver-catalog.sh %O %A %B %P"

echo "Merge driver 'catalog' registered for this repo."
echo "Files using it: see .gitattributes"
