#!/bin/sh
# mdsmith merge-driver pre-merge-commit hook
set -e
cd "$(git rev-parse --show-toplevel)"
set +e
'/usr/local/bin/mdsmith' fix .
status=$?
set -e
if [ "$status" -ne 0 ] && [ "$status" -ne 1 ]; then
  exit "$status"
fi
git diff --name-only -- '*.md' '*.markdown' | xargs git add --
