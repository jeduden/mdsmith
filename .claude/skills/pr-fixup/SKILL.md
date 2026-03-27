---
name: pr-fixup
description: >-
  Push changes, monitor CI, and address review comments
  until the PR is clean. Run after creating or updating
  a PR, or when CI fails or reviewers leave comments.
user-invocable: true
allowed-tools: >-
  Bash(gh pr:*), Bash(gh api:*), Bash(gh run:*),
  Bash(git push:*), Bash(git fetch:*),
  Bash(git rebase:*), Bash(git branch:*),
  Bash(git add:*), Bash(git commit:*),
  Bash(go run ./cmd/mdsmith:*)
argument-hint: "[PR number]"
---

<?include
file: ../../../docs/development/pr-fixup-workflow.md
strip-frontmatter: "true"
?>
Push changes, monitor CI, and address review comments
until the PR is clean. Run this workflow after creating
or updating a PR, or when CI fails or reviewers leave
comments.

## Prerequisites

- Git configured with push access to the remote
- `gh` CLI authenticated with repo access (step 1
  installs it if missing)
- Run each code block as its own Bash call — do not
  chain with `&&` or prefix with inline variable
  assignments (`VAR=x cmd`). Permission patterns match
  on the command prefix, so `gh api ...` must be the
  first token in the command

## Steps

### 1. Ensure `gh` CLI is installed

```bash
gh pr --help
```

If missing, install from
[cli.github.com](https://cli.github.com):

```bash
brew install gh
```

If `brew` is unavailable, download the release tarball
from the [GitHub releases page](https://github.com/cli/cli/releases).
If not authenticated, run `gh auth login` or set
`GITHUB_TOKEN`.

### 2. Identify the PR

Store the PR number, branch, and repo name for later
steps. Run each command separately:

```bash
gh pr view --json number -q '.number'
```

Note the number as `$PR`. Then:

```bash
git branch --show-current
```

Note the branch as `$BRANCH`. Then:

```bash
gh pr view --json headRepository \
  -q '.headRepository.owner.login + "/" + .headRepository.name'
```

Note the repo as `$REPO`.

### 3. Rebase onto the base branch

```bash
gh pr view --json baseRefName -q '.baseRefName'
```

Note the base branch as `$BASE`. Then:

```bash
git fetch origin "$BASE"
```

```bash
git rebase "origin/$BASE"
```

If the rebase produces conflicts, resolve them, then
continue:

```bash
git rebase --continue
```

After a successful rebase, verify linting still passes:

```bash
go run ./cmd/mdsmith check .
```

```bash
go tool golangci-lint run --fix ./...
```

The `--fix` flag auto-corrects formatting issues
(goimports, gofmt). If it produces changes, stage and
include them in the next commit.

### 4. Push changes

After a rebase, a force push is required (subsequent
pushes after CI/review fixes can use a regular push):

```bash
git push --force-with-lease origin "$BRANCH"
```

### 5. Schedule recurring polling

Use the `/loop` skill to re-invoke `/pr-fixup` every
minute. This avoids blocking bash loops and long-running
`--watch` commands:

```text
/loop 1m /pr-fixup
```

On each invocation, check CI and review threads. If
everything is green and resolved, cancel the loop
(step 10). If action is needed, handle it inline and
let the next loop iteration verify the result.

Check CI status:

```bash
gh pr checks "$PR" --json name,state
```

If all checks show `SUCCESS`, proceed to step 7. If
any show `FAILURE`, proceed to step 6. If checks are
still `IN_PROGRESS` or `PENDING`, wait for the next
loop iteration.

### 6. On CI failure — diagnose and fix

Fetch the failed job log:

```bash
gh pr checks "$PR" --json name,state,conclusion \
  -q '.[] | select(.conclusion == "FAILURE")'
```

Get the run ID of the most recent failure:

```bash
gh run list --branch "$BRANCH" \
  --status failure --limit 1 \
  --json databaseId -q '.[0].databaseId'
```

Note the run ID as `$RUN_ID`. Then download the log:

```bash
gh run view "$RUN_ID" --log-failed
```

Read the log, identify the root cause, fix the code,
then run linters before committing:

```bash
go tool golangci-lint run --fix ./...
```

```bash
git add -A
```

```bash
git commit -m "fix: address CI failure"
```

```bash
git push origin "$BRANCH"
```

The next `/loop` iteration will re-check CI.

### 7. Fetch review threads

Fetch all review threads with their comments, paths,
and resolution status in a single GraphQL call:

```bash
gh api graphql -f query='
query($owner: String!, $repo: String!, $pr: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $pr) {
      reviewThreads(first: 100) {
        nodes {
          id
          isResolved
          comments(first: 10) {
            nodes {
              body
              author { login }
              path
              line
            }
          }
        }
      }
    }
  }
}' -f owner="${REPO%%/*}" -f repo="${REPO##*/}" \
   -F pr="$PR"
```

Returns the first 100 threads (10 comments each).
Paginate with `pageInfo` if the PR exceeds this.

### 8. Address each comment

For every unresolved review thread:

1. Read the comment body and file path.
2. Make the requested change (or explain why not).
3. Reply to the thread:

```bash
gh api "repos/$REPO/pulls/$PR/comments" \
  -f body="Fixed — see latest push." \
  -F in_reply_to=COMMENT_ID
```

4. Resolve the thread:

```bash
gh api graphql -f query='
mutation($threadId: ID!) {
  resolveReviewThread(input: {threadId: $threadId}) {
    thread { id isResolved }
  }
}' -f threadId="THREAD_NODE_ID"
```

### 9. Push fixes and repeat

```bash
go tool golangci-lint run --fix ./...
```

```bash
git add -A
```

```bash
git commit -m "fix: address review comments"
```

```bash
git push origin "$BRANCH"
```

The next `/loop` iteration will re-check CI and
threads automatically.

### 10. Final verification and loop cancellation

```bash
gh pr checks "$PR"
```

Re-run the step 7 query and filter for unresolved
threads. If the count is 0 and CI is green, cancel
the recurring loop and report that the PR is ready
for merge.

## Notes

- This workflow works in both local environments and
  Claude Code web sandbox. Step 1 installs `gh` if
  missing.
- Always run `mdsmith check .` and
  `golangci-lint run --fix ./...` before committing to
  catch linting and formatting issues early.
- Keep fix commits small and focused — one commit per
  CI fix, one commit per batch of related review
  comments.
- Use `--force-with-lease` only after rebase (step 3).
  After that, append fix commits with regular pushes so
  reviewers can see incremental progress.

Once the unresolved count is 0 and CI is green, the PR
is ready for merge.
<?/include?>
