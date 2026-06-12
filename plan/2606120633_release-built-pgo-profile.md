---
id: 2606120633
title: Wire PGO into the release build without a committed profile
status: "🔲"
summary: >-
  A committed cmd/mdsmith/default.pgo burdened every
  merge with a repo-specific binary artifact and was
  removed. Generate the profile inside the release
  workflow instead, so published binaries are
  profile-guided while the repository stays
  artifact-free.
model: ""
depends-on: []
---
# Wire PGO into the release build without a committed profile

## Goal

Make the released `mdsmith` binaries profile-guided without
committing a `default.pgo` file to the repository.

## Context

PR 587 briefly committed `cmd/mdsmith/default.pgo` so every
`go build ./cmd/mdsmith` picked it up automatically. That
created a merge problem. The file is a binary pprof profile.
Two branches that both refresh it always conflict, and no
byte-level resolution is meaningful.

A take-current merge driver was tried and reverted. The
managed `.gitattributes` block and `mdsmith merge-driver
install` are product features. They run in users'
repositories. They must never carry entries that only make
sense in mdsmith's own repository.

The profile is now removed and gitignored. Builds are
unoptimized but correct; PGO measured ~0-2% on this workload
(see [the session notes](../docs/research/perf-parity-session.md)),
so nothing user-visible regressed.

## Design

Generate the profile inside the release pipeline instead of
committing it:

1. A `mdsmith-release pgo` subcommand builds the binary,
   runs `mdsmith check` over the two benchmark corpora in
   both configurations through the `MDSMITH_CPUPROFILE`
   hook, and merges the runs into a `default.pgo` in a
   workdir (the same corpus plumbing `mdsmith-release
   bench` already owns).
2. The release workflow runs that subcommand before the
   build matrix and rebuilds with the generated profile
   (`go build` finds it in `cmd/mdsmith/`; the file stays
   untracked).
3. The benchmark harness optionally does the same, so the
   published numbers and the released binaries share one
   build configuration. Decide and document either way.

Two alternatives were considered and dropped. Committing the
profile was reverted (see Context). A repo-local
`.git/info/attributes` entry with a local driver leaves
clones inconsistent and still ships a committed artifact.

## Tasks

1. Add `mdsmith-release pgo <workdir>` reusing the bench
   corpus builders; unit-test the profile-merge step.
2. Call it from `release.yml` before the build matrix and
   pass the workdir profile into the builds.
3. Decide whether `mdsmith-release bench` builds with the
   generated profile; document the choice on the benchmark
   page either way.
4. Update [the PGO page](../docs/development/pgo-profile.md)
   to describe the release-generated flow.

## Acceptance Criteria

- [ ] No `.pgo` file is tracked anywhere in the repository
- [ ] Release binaries are built with a freshly generated
      profile and the workflow logs say so
- [ ] `mdsmith merge-driver install` output is byte-identical
      in user repositories to its pre-PR-587 output
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
