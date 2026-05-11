---
id: 130
title: >-
  Distribute mdsmith binaries via npm, PyPI, and the
  VS Code marketplaces
status: "✅"
summary: >-
  Publish the prebuilt mdsmith binaries already produced
  by the release workflow through npm and PyPI (consumed
  by pip and uv) on every git tag, and publish the VS
  Code extension to the Visual Studio Marketplace and
  Open VSX. Derive every published manifest's version
  from the tag instead of a hard-coded literal. The
  asdf and mise channels are deferred to plan 145
  because both registry submissions live outside this
  repo.
model: opus
---
# Distribute mdsmith via npm, PyPI, and VS Code marketplaces

## Goal

Each `v*` tag should ship the existing binaries
through three extra channels: npm, PyPI, and the two
VS Code extension registries. Every published
manifest carries the tag version (not a hand-edited
string), so `mdsmith version` reports the same value
on every channel. The asdf and mise channels live in
[plan/145](145_asdf-mise-registry-submissions.md)
since their registry submissions sit outside this
repo.

## Background

This section describes the pre-implementation state.

[release.yml](../.github/workflows/release.yml)
already built `mdsmith-<goos>-<goarch>[.exe]` for
linux and darwin on amd64 and arm64, plus windows on
amd64. It also packaged the VS Code extension as a
`.vsix` and uploaded everything plus a `checksums.txt`
to a GitHub release. The Go binary embedded the tag via
`-ldflags="-X main.version=${VERSION}"` (see
[main.go](../cmd/mdsmith/main.go)).

Three gaps remained. First,
[editors/vscode/package.json](../editors/vscode/package.json)
shipped a hard-coded `"version": "0.1.2"`; the
`vsce package --out` flag only controlled the filename.
Second, there was no npm, PyPI, asdf, or mise channel.

Third, the `.vsix` was only attached to the GitHub
release. It was not on the Visual Studio Marketplace,
so VS Code's "Install" button could not find it. It was
not on Open VSX either, so VSCodium, Cursor, Theia,
and Gitpod users had no source. This plan closed the
npm, PyPI, and VS Code marketplace gaps; asdf and mise
moved to [plan/145](145_asdf-mise-registry-submissions.md).

## Distribution strategy per manager

### npm

Use the `optionalDependencies` per-platform pattern
(esbuild, biome, swc, and turbo all ship this way).

The user installs one root package, `@mdsmith/cli`
(unscoped `mdsmith` is taken). It lists
`optionalDependencies` per platform:

- `@mdsmith/linux-x64`
- `@mdsmith/linux-arm64`
- `@mdsmith/darwin-x64`
- `@mdsmith/darwin-arm64`
- `@mdsmith/win32-x64`

Each subpackage sets `os` and `cpu`. npm installs
only the one that matches.

Each subpackage carries the prebuilt binary at
`bin/mdsmith[.exe]`. The root's `bin/mdsmith.js` shim
resolves the platform package via `require.resolve`
and execs its binary. No `postinstall` hook runs at
install time, so mdsmith stays installable in offline
or air-gapped CI and clear of supply-chain policies
that ban network calls during install.

The root lives at `npm/mdsmith/`. Each subpackage
lives at `npm/platforms/<node-platform>-<node-arch>/`.
The release script renames Go assets to match:

| Release asset               | npm package             |
|-----------------------------|-------------------------|
| `mdsmith-linux-amd64`       | `@mdsmith/linux-x64`    |
| `mdsmith-linux-arm64`       | `@mdsmith/linux-arm64`  |
| `mdsmith-darwin-amd64`      | `@mdsmith/darwin-x64`   |
| `mdsmith-darwin-arm64`      | `@mdsmith/darwin-arm64` |
| `mdsmith-windows-amd64.exe` | `@mdsmith/win32-x64`    |

### PyPI

Use the per-platform wheel with bundled binary
pattern (ruff and uv ship this way). One wheel per
platform tag (linux x86_64, linux aarch64, macOS
x86_64, macOS arm64, win amd64) ships
`mdsmith/_bin/mdsmith[.exe]` and a console-script
entry point `mdsmith` that `os.execv`s the bundled
binary. An sdist's build fails fast with a clear
message on unsupported platforms so `pip install`
does not silently do nothing. Works under `pip`,
`uv pip`, `pipx`, `uvx`, and `python -m mdsmith`.
Sources live under `python/`.

### asdf and mise

The asdf-plugin repo and the mise registry entry
both live outside this repo, so they are tracked in
[plan/145](145_asdf-mise-registry-submissions.md).
Until those land, users on mise can still pull the
binary directly via the `ubi:` backend
(`mise use ubi:jeduden/mdsmith@latest`), which the
release smoke-test exercises today.

### VS Code Marketplace and Open VSX

Publish the same `.vsix` to two registries. Stock VS
Code queries the Visual Studio Marketplace
(`marketplace.visualstudio.com`); VSCodium, Cursor,
Theia, and Gitpod query Open VSX (`open-vsx.org`).
The `.vsix` is identical, only the upload tool
differs. Both uploads run from the existing `vscode`
job in
[release.yml](../.github/workflows/release.yml) after
`vsce package`. Use `@vscode/vsce` for Marketplace
and `ovsx` for Open VSX, both with `--packagePath`
pointing at the exact `.vsix` the GitHub release
also attaches, so all three artifacts stay
byte-identical.

Auth uses two GitHub Actions secrets:

- `VSCE_PAT` — an Azure DevOps PAT scoped to
  "Marketplace > Manage" for the `jeduden` publisher.
  No OIDC option exists for the Marketplace today.
- `OVSX_PAT` — an Open VSX publisher token, created
  after claiming the namespace on `open-vsx.org`.

Azure caps PATs at one year; rotate Open VSX annually
too and record the rotation date in
[CLAUDE.md](../CLAUDE.md). If publishing to either
registry fails, the GitHub release `.vsix` remains
the documented fallback.

### Out of scope

Homebrew, Scoop, AUR, Chocolatey, Nix, GoReleaser,
and Docker are follow-ups; none block this plan.

## Versioning from the git tag

Today the only manifest with a hard-coded version is
[editors/vscode/package.json](../editors/vscode/package.json).
The npm root, npm platform subpackages, and the
Python wheel all need the same treatment.

Approach: never commit a real version. Pin every
manifest at `"version": "0.0.0-dev"`. The internal
`cmd/mdsmith-release stamp <ver>` CLI takes the
cleaned tag (no leading `v`) and rewrites every
tracked manifest:

- [editors/vscode/package.json](../editors/vscode/package.json)
- `npm/mdsmith/package.json`, including the
  `optionalDependencies` pins of each platform
  package
- each `npm/platforms/*/package.json`
- `python/pyproject.toml`

[release.yml](../.github/workflows/release.yml) runs
it before each `package` or `publish` step. A
`version-guard` job in
[ci.yml](../.github/workflows/ci.yml) fails non-tag
builds when any manifest deviates from `0.0.0-dev`.
That blocks hand edits from reaching `main`.

The Go binary keeps embedding the tag via
`-X main.version=${VERSION}`. The npm shim and the
wheel exec that binary, so `mdsmith version` matches
the tag on every channel.

## Tasks

- [x] Add `cmd/mdsmith-release/` (subcommands stamp,
  check, build-npm, build-wheels) and Go tests under
  `internal/release/`. stamp is idempotent.
- [x] Add a `version-guard` step to
  [ci.yml](../.github/workflows/ci.yml) that fails
  on non-tag builds when any tracked manifest has a
  non-`0.0.0-dev` version.
- [x] Set
  [editors/vscode/package.json](../editors/vscode/package.json)
  `version` to `0.0.0-dev`. In the `vscode` job of
  [release.yml](../.github/workflows/release.yml),
  run `mdsmith-release stamp` before `vsce package`,
  then
  add two publish steps that reuse the exact `.vsix`
  the job produced: `bunx --bun @vscode/vsce publish
  --packagePath <vsix> --pat $VSCE_PAT` (Marketplace)
  and `bunx --bun ovsx publish --packagePath <vsix>
  --pat $OVSX_PAT` (Open VSX). Before the first tag,
  claim the `jeduden` namespace on Open VSX, mint a
  Marketplace PAT scoped to "Marketplace > Manage",
  and store both as `VSCE_PAT` and `OVSX_PAT`
  repository secrets.
- [x] Scaffold `npm/mdsmith/` with `package.json` and
  `bin/mdsmith.js`. Add a Bun unit test that mocks
  `os.platform()` and `os.arch()` and verifies the
  shim resolves to the expected platform package
  path. Mirror the lint/format setup used by the VS
  Code extension.
- [x] Add `mdsmith-release build-npm` that, given
  the downloaded GitHub release artifacts, emits
  one directory per platform with the binary in
  `bin/` and a generated `package.json`. Add a new
  `npm` job in
  [release.yml](../.github/workflows/release.yml)
  that depends on `build`, downloads artifacts, runs
  the generator, and `npm publish --access public`s
  each subpackage. Root publishes last so users
  never see a missing optional dependency.
- [x] Add `python/pyproject.toml`,
  `python/mdsmith/__init__.py`, and
  `python/mdsmith/__main__.py`. Add
  `mdsmith-release build-wheels`. Wire a `pypi` job in
  [release.yml](../.github/workflows/release.yml)
  that stages the binary artifacts under
  `python/mdsmith/_bin/`, builds one wheel per
  platform tag with `python -m build`, and uploads
  via `pypa/gh-action-pypi-publish` using PyPI
  trusted publishing (OIDC). No long-lived token.
- [x] Add `docs/guides/install.md` covering
  `npm i -g @mdsmith/cli`, `npx @mdsmith/cli`,
  `pip install mdsmith`, `uvx mdsmith`,
  `mise use mdsmith@latest`, `asdf install mdsmith`,
  the Marketplace and Open VSX install paths for
  the VS Code extension, and the existing
  direct-download flow. Link it from the README and
  the catalog in [CLAUDE.md](../CLAUDE.md).
- [x] Add a post-release smoke-test job that runs
  one clean container per channel and asserts
  `mdsmith version` prints the expected tag.

## Acceptance Criteria

- [x] Pushing a `vX.Y.Z` tag publishes
      `@mdsmith/cli@X.Y.Z` and the five
      `@mdsmith/<platform>` subpackages on npm.
- [x] The same tag publishes `mdsmith==X.Y.Z` wheels
      for the five supported platform tags on PyPI.
- [x] The same tag still produces the existing GitHub
      release assets and `.vsix`.
- [x] `npm i -g @mdsmith/cli && mdsmith version`
      prints `mdsmith vX.Y.Z` on all five supported
      platforms.
- [x] `pip install mdsmith==X.Y.Z && mdsmith version`
      and `uvx mdsmith@X.Y.Z version` print
      `mdsmith vX.Y.Z` on the same five platforms.
- [x] The `.vsix` from the `vscode` job has its
      internal `package.json` `version` equal to
      `X.Y.Z`.
- [x] After the tag job finishes, `jeduden.mdsmith`
      `X.Y.Z` is listed on the Visual Studio
      Marketplace and installs via
      `code --install-extension jeduden.mdsmith`.
- [x] The same version is listed on Open VSX and
      installs in VSCodium via
      `codium --install-extension jeduden.mdsmith`.
- [x] The Marketplace, Open VSX, and GitHub release
      `.vsix` have identical SHA-256 sums.
- [x] CI on `main` fails when any tracked manifest
      has a version other than `0.0.0-dev`.
- [x] The new `docs/guides/install.md` documents
      every channel above and is linked from the
      README and the catalog in
      [CLAUDE.md](../CLAUDE.md).
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues.

Deferred to [plan/145](145_asdf-mise-registry-submissions.md)
(out-of-repo registry submissions):

- `mise use mdsmith@X.Y.Z && mdsmith version` prints
  `mdsmith vX.Y.Z`.
- `asdf plugin add mdsmith` then
  `asdf install mdsmith X.Y.Z` prints
  `mdsmith vX.Y.Z`.
