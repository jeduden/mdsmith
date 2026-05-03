#!/bin/sh
# mdsmith merge-driver pre-merge-commit hook
set -e
cd "$(git rev-parse --show-toplevel)"
'/usr/local/bin/mdsmith' fix . || true
git diff --name-only -- '*.md' '*.markdown' | while IFS= read -r f; do
  if [ -n "$f" ]; then
    git add -- "$f"
  fi
done
