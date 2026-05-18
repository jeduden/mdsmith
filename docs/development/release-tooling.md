---
title: Release Tooling Architecture
summary: >-
  Every GitHub Actions workflow that needs runtime
  logic invokes the `mdsmith-release` Go CLI rather
  than carrying inline shell or per-language scripts.
  This page captures the rule and the subcommands it
  applies to.
---
# Release Tooling Architecture

`mdsmith-release` is the single Go binary the GitHub
Actions workflows lean on for runtime logic. The
rule is short.

## The rule

Workflow steps that need runtime logic invoke
`go run ./cmd/mdsmith-release <subcommand>`. They do
not run inline scripts in TypeScript, Python, or any
other interpreter.

The CLI is not part of the user-facing `mdsmith`
binary. Its subcommands are release-pipeline
plumbing and have no value outside the workflow.

## Why one binary

Every alternative was tried at least once.

Inline shell is fine for one-liners. It collapses
under conditionals, date math, or YAML parsing.

Per-language helpers (TS via bun, Python) each add a
runtime to set up. Each adds a lockfile, a config,
a CI job, and a Codecov flag. The repo accumulated
three coverage flags before this rule was written.

A second Go binary splits the build matrix. It
invites code drift between two `internal/release`
consumers.

A single Go binary co-located with the rest of the
release tooling wins on every axis. One `go.mod`,
one test suite, one coverage flag, one workflow
setup step.

## Subcommands

| Subcommand                  | Invoked by                                 |
|-----------------------------|--------------------------------------------|
| `stamp <version>`           | `release.yml` publishing jobs; `pages.yml` |
| `publish-release`           | `release.yml` release job                  |
| `check`                     | `ci.yml` version-guard                     |
| `build-npm <art> <out>`     | `release.yml` npm job                      |
| `build-wheels <art> <out>`  | `release.yml` pypi job                     |
| `sync-docs <src> <dst>`     | composed by `build-website`                |
| `build-website [flags]`     | `pages.yml` deploy job                     |
| `check-secret-rotations`    | `secret-rotation-reminder.yml`             |
| `record-rotation <t> <d>`   | `record-secret-rotation.yml`               |
| `merge-coverage -o <o> <p>` | `ci.yml` test job                          |
| `bench [workdir]`           | `benchmark.yml` record job; `run.sh`       |
| `pull-site-assets`          | `pages.yml` deploy job                     |

Each subcommand lives under `cmd/mdsmith-release/`.
It delegates to a function in `internal/release/`.
The split keeps argument parsing thin. The logic
rides the same `go test` suite as the rest of the
package.

## What goes where

`mdsmith` is for content operations. Any command a
contributor runs locally lives here.

`mdsmith-release` is for pipeline plumbing. Only CI
runs these. The layout and env are baked in.

For new logic, ask: does a contributor ever run
this locally? If yes, it goes in `mdsmith`. If no,
in `mdsmith-release`.

## Adding a new workflow surface

The pattern:

1. Add the function under `internal/release/`.
   Pure logic gets its own file. Orchestrators that
   shell out (e.g. `gh`) keep the command
   injectable so tests can swap in a fake.
2. Add a subcommand to
   `cmd/mdsmith-release/main.go`. Parse arguments
   and call the function. Match the existing
   pattern: pflag with `ContinueOnError`, a
   `Usage` closure, and `reportError` for exit
   codes.
3. Add Go tests under `internal/release/`. Pure
   functions get table-driven tests. Filesystem
   logic gets a `t.TempDir()` fixture.
4. Update the workflow YAML to invoke
   `go run ./cmd/mdsmith-release <subcommand>`.
   Drop any prior interpreter setup steps.

The result is one less language in the repo and one
less CI job to keep green.

## Test fixtures

`buildnpm_test.go` asserts the matrix matches the
canonical npm-channel doc. The bullet list under
`release-channels/npm.md` is the source of truth.

`secretrotations_test.go` uses `t.TempDir()` files.
A shell stub stands in for the `gh` binary. No live
API calls run in tests.

The pattern for any new subcommand: pure logic plus
a small fake for each external command.
