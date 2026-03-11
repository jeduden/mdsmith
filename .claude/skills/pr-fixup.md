# PR Fixup Skill

Invoke with `/pr-fixup` after creating or updating a PR,
or when CI fails or reviewers leave comments.

<?include
file: ../../docs/development/pr-fixup-workflow.md
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

## Steps

### 1. Ensure `gh` CLI is installed

Check whether `gh` is on the PATH. If missing, download
the release tarball from GitHub and copy the binary into
`/usr/local/bin`. This approach works in sandboxes where
package-manager repos are blocked:

```bash
if ! command -v gh &>/dev/null; then
  GH_VER="2.67.0"
  ARCH=$(uname -m)
  case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *)
      echo "ERROR: unsupported arch $ARCH" >&2
      exit 1 ;;
  esac
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  URL="https://github.com/cli/cli/releases"
  URL="$URL/download/v${GH_VER}"
  URL="$URL/gh_${GH_VER}_${OS}_${ARCH}.tar.gz"
  TMP=$(mktemp -d)
  trap 'rm -rf "$TMP"' EXIT
  SUDO=""
  if command -v sudo &>/dev/null && [ "$(id -u)" -ne 0 ]; then
    SUDO="sudo "
  fi
  if curl -fsSL "$URL" -o "$TMP/gh.tar.gz" && \
     tar -xzf "$TMP/gh.tar.gz" -C "$TMP"; then
    ${SUDO}cp "$TMP"/gh_*/bin/gh /usr/local/bin/gh
  elif command -v brew &>/dev/null; then
    brew install gh
  else
    echo "ERROR: could not install gh" >&2
    echo "Install manually: https://cli.github.com" >&2
    exit 1
  fi
fi
gh --version
```

If `gh` is installed but not authenticated, run
`gh auth login` or set `GITHUB_TOKEN` in the environment.

### 2. Identify the PR

```bash
PR=$(gh pr view --json number -q '.number')
BRANCH=$(git branch --show-current)
REPO=$(gh repo view --json nameWithOwner \
  -q '.nameWithOwner')
```

### 3. Rebase onto the base branch

```bash
BASE=$(gh pr view --json baseRefName \
  -q '.baseRefName')
git fetch origin "$BASE"
git rebase "origin/$BASE"
```

If the rebase produces conflicts, resolve them, then
continue:

```bash
git rebase --continue
```

After a successful rebase, verify linting still passes:

```bash
mdsmith check .
```

### 4. Push changes

After a rebase, a force push is required (subsequent
pushes after CI/review fixes can use a regular push):

```bash
git push --force-with-lease origin "$BRANCH"
```

### 5. Poll CI checks until they finish

```bash
gh pr checks "$PR" --watch --fail-fast
```

If `--watch` is unavailable (web sandbox), poll manually:

```bash
while true; do
  STATUS=$(gh pr checks "$PR" \
    --json name,state,conclusion \
    -q '[.[] | select(.state != "COMPLETED")] | length')
  if [ "$STATUS" = "0" ]; then break; fi
  sleep 30
done
```

### 6. On CI failure — diagnose and fix

Fetch the failed job log:

```bash
# list failed checks
gh pr checks "$PR" --json name,state,conclusion \
  -q '.[] | select(.conclusion == "FAILURE")'

# get the run ID and download logs
RUN_ID=$(gh run list --branch "$BRANCH" \
  --status failure --limit 1 \
  --json databaseId -q '.[0].databaseId')
gh run view "$RUN_ID" --log-failed
```

Read the log, identify the root cause, fix the code,
then:

```bash
git add -A && git commit -m "fix: address CI failure"
git push origin "$BRANCH"
```

Return to step 5.

### 7. Fetch review comments

Retrieve all review comments on the PR:

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

### 8. Retrieve review thread IDs for resolving

GitHub review comments map to threads via GraphQL.
Query the thread node IDs:

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

This query returns the first 100 threads (10 comments
each). Paginate with `pageInfo` if the PR exceeds this.

### 9. Address each comment

For every unresolved review thread:

1. Read the comment body and file path.
2. Make the requested change (or explain why not).
3. Reply to the thread:

```bash
# Reply to an inline review comment
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

### 10. Push fixes and repeat

```bash
git add -A && git commit -m "fix: address review comments"
git push origin "$BRANCH"
```

Return to step 5 (checks) and repeat the cycle until:

- All CI checks pass, AND
- The latest review has no unresolved comments
  (a review with state APPROVED or COMMENTED
  with zero new actionable items).

### 11. Final verification

```bash
# Confirm all checks pass
gh pr checks "$PR"

# Confirm no unresolved threads remain
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

If the unresolved count is 0 and CI is green, proceed
to the notes below.

## Notes

- This workflow works in both local environments and
  Claude Code web sandbox. Step 1 installs `gh` if
  missing.
- Always run `mdsmith check .` before committing to
  catch linting issues early.
- Keep fix commits small and focused — one commit per
  CI fix, one commit per batch of related review
  comments.
- Use `--force-with-lease` only after rebase (step 3).
  After that, append fix commits with regular pushes so
  reviewers can see incremental progress.

Once the unresolved count is 0 and CI is green, the PR
is ready for merge.
<?/include?>
