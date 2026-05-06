---
id: 130
title: >-
  Distribute mdsmith binaries via npm, PyPI, asdf,
  mise, and the VS Code marketplaces
status: "🔳"
summary: >-
  Publish the prebuilt mdsmith binaries already produced
  by the release workflow through npm, PyPI (consumed by
  pip and uv), asdf, and mise on every git tag, and
  publish the VS Code extension to the Visual Studio
  Marketplace and Open VSX. Derive every published
  manifest's version from the tag instead of a
  hard-coded literal.
model: opus
---
# Distribute mdsmith via npm, PyPI, asdf, mise, and VS Code marketplaces

## Goal

Each `v*` tag should ship the existing binaries
through five extra channels: npm, PyPI, asdf, mise,
and the two VS Code extension registries. Every
published manifest carries the tag version (not a
hand-edited string), so `mdsmith version` reports
the same value on every channel.

## Background

[release.yml](../.github/workflows/release.yml)
already builds `mdsmith-<goos>-<goarch>[.exe]` for
linux and darwin on amd64 and arm64, plus windows on
amd64. It also packages the VS Code extension as a
`.vsix` and uploads everything plus a `checksums.txt`
to a GitHub release. The Go binary embeds the tag via
`-ldflags="-X main.version=${VERSION}"` (see
[main.go](../cmd/mdsmith/main.go)).

Three gaps remain. First,
[editors/vscode/package.json](../editors/vscode/package.json)
ships a hard-coded `"version": "0.1.2"`; the
`vsce package --out` flag only controls the filename.
Second, there is no npm, PyPI, asdf, or mise channel.

Third, the `.vsix` is only attached to the GitHub
release. It is not on the Visual Studio Marketplace,
so VS Code's "Install" button cannot find it. It is
not on Open VSX either, so VSCodium, Cursor, Theia,
and Gitpod users have no source. This plan closes
all three gaps in one pass.

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

### asdf

Publish a separate repo `jeduden/asdf-mdsmith` with
the standard plugin layout:

- `bin/list-all` calls `git ls-remote --tags` on the
  mdsmith repo, then strips `refs/tags/`, drops the
  `^{}` deref entries, and removes the leading `v`
  so the output is plain `X.Y.Z` as asdf expects. No
  GitHub token required; works through HTTPS git.
- `bin/download` `curl -fL`s the matching release
  asset.
- `bin/install` verifies it against `checksums.txt`
  and places the binary as `bin/mdsmith`.
- `bin/list-bin-paths` prints `bin`.

After one release cycle, file a PR to
`asdf-vm/asdf-plugins` so `asdf plugin add mdsmith`
resolves without an explicit URL.

### mise

Preferred path: add an entry to `mise-plugins/registry`
using the `ubi` backend. mise's `ubi` reads GitHub
release assets directly given our naming, so
`mise use mdsmith@latest` works without us shipping
plugin code. Fallback: mise consumes asdf plugins
natively, so `mise use asdf:jeduden/asdf-mdsmith`
keeps working even before the registry PR lands.
Document the registry path as primary in
`docs/guides/install.md` (task 9).

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
  run `set-version.sh` before `vsce package`, then
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
- [ ] **Follow-up (separate repo):** create
  `jeduden/asdf-mdsmith` with `bin/list-all`,
  `bin/download`, `bin/install`, and
  `bin/list-bin-paths`; add a CI workflow that runs
  `asdf install mdsmith latest` against the most
  recent release; open a PR to `asdf-vm/asdf-plugins`
  after one successful release cycle.
- [ ] **Follow-up (external PR):** submit a PR to
  `mise-plugins/registry` adding mdsmith via the
  `ubi` backend pointing at `jeduden/mdsmith`
  releases.
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

- [ ] Pushing a `vX.Y.Z` tag publishes
      `@mdsmith/cli@X.Y.Z` and the five
      `@mdsmith/<platform>` subpackages on npm.
- [ ] The same tag publishes `mdsmith==X.Y.Z` wheels
      for the five supported platform tags on PyPI.
- [ ] The same tag still produces the existing GitHub
      release assets and `.vsix`.
- [ ] `npm i -g @mdsmith/cli && mdsmith version`
      prints `mdsmith vX.Y.Z` on all five supported
      platforms.
- [ ] `pip install mdsmith==X.Y.Z && mdsmith version`
      and `uvx mdsmith@X.Y.Z version` print
      `mdsmith vX.Y.Z` on the same five platforms.
- [ ] `mise use mdsmith@X.Y.Z && mdsmith version`
      prints `mdsmith vX.Y.Z`.
- [ ] `asdf plugin add mdsmith` then
      `asdf install mdsmith X.Y.Z` prints
      `mdsmith vX.Y.Z`.
- [ ] The `.vsix` from the `vscode` job has its
      internal `package.json` `version` equal to
      `X.Y.Z`.
- [ ] After the tag job finishes, `jeduden.mdsmith`
      `X.Y.Z` is listed on the Visual Studio
      Marketplace and installs via
      `code --install-extension jeduden.mdsmith`.
- [ ] The same version is listed on Open VSX and
      installs in VSCodium via
      `codium --install-extension jeduden.mdsmith`.
- [ ] The Marketplace, Open VSX, and GitHub release
      `.vsix` have identical SHA-256 sums.
- [ ] CI on `main` fails when any tracked manifest
      has a version other than `0.0.0-dev`.
- [ ] The new `docs/guides/install.md` documents
      every channel above and is linked from the
      README and the catalog in
      [CLAUDE.md](../CLAUDE.md).
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues.
