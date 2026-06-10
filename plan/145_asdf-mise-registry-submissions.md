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

2. Add a CI workflow on `jeduden/asdf-mdsmith` that
   runs `asdf install mdsmith latest` against the
   most recent release and asserts `mdsmith version`
   matches the resolved tag.
3. After one successful release cycle, file a PR to
   [`asdf-vm/asdf-plugins`](https://github.com/asdf-vm/asdf-plugins)
   adding mdsmith so `asdf plugin add mdsmith`
   resolves without an explicit URL.
4. File a PR to mise's curated registry: the
   `registry/` directory in
   [`jdx/mise`](https://github.com/jdx/mise), one
   TOML file per tool. (The former root
   `registry.toml` was split into per-tool files;
   the separate `mise-plugins/registry` repo was
   archived in Oct 2024.) Add
   `registry/mdsmith.toml` with a `[tools.mdsmith]`
   entry on the `github:jeduden/mdsmith` backend
   (`ubi:` is rejected for new entries; `aqua:` is
   preferred only for tools already in the aqua
   registry) and the required `test` field. The PR
   body must make a popularity/maintenance case,
   since the registry is curated. Once merged, the
   prefix-less `mise use mdsmith@latest` form starts
   resolving on user CLIs without any code change in
   this repo.
5. Update
   [docs/guides/install.md](../docs/guides/install.md)
   to drop the "pending follow-up" badge from the
   asdf and short-mise sections once each registry PR
   merges.
6. Update the release-workflow smoke-test matrix in
   [release.yml](../.github/workflows/release.yml)
   to also exercise `asdf install mdsmith X.Y.Z` and
   the bare `mise use mdsmith@X.Y.Z` form, in
   addition to the `ubi:` form already covered.

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
- [ ] The smoke-test matrix in
      [release.yml](../.github/workflows/release.yml)
      runs `asdf install mdsmith` and bare
      `mise use mdsmith@VER` channels green on a tag.
