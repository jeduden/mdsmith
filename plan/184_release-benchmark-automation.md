---
id: 184
title: Automate the cross-tool benchmark on merge to main and publish numbers to the assets branch
status: "✅"
model: opus
depends-on: []
summary: >-
  Fold docs/research/benchmarks/run.sh into a pinned,
  integrity-verified `mdsmith-release bench` subcommand;
  run it on every merge to main and publish the refreshed
  JSON + fragments to the orphan `assets` branch (the
  demo.gif pattern) so the website serves current
  cross-tool numbers without a maintainer hand-refresh.
---
# Automate the cross-tool benchmark and publish to the assets branch

## Goal

The mdsmith-vs-Rust comparison is a hand-refreshed
artifact. See the
[benchmark research doc](../docs/research/benchmarks/README.md).
`run.sh` installs the tools, runs hyperfine, promotes
JSON, and regenerates fragments by hand.

CI (`bench-fragments`) only checks that the committed
fragments match the committed JSON. It never re-measures.
So the table goes stale after every engine change. It is
stale now. It predates both plan 175's single-core work
and the Run parallelization.

Fix this the way `demo.gif` is built. On merge to `main`,
measure. Push the result to the orphan `assets` branch.
The website pulls from there.

## Background

- `demo.yml` → `record-demo.yml`: on `push: main` (skip the
  bot to avoid loops) record the artifact, then a `publish`
  job force-updates the orphan `assets` branch with only
  that artifact, committing **only if it changed**.
  `website/layouts/partials/hero.html` pulls
  `raw.githubusercontent.com/<repo>/assets/assets/demo.gif`.
- `docs/development/release-tooling.md`: workflow runtime
  logic goes through `go run ./cmd/mdsmith-release <sub>`,
  not inline scripts. So the harness becomes a subcommand.
- `run.sh` already version-pins hyperfine/mado/panache via
  release tarballs but **without checksums**; `rumdl`
  (`uv tool install rumdl`) and `markdownlint-cli2`
  (`npm i`) are unpinned entirely.

## Decisions (locked)

1. Trigger: on merge to `main`, publish to the existing
   orphan `assets` branch under `assets/benchmarks/`
   (reuse, don't proliferate branches). Record-only —
   never gates a merge or a release.
2. Harness lives in `mdsmith-release bench` (Go owns
   install + integrity + hyperfine + promote + fragment
   regen); `run.sh` becomes a thin wrapper that calls it
   so local hand-refresh and CI run identical logic.
3. Pin **version + integrity**: every tool fetch is a
   pinned version verified by SHA-256; `markdownlint-cli2`
   via a committed lockfile + `npm ci`.
4. Lands on PR #330 (this branch).

## Tasks

1. [x] Create this plan.
2. [x] `internal/release` benchmark core + `mdsmith-release
   bench` subcommand: pinned-tool manifest
   `{tool,version,url,sha256}` (`benchTools()` in
   `internal/release/bench.go`), fail-loud SHA-256
   verifier (`verifyChecksum`), corpus build, hyperfine
   orchestration, JSON promote into
   `docs/research/benchmarks/data/` (`promoteBenchJSON`),
   fragment regen via the existing `gen_fragments.py`.
   Unit-tested the pure parts in `bench_test.go`
   (manifest invariants, verifier ok/mismatch, promote
   copy/missing-source, tar extraction).
3. [x] Pinned `rumdl` 0.1.93 and `markdownlint-cli2`
   0.22.1. Recorded real SHA-256s for the four binary
   tools (hyperfine/mado/panache/rumdl); rumdl moved from
   the unpinned `uv tool install` to its pinned GitHub
   release tarball so it shares the verify path. The
   fifth tool, `markdownlint-cli2`, is pinned by the
   committed `docs/research/benchmarks/npm/`
   package.json + package-lock.json and installs via
   `npm ci` (npm's lockfile integrity is its SHA).
4. [x] Rewrote `run.sh` as a thin wrapper over
   `go run ./cmd/mdsmith-release bench`. The
   `bench-fragments` drift gate is untouched.
5. [x] `benchmark.yml`: `push: main`, skip
   `github-actions[bot]`; `record` runs the subcommand +
   normalizes fragments; `publish` updates the `assets`
   branch under `assets/benchmarks/` (JSON + fragments),
   commit only on change. Modelled on `demo.yml`. Both
   workflows are now subtree-scoped so they coexist on
   the shared branch (see Deviations).
6. [x] Build-time fetch (chosen over the runtime model):
   `mdsmith-release pull-site-assets` pulls the published
   fragments + demo GIF into the tree during `pages.yml`;
   a scoped `mdsmith fix` rebakes the comparison page's
   `<?include?>` body. The demo GIF now ships as a
   first-party static asset (was a runtime
   `raw.githubusercontent` hotlink). `bench-fragments`
   stays coherent: it is a separate workflow that never
   calls the pull and still validates the committed
   snapshot vs committed JSON.
7. [x] Gates pass: `go build ./...`, `go test ./...`,
   `golangci-lint`, `mdsmith check .`. End-to-end
   (`benchmark.yml`, the assets-branch push, and the
   site build-time fetch) is only exercisable on merge
   to main and cannot be validated on the PR.

## Deviations

- Website mechanism: user chose **build-time fetch** over
  the runtime `demo.gif`-style fetch, and asked that
  `demo.gif` move to build-time too. Implemented as
  `pull-site-assets` in `pages.yml`; `hero.html` now
  serves a local `img/demo.gif` (`.gitignore`d).
- `demo.yml` publish made subtree-safe (dropped the
  `git rm -rf . && git clean -fdx` whole-tree wipe). Both
  it and `benchmark.yml` now touch only their own subtree
  on the shared orphan `assets` branch, so neither erases
  the other's output.
- `rumdl` is pinned via its GitHub release tarball + a
  SHA-256 (cross-checked against the publisher `.sha256`)
  rather than `uv`, so all four binary tools share one
  fetch+verify path.

## Acceptance Criteria

- [x] `mdsmith-release bench` runs the identical
      commands/flags/corpora as the old `run.sh` and
      reuses `gen_fragments.py`, so it reproduces the four
      `data/*.json` files and the fragments byte-for-byte
      on the same inputs. (The timing values are
      non-deterministic; the format/pipeline is
      identical. Full e2e only on merge to main.)
- [x] A tampered download (wrong SHA-256) fails the run
      loudly; covered by `TestVerifyChecksum` /
      `TestExtractTarGzBinary` in `bench_test.go`.
- [x] `run.sh` delegates to the subcommand and the
      `bench-fragments` drift gate is unchanged (stays
      green: committed JSON/fragments untouched).
- [x] `benchmark.yml` writes only `assets/benchmarks/`,
      commits only on change, and pushes to the `assets`
      branch — never `main`. (Exercisable only on merge.)
- [x] Website serves numbers from the `assets` branch via
      build-time fetch (`pull-site-assets` +
      scoped `mdsmith fix` in `pages.yml`). (Exercisable
      only on a site deploy.)
- [x] `go test ./...`, `golangci-lint`, `mdsmith check .`
      all pass.
