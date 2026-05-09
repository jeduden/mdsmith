---
name: gh-resolve-threads
description: >-
  Canonical workflow for resolving pull request review
  threads via the gh CLI — single source of truth that
  other skills (e.g. pr-fixup) should defer to instead
  of duplicating. MUST use whenever you need to
  resolve, fetch, or interact with PR review threads.
  GitHub MCP CANNOT resolve review threads and CANNOT
  retrieve thread IDs — do NOT attempt GitHub MCP for
  anything thread-related. Trigger on "resolve
  threads", "mark as resolved", "address review
  comments", "clean up the PR", "which comments are
  still open", or any reference to PR review feedback.
user-invocable: true
allowed-tools: >-
  Bash(gh pr:*), Bash(gh api:*), Bash(gh auth:*),
  Bash(gh --version:*),
  Bash(git push:*), Bash(git add:*),
  Bash(git commit:*), Bash(git branch:*)
---

# Resolve PR Review Threads via `gh` CLI

GitHub MCP cannot resolve threads or retrieve thread
IDs. Use `gh` CLI and GraphQL as described below. Run
each fenced block as its own Bash call — do not chain
with `&&`.

**Prerequisite:** You must be inside a git repo on the
PR's branch.

## Step 1 — Ensure `gh` is installed and authenticated

```bash
gh --version
```

If missing, install `gh` v2.92.0 from GitHub releases.
Run each block as its own Bash call so allowlist
prefix matching keeps working — every command starts
with an allowed prefix, with no leading shell
assignment, subshell, or `echo` pipe.

Download the linux_amd64 tarball:

```bash
curl -fsSL --max-time 600 \
  "https://github.com/cli/cli/releases/download/v2.92.0/gh_2.92.0_linux_amd64.tar.gz" \
  -o /tmp/gh.tar.gz
```

Verify the SHA256 before extracting. The hash below
matches the published checksums at this URL:

```text
https://github.com/cli/cli/releases/download/v2.92.0/gh_2.92.0_checksums.txt
```

The heredoc keeps `sha256sum` as the leading command:

```bash
sha256sum -c - <<'EOF'
b57848131bdf0c229cd35e1f2a51aa718199858b2e728410b37e89a428943ec4  /tmp/gh.tar.gz
EOF
```

If `sha256sum -c` reports a mismatch, stop and report
the failure — do not run the unverified binary. For
non-amd64 Linux or macOS, fetch the matching hash
from the same checksums file before downloading.

```bash
tar xz -C /tmp -f /tmp/gh.tar.gz
```

Copy the binary into a directory on `$PATH`. Use
`/usr/local/bin/gh` for a system-wide install (needs
root):

```bash
cp /tmp/gh_2.92.0_linux_amd64/bin/gh /usr/local/bin/gh
```

Or use `$HOME/.local/bin/gh` for a user-local install
(no root needed; ensure that directory is on `$PATH`).
Make sure the directory exists first:

```bash
mkdir -p "$HOME/.local/bin"
```

```bash
cp /tmp/gh_2.92.0_linux_amd64/bin/gh "$HOME/.local/bin/gh"
```

Authentication is handled by the trailing
"Authenticate if needed" block.

If that fails (redirect blocked), try apt. Each line
is its own Bash call:

```bash
type -p wget >/dev/null
```

If that exits non-zero, refresh the package index then
install wget (separate blocks so each leading command
is allowlisted):

```bash
apt-get update
```

```bash
apt-get install wget -y
```

```bash
mkdir -p -m 755 /etc/apt/keyrings
```

Download the keyring directly to its destination
(no pipe, single-command):

```bash
wget -q -O /etc/apt/keyrings/githubcli-archive-keyring.gpg \
  https://cli.github.com/packages/githubcli-archive-keyring.gpg
```

Print the architecture in its own block:

```bash
dpkg --print-architecture
```

Substitute the printed value (e.g. `amd64`) for
`ARCH_PLACEHOLDER` in the heredoc below before
running. The `<<'EOF'` quoting prevents any shell
expansion inside the body — the literal text after
substitution is what gets written:

```bash
tee /etc/apt/sources.list.d/github-cli.list > /dev/null <<'EOF'
deb [arch=ARCH_PLACEHOLDER signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main
EOF
```

```bash
apt-get update
```

```bash
apt-get install gh -y
```

If both fail, the user needs to allow
`release-assets.githubusercontent.com` or
`cli.github.com` in their network config.

Authenticate if needed. First check status:

```bash
gh auth status
```

If unauthenticated, log in. With a token in the
environment:

```bash
gh auth login --with-token <<< "${GITHUB_TOKEN}"
```

If `GITHUB_TOKEN` is not set, run the interactive
flow instead and follow the prompts:

```bash
gh auth login
```

## Step 2 — Identify PR, owner, and repo

```bash
gh pr view --json number -q '.number'
```

Note the number as `$PR`. Then fetch the head repo
owner and name as separate values (matching
`/merge-queue`):

```bash
gh pr view --json headRepositoryOwner \
  -q '.headRepositoryOwner.login'
```

Note the result as `$OWNER`. Then:

```bash
gh pr view --json headRepository \
  -q '.headRepository.name'
```

Note the result as `$REPO`.

## Step 3 — Fetch review threads

The query accepts an optional `$cursor` variable so
the same snippet works for both the first page and
subsequent pages:

```bash
gh api graphql -f query='
query($owner: String!, $repo: String!, $pr: Int!, $cursor: String) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $pr) {
      reviewThreads(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
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
}' -f owner="$OWNER" -f repo="$REPO" -F pr="$PR"
```

Returns the first 100 threads (10 comments each). If
`pageInfo.hasNextPage` is `true`, rerun the same call
with `-f cursor="<endCursor>"` appended (using the
`endCursor` returned in the previous response). Repeat
until `hasNextPage` is `false`:

```bash
gh api graphql -f query='
query($owner: String!, $repo: String!, $pr: Int!, $cursor: String) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $pr) {
      reviewThreads(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
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
}' -f owner="$OWNER" -f repo="$REPO" \
   -F pr="$PR" -f cursor="<endCursor>"
```

Otherwise unresolved threads beyond the first 100 are
silently omitted.

For each unresolved thread, note its `id` and read the
comment at `comments.nodes[0]` to understand what to
fix and where.

## Step 4 — Address comments, commit, push

Make the code changes for each unresolved thread. Skip
threads you cannot or should not address. Then:

```bash
git add -A
```

```bash
git commit -m "fix: address review comments"
```

```bash
git branch --show-current
```

Note the branch as `$BRANCH`. Then:

```bash
git push origin "$BRANCH"
```

After every push, request a Copilot re-review so the
bot looks at the latest commit:

```bash
gh api --method POST \
  "repos/$OWNER/$REPO/pulls/$PR/requested_reviewers" \
  -f 'reviewers[]=copilot-pull-request-reviewer[bot]'
```

## Step 5 — Resolve addressed threads

For each thread you addressed, resolve it using its
`id` from step 3:

```bash
gh api graphql -f query='
mutation($threadId: ID!) {
  resolveReviewThread(input: {threadId: $threadId}) {
    thread { id isResolved }
  }
}' -f threadId="THREAD_NODE_ID"
```

Resolve one at a time. Do NOT resolve threads you did
not address.

## Step 6 — Verify

Re-run the step 3 query. Confirm addressed threads
show `"isResolved": true`. Report to the user what
was resolved and what remains open.
