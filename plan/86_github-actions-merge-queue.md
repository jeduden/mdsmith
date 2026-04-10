---
id: 86
title: 'GitHub Actions merge queue'
status: "🔲"
summary: >-
  Bors-style merge queue using only GitHub Actions:
  label state machine, batch merging, binary bisection
  on failure, and escalation triggers.
---
# GitHub Actions merge queue

## Goal

Build a merge queue that serializes PR merges to
`main` using only GitHub Actions. No external server.
No GitHub native merge queue. Supports batch testing
and binary bisection on CI failure, modeled after Gas
Town's Refinery.

## Background

GitHub's native merge queue needs a Team or Enterprise
plan for private repos. It offers limited control over
batching and failure handling. Gas Town's Refinery
implements Bors-style queuing (batch, test tip, bisect
on failure) but is coupled to the Gas Town daemon.

This plan builds the same algorithm as a standalone
GitHub Actions workflow. Labels drive the state
machine. Branches serve as the batch mechanism.

## Design overview

### State machine

Labels on PRs encode queue state:

| Label           | Meaning                       |
|-----------------|-------------------------------|
| `queue`         | Author requests merge         |
| `queue:pending` | Waiting in line               |
| `queue:active`  | Currently in a batch under CI |
| `queue:failed`  | Batch CI failed, ejected      |

A single concurrency group (`merge-queue`) ensures
only one queue run executes at a time.

### Branch naming

Batch branches live under `merge-queue/`:

- `merge-queue/batch-<run_id>` — combined PRs
- `merge-queue/bisect-<run_id>-<half>` — bisection

All batch branches are ephemeral. They are deleted
after merge or failure.

### Core algorithm

```text
1. List PRs labeled queue or queue:pending
   (oldest first).
2. Relabel all as queue:active.
3. Create batch branch from main.
4. Merge each PR into batch branch.
   Conflict → eject PR (label queue:failed).
5. Run CI on batch branch tip.
6. CI passes → fast-forward main, close PRs.
7. CI fails, batch = 1 → label queue:failed.
8. CI fails, batch > 1 → binary bisect.
9. Check for newly queued PRs. Re-trigger if any.
```

### Binary bisection

When a batch of N PRs fails CI:

```text
1. Split into L (first half) and R (rest).
2. Test L on a bisect branch from main.
3. L passes → failure is in R.
   Merge L to main. Re-queue R for bisect.
4. L fails → failure is in L.
   Park R as queue:pending. Re-queue L.
5. Recurse until batch = 1 isolates the
   failing PR.
```

Worst case: `ceil(log2(N)) + 1` CI runs. A batch
of 8 needs at most 4 runs to find the failure.

### Workflow files

#### ci-reusable.yml

Extract the existing CI jobs (`lint`, `test`,
`mdsmith`, `demo`) into a reusable workflow that
accepts a `ref` input. Each job checks out
`inputs.ref` instead of the default ref.

```yaml
on:
  workflow_call:
    inputs:
      ref:
        required: true
        type: string
```

#### ci.yml (updated)

Thin wrapper calling `ci-reusable.yml` with the
default SHA. Keeps `push`, `pull_request`, and
`merge_group` triggers unchanged.

#### merge-queue.yml

Primary orchestrator. Triggers on `pull_request:
[labeled]` and `workflow_dispatch` (for self-
re-trigger and bisection inputs).

```yaml
concurrency:
  group: merge-queue
  cancel-in-progress: false
```

Jobs:

1. **prepare** — Collect queued PRs from labels or
   dispatch input. Output JSON PR list.
2. **batch** — Create batch branch, merge PRs.
   Eject conflicts.
3. **verify** — Call `ci-reusable.yml` against the
   batch branch.
4. **merge-or-bisect** — Fast-forward main on pass.
   Trigger bisection dispatch on fail.
5. **notify** — Comment on each PR with result.
6. **cleanup** — Delete branches. Re-trigger if
   more PRs are queued.

### Authentication

The workflow needs `contents: write` and
`pull-requests: write`. Use `GITHUB_TOKEN` if branch
protection allows Actions to push. Otherwise use a
fine-grained PAT stored as a repo secret.

## Phases

### Phase 1 — Serial queue (MVP)

One PR at a time. No batching, no bisection.

- Scan for oldest `queue`-labeled PR
- Create batch branch with single PR, run CI
- Pass → merge to main. Fail → label `queue:failed`
- Re-trigger for next queued PR

Validates the state machine, branch management, CI
reuse, and merge mechanics.

### Phase 2 — Batch merging

Combine up to N queued PRs (default 5) into one
batch branch. Run CI once on the tip.

- Pass → fast-forward main, close all PRs
- Fail → proceed to phase 3

### Phase 3 — Binary bisection

On batch failure, self-invoke via `workflow_dispatch`
with `bisect: true` and `batch_prs` JSON input.
Recursively split until the failing PR is isolated.

## Escalation: when you need a Bors server

A GitHub Actions queue has inherent limits. Migrate
to a dedicated service (Bors-NG, Mergify, Kodiak,
or Gas Town Refinery) when any trigger fires.

### Throughput triggers

- **Queue depth > 10 PRs regularly.** One batch at
  a time. CI at 5 min means 30+ min to drain 10 PRs
  with bisection.
- **CI > 15 minutes.** Long CI multiplied by bisect
  rounds creates unacceptable latency. A server can
  run parallel speculative CI.
- **More than 20 PRs merged per day.** The label
  state machine becomes a bottleneck.

### Feature triggers

- **Priority queues.** Labels have no ordering. A
  server can implement priority lanes.
- **Cross-repo coordination.** Merging repo A
  depends on repo B. Actions queues are per-repo.
- **Dependent PR chains.** Stacked PRs need
  dependency tracking labels cannot express.
- **Rollback automation.** Auto-revert on post-merge
  failure requires persistent state.

### Reliability triggers

- **Label race conditions.** Concurrent label events
  can cause duplicate merges. A server has
  transactional state.
- **Actions outages.** A self-hosted server with its
  own runners is more resilient.
- **Audit trail needed.** Actions logs are ephemeral.
  A server provides persistent history and metrics.

### Cost triggers

- **Minutes budget exceeded.** Bisection multiplies
  CI runs. A batch of 8 that fails costs up to 4x
  the normal CI bill.

### Decision matrix

| Scenario          | Actions queue | Bors server |
|-------------------|---------------|-------------|
| Solo / small team | sufficient    | overkill    |
| < 10 PRs/day      | sufficient    | optional    |
| 10-20 PRs/day     | monitor       | recommended |
| > 20 PRs/day      | too slow      | required    |
| CI < 5 min        | sufficient    | optional    |
| CI 5-15 min       | monitor       | recommended |
| CI > 15 min       | bisect lag    | required    |
| Priority merges   | no support    | required    |
| Cross-repo deps   | no support    | required    |
| Stacked PRs       | no support    | required    |

## Tasks

1. Create labels (`queue`, `queue:pending`,
   `queue:active`, `queue:failed`) via `gh label`
2. Extract `ci-reusable.yml` from `ci.yml` with
   `ref` input parameter
3. Update `ci.yml` to call `ci-reusable.yml`
4. Implement phase 1 serial queue workflow
5. Test phase 1: enqueue PR, verify CI runs on
   batch branch, confirm merge to main
6. Test phase 1 failure: verify `queue:failed`
   label and comment on CI failure
7. Implement phase 2 batch merging
8. Implement phase 3 binary bisection via
   `workflow_dispatch` self-invocation
9. Test phase 3: enqueue 4 PRs (one fails),
   verify bisection isolates the failure
10. Add `/queue` comment trigger via
    `issue_comment` event
11. Document usage in `docs/development/`

## Acceptance Criteria

- [ ] Serial queue: PR labeled `queue` merges to
      `main` after CI passes on batch branch
- [ ] Failed CI: PR ejected with `queue:failed`
      label and explanatory comment
- [ ] Batch merge: up to N queued PRs merged in
      one CI run when all pass
- [ ] Bisection: batch failure triggers binary
      bisect; failing PR isolated, passing PRs merge
- [ ] Merge conflicts: PR that cannot merge into
      batch branch is ejected immediately
- [ ] Re-trigger: workflow checks for new queued
      PRs after each run and self-triggers
- [ ] Existing CI unchanged: `push`/`pull_request`
      triggers work as before
- [ ] Escalation triggers documented
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
