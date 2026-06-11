#!/bin/sh
# mdsmith merge-driver pre-merge-commit hook
# Re-runs mdsmith fix once git has resolved every per-file
# merge, so generated sections reflect the final merged
# state of every source file. mdsmith fix walks the worktree
# respecting .mdsmith.yml ignore patterns — the same set
# marked with merge=mdsmith in .gitattributes.
set -e
cd "$(git rev-parse --show-toplevel)"
# Stage one path, retrying a transient .git/index.lock with
# bounded backoff. Never removes a lock it did not create; a
# persistent lock exits non-zero with a clear message.
mdsmith_git_add() {
  _attempt=0
  while :; do
    _err=$(git add -- "$1" 2>&1)
    _status=$?
    [ "$_status" -eq 0 ] && return 0
    case "$_err" in
      *index.lock*"File exists"*)
        if [ "$_attempt" -ge 5 ]; then
          echo "mdsmith pre-merge-commit hook: index locked: $_err" >&2
          exit 1
        fi
        _attempt=$((_attempt + 1))
        sleep 0.1 2>/dev/null || sleep 1
        ;;
      *)
        echo "$_err" >&2
        exit "$_status"
        ;;
    esac
  done
}
# `set +e` around the fix invocation so we can capture its
# raw exit code. `if ! cmd; then status=$?; ...` looks
# tempting, but POSIX `! cmd` returns the logical NOT of
# cmd's exit status, so `$?` immediately after is 0 when
# cmd exited 1 — and the `[ "$status" -ne 1 ]` guard
# would then exit before the staging loop ever runs.
set +e
'/usr/local/bin/mdsmith' fix --no-build .
status=$?
if [ "$status" -ne 0 ] && [ "$status" -ne 1 ]; then
  exit "$status"
fi
# Stay under `set +e`: mdsmith_git_add captures each `git add`
# exit status to classify a lock failure, and exits on a hard
# error. The `while` loop runs in the pipeline's subshell, so a
# `mdsmith_git_add` exit there ends only the subshell; capture
# the pipeline status afterward and re-raise it so a persistent
# lock (or other hard error) aborts the whole hook.
#
# Capture the changed-file list first and check `git diff`'s own
# exit status. Piping `git diff` straight into the loop would tie
# $? to the `while` (which exits 0 on empty input), masking a
# hard `git diff` failure and committing without staging fixes.
changed_md=$(git diff --name-only -- '*.md' '*.markdown')
diff_status=$?
if [ "$diff_status" -ne 0 ]; then
  exit "$diff_status"
fi
printf '%s\n' "$changed_md" | while IFS= read -r f; do
  if [ -n "$f" ]; then
    mdsmith_git_add "$f"
  fi
done
stage_status=$?
if [ "$stage_status" -ne 0 ]; then
  exit "$stage_status"
fi
