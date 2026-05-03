#!/bin/sh
# mdsmith merge-driver pre-merge-commit hook
# Re-runs mdsmith fix once git has resolved every per-file
# merge, so generated sections reflect the final merged
# state of every source file. mdsmith fix walks the worktree
# respecting .mdsmith.yml ignore patterns — the same set
# marked with merge=mdsmith in .gitattributes.
set -e
cd "$(git rev-parse --show-toplevel)"
# `set +e` around the fix invocation so we can capture its
# raw exit code. `if ! cmd; then status=$?; ...` looks
# tempting, but POSIX `! cmd` returns the logical NOT of
# cmd's exit status, so `$?` immediately after is 0 when
# cmd exited 1 — and the `[ "$status" -ne 1 ]` guard
# would then exit before the staging loop ever runs.
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
