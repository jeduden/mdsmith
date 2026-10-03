---
id: 2610031843
title: Name the plan9 spawn-test tag in the developer guide
status: "🔲"
model: haiku
summary: >-
  Plan 2610030243 retagged the spawn test files
  `unix || windows || plan9`, but the developer guide
  still tells contributors to tag them
  `unix || windows`. The guide is included into
  CLAUDE.md, AGENTS.md, and the Copilot instructions,
  so changing it needs the user's consent.
depends-on: [2610030243]
---
# Name the plan9 spawn-test tag in the developer guide

## Goal

The developer guide names the spawn-test build tag that
CI enforces, so new spawn test files stay in the plan9
vet.

## Background

Plan
[2610030243](2610030243_plan9-vet-unix-windows-build-tests.md)
retagged the spawn test files in `internal/build`,
`internal/release`, and `cmd/mdsmith-release`. The new
tag is `unix || windows || plan9`, so
`GOOS=plan9 go vet ./...` type-checks them. The "Test
Fixtures" section of
[docs/development/index.md](../docs/development/index.md)
still says to tag them `//go:build unix || windows`.

`TestProcTestFilesCoverPlan9` fails on that tag in
`internal/build`, so a contributor who follows the
guide gets a red test. In the other packages the old
tag drops the file from the plan9 vet with no error.

That guide is `<?include?>`d into
[CLAUDE.md](../CLAUDE.md), [AGENTS.md](../AGENTS.md),
and `.github/copilot-instructions.md`. Editing it and
running `mdsmith fix` regenerates all three. CLAUDE.md
changes need the user's own approval, so the agent that
ran plan 2610030243 deferred this edit.

## Tasks

1. Get the user's consent to regenerate CLAUDE.md.
2. In the "Test Fixtures" section of
   `docs/development/index.md`, change the tag to
   `//go:build unix || windows || plan9`. Say that a
   test running an `sh` script, an `sh` argv, or a
   recipe calls `skipWithoutPOSIXTools` (or
   `skipOnPlan9`), because plan9 has only `rc`.
3. Run `mdsmith fix CLAUDE.md AGENTS.md
   .github/copilot-instructions.md` to regenerate the
   included copies.

## Acceptance Criteria

- [ ] `docs/development/index.md` and its three included
      copies name `unix || windows || plan9` and the
      plan9 skip helper.
- [ ] `mdsmith check .` passes.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
