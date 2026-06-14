---
id: 145
title: >-
  Publish mdsmith via asdf and mise registry
  submissions
status: "🔳"
summary: >-
  Land the asdf-plugin repo (jeduden/asdf-mdsmith) and
  the jdx/mise registry entry that the multi-channel
  release pipeline already documents but cannot trigger
  from this repo. Spun out of plan/130 because both
  tasks ship outside this repo.
model: opus
---
# Publish mdsmith via asdf and mise

## Goal

The release pipeline already attaches per-platform
binaries to every `vX.Y.Z` GitHub release. The
smoke-test job in
[release.yml](../.github/workflows/release.yml)
verifies the binary works through
`mise use -g ubi:jeduden/mdsmith@VER`. The gap left
is that `asdf install mdsmith` and the bare
`mise use mdsmith@latest` form do not yet resolve.
Neither registry knows about us yet.

## Background

`mise` reads our GitHub releases directly. Its
`github` backend already resolves
`mise use github:jeduden/mdsmith@VER`. The `asdf:`
and `go:` backends work too. A registry entry only
adds the shorter, prefix-less `mdsmith@VER` form.

`asdf` is different. It needs a plugin repo that
knows how to list versions and fetch the binary.
Once that plugin repo exists, the
`asdf-vm/asdf-plugins` index lets users skip the URL
on `asdf plugin add mdsmith`.

## Tasks

1. Create the `jeduden/asdf-mdsmith` repo with the
   standard plugin layout:

  - `bin/list-all` calls `git ls-remote --tags` on
     this repo, strips `refs/tags/`, drops the `^{}`
     deref entries, and removes the leading `v` so
     the output is plain `X.Y.Z` as asdf expects. No
     GitHub token required; works through HTTPS git.
  - `bin/download` `curl -fL`s the matching release
     asset.
  - `bin/install` verifies it against `checksums.txt`
     and places the binary as `bin/mdsmith`.
  - `bin/list-bin-paths` prints `bin`.

2. Add a CI workflow on `jeduden/asdf-mdsmith`.
   Run `asdf install mdsmith latest` and assert
   that `mdsmith version` matches the resolved tag.
3. After one successful release cycle, file a PR to
   [`asdf-vm/asdf-plugins`](https://github.com/asdf-vm/asdf-plugins).
   The entry lets `asdf plugin add mdsmith` resolve
   without an explicit URL. See the Blockers section
   before filing — adoption is the current gate.
4. File a PR to mise's curated registry at
   [`jdx/mise`](https://github.com/jdx/mise).
   Each tool gets one TOML file under `registry/`
   (the former `mise-plugins/registry` is archived).
   Add `registry/mdsmith.toml` with a
   `[tools.mdsmith]` section on the
   `github:jeduden/mdsmith` backend and a `test`
   field (`ubi:` is rejected; `aqua:` only works if
   mdsmith is already in the Aqua registry; use
   `github:`). The PR body must make a popularity
   and maintenance case. On merge,
   `mise use mdsmith@latest` resolves without a
   backend prefix.

   **Filed and rejected.** See the Blockers section
   for details and the re-submission trigger.
5. Update
   [docs/guides/install.md](../docs/guides/install.md)
   to drop the "pending follow-up" badge from the
   asdf and short-mise sections once each registry PR
   merges.
6. [x] Update the release-workflow smoke-test matrix
   in [release.yml](../.github/workflows/release.yml)
   to exercise `asdf install mdsmith X.Y.Z` and
   `mise use mdsmith@X.Y.Z` alongside `ubi:`.
   The `asdf` channel must pass (users install day
   one via the explicit plugin URL).
   The `mise-registry` channel is best-effort;
   it warns and exits 0 until the registry PR merges.

## Acceptance Criteria

- [x] `jeduden/asdf-mdsmith` exists with the four
      `bin/` scripts and a green CI workflow.
- [ ] `asdf plugin add mdsmith` resolves without an
      explicit URL after the asdf-plugins PR merges.
- [x] `asdf install mdsmith X.Y.Z` then
      `mdsmith version` prints `mdsmith vX.Y.Z`.
- [ ] `mise use mdsmith@X.Y.Z` (no backend prefix)
      resolves after the `jdx/mise` registry PR
      merges, and `mdsmith version` prints
      `mdsmith vX.Y.Z`.
- [ ] [docs/guides/install.md](../docs/guides/install.md)
      no longer flags asdf or short-form mise as
      pending follow-ups.
- [x] The smoke-test matrix in
      [release.yml](../.github/workflows/release.yml)
      runs `asdf install mdsmith` and bare
      `mise use mdsmith@VER` channels green on a tag.
      The `asdf` channel is required-green; the bare
      `mise-registry` channel is best-effort until
      the jdx/mise registry PR merges.

## Blockers

The remaining work is gated on two curated upstream
registries, and neither will accept mdsmith at its
current adoption level:

- **mise (Task 4):**
  [jdx/mise#10320](https://github.com/jdx/mise/pull/10320)
  was filed with the correct `registry/mdsmith.toml`
  and closed unmerged on 2026-06-11 — 7 stars is below
  the registry's bar for new tools. The bare
  `mise use mdsmith@VER` form cannot resolve until a
  re-submission is accepted.
- **asdf (Task 3):** the one-successful-release-cycle
  precondition is now met — the pipeline has shipped
  through `v0.47.0` (2026-06-14) — so the only remaining
  gate is adoption. No PR to
  [`asdf-vm/asdf-plugins`](https://github.com/asdf-vm/asdf-plugins)
  has been filed; that index has comparable curation
  expectations, so a submission now would meet
  the same popularity objection that closed the mise
  PR. The `plugins/mdsmith` index entry does not exist.
- **Docs (Task 5):** because neither registry PR has
  merged,
  [docs/guides/install.md](../docs/guides/install.md)
  must keep flagging the bare `asdf plugin add mdsmith`
  and bare `mise use mdsmith@VER` forms as not-yet-
  resolving. The current "needs a registry entry" notes
  are accurate and stay until a PR lands.

In-repo work is done: the `jeduden/asdf-mdsmith`
plugin, its CI, and the release.yml smoke-test matrix.
The plan stays open until adoption clears the
registries' bar.
