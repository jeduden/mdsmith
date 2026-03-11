# PR Fixup — Agent Instructions

Instructions for AI coding agents (Codex, Copilot,
Claude) to push changes, monitor CI, and address review
comments until the PR is clean.

See also: [AGENTS.md](AGENTS.md) for general development
instructions.

## When to use

Run this workflow after creating or updating a PR, or
when CI fails or reviewers leave comments.

## Prerequisites

- `gh` CLI authenticated with repo access
- Git configured with push access to the remote

## Workflow

### 1. Identify the PR

```bash
PR=$(gh pr view --json number -q '.number')
BRANCH=$(git branch --show-current)
REPO=$(gh repo view --json nameWithOwner \
  -q '.nameWithOwner')
```

### 2. Push pending changes

```bash
git push origin "$BRANCH"
```

### 3. Poll CI checks until they finish

```bash
gh pr checks "$PR" --watch --fail-fast
```

If `--watch` is unavailable, poll manually:

```bash
while true; do
  STATUS=$(gh pr checks "$PR" \
    --json name,state,conclusion \
    -q '[.[] | select(.state != "COMPLETED")] | length')
  if [ "$STATUS" = "0" ]; then break; fi
  sleep 30
done
```

### 4. On CI failure — diagnose and fix

```bash
# list failed checks
gh pr checks "$PR" --json name,state,conclusion \
  -q '.[] | select(.conclusion == "FAILURE")'

# get the run ID and download logs
RUN_ID=$(gh run list --branch "$BRANCH" --limit 1 \
  --json databaseId -q '.[0].databaseId')
gh run view "$RUN_ID" --log-failed
```

Read the log, identify the root cause, fix the code,
then:

```bash
git add -A && git commit -m "fix: address CI failure"
git push origin "$BRANCH"
```

Return to step 3.

### 5. Fetch review comments

```bash
# PR-level review comments (inline code comments)
gh api "repos/$REPO/pulls/$PR/comments" \
  --paginate \
  --jq '.[] | {
    id: .id,
    node_id: .node_id,
    path: .path,
    line: .line,
    body: .body,
    user: .user.login,
    in_reply_to_id: .in_reply_to_id,
    created_at: .created_at
  }'
```

```bash
# PR issue-level comments (general discussion)
gh api "repos/$REPO/issues/$PR/comments" \
  --paginate \
  --jq '.[] | {
    id: .id,
    node_id: .node_id,
    body: .body,
    user: .user.login,
    created_at: .created_at
  }'
```

```bash
# Full reviews with state (APPROVED,
# CHANGES_REQUESTED, COMMENTED)
gh api "repos/$REPO/pulls/$PR/reviews" \
  --paginate \
  --jq '.[] | {
    id: .id,
    node_id: .node_id,
    state: .state,
    body: .body,
    user: .user.login
  }'
```

### 6. Retrieve review thread IDs for resolving

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

### 7. Address each comment

For every unresolved review thread:

1. Read the comment body and file path.
2. Make the requested change (or explain why not).
3. Reply to the thread:

```bash
gh api "repos/$REPO/pulls/$PR/comments" \
  -f body="Fixed — see latest push." \
  -F in_reply_to_id=COMMENT_ID
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

### 8. Push fixes and repeat

```bash
git add -A && git commit -m "fix: address review comments"
git push origin "$BRANCH"
```

Return to step 3 and repeat the full cycle until:

- All CI checks pass, AND
- The latest review has no unresolved comments
  (a review with state APPROVED or COMMENTED
  with zero new actionable items).

### 9. Final verification

```bash
gh pr checks "$PR"

gh api graphql -f query='
query($owner: String!, $repo: String!, $pr: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $pr) {
      reviewThreads(first: 100) {
        nodes {
          id
          isResolved
        }
      }
    }
  }
}' -f owner="${REPO%%/*}" -f repo="${REPO##*/}" \
   -F pr="$PR" \
   --jq '.data.repository.pullRequest.reviewThreads.nodes
     | map(select(.isResolved == false)) | length'
```

If the unresolved count is 0 and CI is green, the PR is
ready for merge.

## Notes

- Always run `mdsmith check .` before committing.
- Keep fix commits small and focused.
- Do not force-push; append fix commits so reviewers
  can see incremental progress.
