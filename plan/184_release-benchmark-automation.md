---
id: 184
title: Automate the cross-tool benchmark on merge to main and publish numbers to the assets branch
status: "🔳"
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

The mdsmith-vs-Rust comparison
([benchmark research doc](../docs/research/benchmarks/README.md))
is a hand-refreshed artifact: `run.sh` installs the tools,
runs hyperfine, promotes JSON, regenerates fragments. CI
(`bench-fragments`) only checks the committed fragments
match the committed JSON — it never re-measures. So the
table drifts stale after every engine change (it is stale
right now: it predates plan 175's single-core work *and*
the Run parallelization).

Automate it the way `demo.gif` is automated: on merge to
`main`, measure, and push the result to the orphan
`assets` branch; the website pulls from there.

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
2. [ ] `internal/release` benchmark core + `mdsmith-release
   bench` subcommand: pinned-tool manifest
   `{tool,version,url,sha256}`, fail-loud SHA-256
   verifier, corpus build, hyperfine orchestration, JSON
   promote into `docs/research/benchmarks/data/`, fragment
   regen via the existing `gen_fragments.py`. Unit-test
   the pure parts (manifest invariants, verifier
   ok/mismatch, promote copy/missing-source).
3. [ ] Choose + pin `rumdl` and `markdownlint-cli2`
   versions; record real SHA-256s for all five tools;
   commit `markdownlint-cli2` lockfile, install via
   `npm ci`.
4. [ ] Rewrite `run.sh` as a wrapper over
   `go run ./cmd/mdsmith-release bench` (no behaviour
   change for the local hand-refresh; keeps the
   `bench-fragments` drift gate intact).
5. [ ] `benchmark.yml`: `push: main`, skip
   `github-actions[bot]`; `record` runs the subcommand;
   `publish` force-updates `assets` branch under
   `assets/benchmarks/` (JSON + fragments), commit only
   on change. Model on `demo.yml`.
6. [ ] Website + docs: serve headline/results from the
   `assets` branch like `demo.gif`; keep `bench-fragments`
   coherent (it still validates committed fragments vs
   committed JSON as the in-repo snapshot).
7. [ ] Gates: `go build`, `go test ./...`,
   `golangci-lint`, `mdsmith check .`. End-to-end
   (`benchmark.yml`) is only exercisable on merge to main —
   note this explicitly; it cannot be validated on the PR.

## Acceptance Criteria

- [ ] `mdsmith-release bench` reproduces the four
      `data/*.json` files and the fragments byte-for-byte
      vs the current `run.sh` on the same inputs.
- [ ] A tampered download (wrong SHA-256) fails the run
      loudly; covered by a unit test.
- [ ] `run.sh` delegates to the subcommand and the
      `bench-fragments` drift gate stays green.
- [ ] `benchmark.yml` pushes to `assets/benchmarks/` only
      when numbers change and never to `main`.
- [ ] Website renders numbers fetched from the `assets`
      branch.
- [ ] `go test ./...`, `golangci-lint`, `mdsmith check .`
      all pass.
