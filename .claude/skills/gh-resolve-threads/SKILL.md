---
name: gh-resolve-threads
description: >-
  Resolve pull request review threads using the gh
  CLI. MUST use this skill whenever you need to
  resolve, fetch, or interact with PR review threads.
  GitHub MCP CANNOT resolve review threads and CANNOT
  retrieve thread IDs — do NOT attempt GitHub MCP for
  anything thread-related. Trigger on "resolve
  threads", "mark as resolved", "address review
  comments", "clean up the PR", "which comments are
  still open", or any reference to PR review feedback.
  If already in the pr-fixup skill, still follow these
  steps for thread resolution — do not improvise.
user-invocable: true
allowed-tools: >-
  Bash(gh pr:*), Bash(gh api:*), Bash(gh auth:*),
  Bash(gh --version:*),
  Bash(curl:*), Bash(sha256sum:*), Bash(tar:*),
  Bash(cp:*),
  Bash(apt-get:*), Bash(wget:*), Bash(tee:*),
  Bash(mkdir:*), Bash(dpkg:*), Bash(type:*),
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

If missing, install from GitHub releases. The block
below downloads `gh` v2.92.0 and verifies the
published SHA256 before extracting:

```bash
GH_VERSION=2.92.0
# Hash for gh_${GH_VERSION}_linux_amd64.tar.gz from
# https://github.com/cli/cli/releases/download/v2.92.0/gh_2.92.0_checksums.txt
GH_SHA256=b57848131bdf0c229cd35e1f2a51aa718199858b2e728410b37e89a428943ec4
curl -fsSL --max-time 600 \
  "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_amd64.tar.gz" \
  -o /tmp/gh.tar.gz
echo "${GH_SHA256}  /tmp/gh.tar.gz" | sha256sum -c -
tar xz -C /tmp -f /tmp/gh.tar.gz
cp /tmp/gh_${GH_VERSION}_linux_amd64/bin/gh /usr/local/bin/gh
echo "${GITHUB_TOKEN}" | gh auth login --with-token
```

If `sha256sum -c` reports a mismatch, stop and report
the failure — do not run the unverified binary. For
non-amd64 Linux or macOS, fetch the matching hash
from the same checksums file before downloading.

If that fails (redirect blocked), try apt:

```bash
(type -p wget >/dev/null || apt-get install wget -y)
mkdir -p -m 755 /etc/apt/keyrings
wget -qO- https://cli.github.com/packages/githubcli-archive-keyring.gpg \
  | tee /etc/apt/keyrings/githubcli-archive-keyring.gpg > /dev/null
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
  | tee /etc/apt/sources.list.d/github-cli.list > /dev/null
apt-get update && apt-get install gh -y
```

If both fail, the user needs to allow
`release-assets.githubusercontent.com` or
`cli.github.com` in their network config.

Authenticate if needed:

```bash
gh auth status
```

```bash
echo "${GITHUB_TOKEN}" | gh auth login --with-token
```

## Step 2 — Identify PR and repo

```bash
gh pr view --json number -q '.number'
```

Note the number as `$PR`. Then:

```bash
gh pr view --json headRepository \
  -q '.headRepository.owner.login + "/" + .headRepository.name'
```

Note the repo (e.g. `owner/name`) as `$REPO`.

## Step 3 — Fetch review threads

```bash
gh api graphql -f query='
query($owner: String!, $repo: String!, $pr: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $pr) {
      reviewThreads(first: 100) {
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
}' -f owner="${REPO%%/*}" -f repo="${REPO##*/}" -F pr="$PR"
```

Returns the first 100 threads (10 comments each). If
`pageInfo.hasNextPage` is `true`, paginate by passing
the returned `endCursor` as an `after:` argument until
it is `false` — otherwise unresolved threads beyond the
first 100 will be silently omitted.

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
git push origin "$(git branch --show-current)"
```

After every push, request a Copilot re-review so the
bot looks at the latest commit:

```bash
gh api --method POST \
  "repos/$REPO/pulls/$PR/requested_reviewers" \
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
