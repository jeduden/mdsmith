#!/bin/sh
# mdsmith merge-driver pre-merge-commit hook
set -e
set +e
'/usr/local/bin/mdsmith' fix .
status=$?
set -e
if [ "$status" -ne 0 ] && [ "$status" -ne 1 ]; then
  exit "$status"
fi
git diff --name-only -- '*.md' '*.markdown' | while IFS= read -r f; do
  if [ -n "$f" ]; then
    git add -- "$f"
  fi
done
