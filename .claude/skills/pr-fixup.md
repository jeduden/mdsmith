# PR Fixup Skill

Invoke with `/pr-fixup` after creating or updating a PR,
or when CI fails or reviewers leave comments.

<?include
file: ../../docs/guides/pr-fixup-workflow.md
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

Before running any `gh` commands, verify it is available.
If not, install it using the system package manager:

```bash
if ! command -v gh &>/dev/null; then
  if command -v apt-get &>/dev/null; then
    # Debian / Ubuntu (includes Claude Code web sandbox)
    (type -p wget >/dev/null || \
      sudo apt-get update && \
      sudo apt-get install -y wget) && \
    sudo mkdir -p -m 755 \
      /etc/apt/keyrings && \
    out=$(mktemp) && \
    wget -nv -O"$out" \
      https://cli.github.com/packages/githubcli-archive-keyring.gpg && \
    cat "$out" \
      | sudo tee /etc/apt/keyrings/githubcli-archive-keyring.gpg \
      >/dev/null && \
    sudo chmod go+r \
      /etc/apt/keyrings/githubcli-archive-keyring.gpg && \
    echo "deb [arch=$(dpkg --print-architecture) \
      signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] \
      https://cli.github.com/packages stable main" \
      | sudo tee /etc/apt/sources.list.d/github-cli.list \
      >/dev/null && \
    sudo apt-get update && \
    sudo apt-get install -y gh
  elif command -v dnf &>/dev/null; then
    # Fedora / RHEL
    sudo dnf install -y gh
  elif command -v pacman &>/dev/null; then
    # Arch Linux
    sudo pacman -S --noconfirm github-cli
  elif command -v brew &>/dev/null; then
    # macOS / Homebrew on Linux
    brew install gh
  else
    echo "ERROR: no supported package manager found" >&2
    echo "Install gh manually: https://cli.github.com" >&2
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

### 3. Push pending changes

```bash
git push origin "$BRANCH"
```

### 4. Poll CI checks until they finish

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

### 5. On CI failure — diagnose and fix

Fetch the failed job log:

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

Return to step 4.

### 6. Fetch review comments

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

### 7. Retrieve review thread IDs for resolving

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

### 8. Address each comment

For every unresolved review thread:

1. Read the comment body and file path.
2. Make the requested change (or explain why not).
3. Reply to the thread:

```bash
# Reply to an inline review comment
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

### 9. Push fixes and repeat

```bash
git add -A && git commit -m "fix: address review comments"
git push origin "$BRANCH"
```

Return to step 4 and repeat the full cycle until:

- All CI checks pass, AND
- The latest review has no unresolved comments
  (a review with state APPROVED or COMMENTED
  with zero new actionable items).

### 10. Final verification

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
- Do not force-push; append fix commits so reviewers
  can see incremental progress.

Once the unresolved count is 0 and CI is green, the PR
is ready for merge.
<?/include?>
