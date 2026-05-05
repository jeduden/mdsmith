---
id: 130
title: Distribute mdsmith binaries via npm, PyPI, asdf, and mise
status: "🔲"
summary: >-
  Publish the prebuilt mdsmith binaries already produced
  by the release workflow through npm, PyPI (consumed by
  pip and uv), asdf, and mise on every git tag. Derive
  every published manifest's version from the tag instead
  of a hard-coded literal.
model: opus
---
# Distribute mdsmith binaries via npm, PyPI, asdf, and mise

## Goal

Each `v*` tag should ship the existing release binaries
through four extra channels: npm, PyPI, asdf, and mise.
Every published manifest should carry the tag version,
not a hand-edited string. Users pick the package manager
their stack already uses; mdsmith reports the same
version regardless of the channel.

## Background

[release.yml](../.github/workflows/release.yml) already
builds `mdsmith-<goos>-<goarch>[.exe]` for the cross
product of linux, darwin, and windows with amd64 and
arm64. It also packages the VS Code extension as a
`.vsix` and uploads everything plus a `checksums.txt` to
a GitHub release. The Go binary embeds the tag via
`-ldflags="-X main.version=${VERSION}"` (see
[main.go](../cmd/mdsmith/main.go)).

Two gaps remain.

First, the `.vsix` job strips the leading `v` from the
tag for the filename, but
[editors/vscode/package.json](../editors/vscode/package.json)
still ships a hard-coded `"version": "0.1.2"`. The
`vsce package --out` flag controls only the filename.
The manifest inside the package keeps the literal.

Second, there is no npm, PyPI, asdf, or mise channel.
This plan adds all four and fixes the version drift in
the same pass so the same machinery covers every
channel.

## Distribution strategy per manager

### npm

Use the `optionalDependencies` per-platform pattern.
This is what esbuild, biome, swc, and turbo settled on.

The user installs one root package: `mdsmith`. It
lists `optionalDependencies` for one subpackage per
platform. The names are `@mdsmith/linux-x64`,
`@mdsmith/linux-arm64`, `@mdsmith/darwin-x64`,
`@mdsmith/darwin-arm64`, and `@mdsmith/win32-x64`.
Each subpackage sets `os` and `cpu`. npm installs
only the matching one and skips the rest.

Each subpackage carries the prebuilt binary at
`bin/mdsmith` (or `mdsmith.exe`) and a tiny
`package.json` that declares `bin`. The root package's
`bin/mdsmith.js` shim resolves the platform package via
`require.resolve` and `execFileSync`s its binary. No
`postinstall` hook downloads from GitHub at install
time. That keeps mdsmith installable in offline or
air-gapped CI and keeps it clear of supply-chain
policies that ban network calls during install.

Sources live under `npm/mdsmith/` (the root) and
`npm/platforms/<goos>-<goarch>/` (one each). The
platform subpackages are generated at release time
from a template.

### PyPI

Use the per-platform wheel with bundled binary
pattern. ruff and uv ship this way. `cibuildwheel` is
overkill since mdsmith does not compile any C — we
only need to attach a prebuilt binary to a wheel with
the right platform tag.

Build one wheel per platform tag: linux x86_64,
linux aarch64, macOS x86_64, macOS arm64, win amd64.
Each wheel ships `mdsmith/_bin/mdsmith[.exe]` and a
small `mdsmith/__main__.py` that `os.execv`s the
bundled binary. The wheel exposes a console script
entry point also named `mdsmith`.

Add an sdist `mdsmith-<ver>.tar.gz` with a
`pyproject.toml` build that fails fast with a clear
message when no wheel exists for the user's platform.
That keeps `pip install mdsmith` from silently doing
nothing on an unsupported arch.

This setup works under `pip install mdsmith`,
`uv pip install mdsmith`, `pipx install mdsmith`,
`uvx mdsmith`, and `python -m mdsmith`. Sources live
under `python/`.

### asdf

Publish a separate repo `jeduden/asdf-mdsmith` with
the standard asdf plugin layout:

- `bin/list-all` calls `git ls-remote --tags` on the
  mdsmith repo. No GitHub token is required and it
  works behind firewalls that allow HTTPS git.
- `bin/download` `curl -fL`s the matching
  `mdsmith-<goos>-<goarch>` from the release.
- `bin/install` verifies against `checksums.txt` and
  places the binary as `bin/mdsmith` in the install
  path.
- `bin/list-bin-paths` prints `bin`.

After one release cycle of self-hosted use, file a PR
to `asdf-vm/asdf-plugins` so `asdf plugin add mdsmith`
resolves without an explicit URL.

### mise

Two paths, picked in order.

The preferred path is to add an entry to the mise
registry that points at the existing GitHub releases
through the `ubi` backend. mise's `ubi` backend reads
GitHub release assets directly given the asset
naming we already use, so `mise use mdsmith@latest`
works without us shipping any plugin code. File a PR
to `mise-plugins/registry`.

The fallback path is the asdf plugin above. mise
consumes asdf plugins natively, so
`mise use asdf:jeduden/asdf-mdsmith` keeps working
even before the registry PR lands.

The new `docs/guides/install.md` (added by task 9
below) documents the registry path as primary. The
asdf path is the documented fallback.

### Out of scope

Homebrew tap, Scoop bucket, AUR, Chocolatey, Nix
flake, GoReleaser migration, and a Docker image are
all sensible follow-ups. None block the four channels
above. Each can be added in a later plan once the
versioning machinery is in place.

## Versioning from the git tag

Today the only manifest with a hard-coded version is
[editors/vscode/package.json](../editors/vscode/package.json).
The npm root, npm platform subpackages, and the Python
wheel all need the same treatment.

The approach: never commit a real version. Pin every
manifest at `"version": "0.0.0-dev"`. Rewrite each
manifest in CI from `${GITHUB_REF_NAME#v}` before
publishing.

Concretely:

- Add a `scripts/set-version.sh <ver>` helper. It takes
  the cleaned tag (no leading `v`) and rewrites
  `editors/vscode/package.json`, `npm/mdsmith/package.json`,
  each `npm/platforms/*/package.json`, and
  `python/pyproject.toml`. It also bumps the
  `optionalDependencies` pin of each platform package
  in the root so they match.
- Wire the helper into
  [release.yml](../.github/workflows/release.yml) as a
  step that runs before each `package` or `publish`
  step.
- Add a CI guard in
  [ci.yml](../.github/workflows/ci.yml) that fails on
  non-tag builds when any tracked manifest has a
  version other than `0.0.0-dev`. That blocks
  accidental hand edits from reaching `main`.
- Keep surfacing the tag to the Go binary the way
  [release.yml](../.github/workflows/release.yml)
  already does (`-X main.version=${VERSION}`). The
  npm shim and the wheel both invoke the embedded
  binary, so `mdsmith version` reports the tag on
  every channel.

## Tasks

1. Add `scripts/set-version.sh`. Add a unit test (a
   Bash test or a small Go test fixture) that asserts
   each tracked manifest is rewritten correctly and
   the script is idempotent.
2. Add a `version-guard` step to
   [ci.yml](../.github/workflows/ci.yml) that fails
   on non-tag builds when any tracked manifest has a
   non-`0.0.0-dev` version.
3. Set
   [editors/vscode/package.json](../editors/vscode/package.json)
   `version` to `0.0.0-dev`. Run `set-version.sh` in
   the `vscode` job of
   [release.yml](../.github/workflows/release.yml)
   before `vsce package`. Verify by inspecting the
   generated `.vsix` `package.json` in the job log.
4. Scaffold `npm/mdsmith/` with `package.json` and
   `bin/mdsmith.js`. Add a Bun unit test (consistent
   with the VS Code extension) that mocks
   `os.platform()` and `os.arch()` and verifies the
   shim resolves to the expected platform package
   path. Mirror the lint and format settings used by
   the VS Code extension.
5. Add `scripts/build-npm-platforms.sh`. Given the
   downloaded GitHub release artifacts, it emits one
   directory per platform with the binary in `bin/`
   and a generated `package.json`. Add a new `npm`
   job in [release.yml](../.github/workflows/release.yml)
   that depends on `build`, downloads the artifacts,
   runs the generator, and `npm publish --access public`s
   each subpackage. The root publishes last so users
   never see a missing optional dependency.
6. Add `python/pyproject.toml`,
   `python/mdsmith/__init__.py`, and
   `python/mdsmith/__main__.py`. Add
   `scripts/build-wheels.sh`. Wire a `pypi` job in
   [release.yml](../.github/workflows/release.yml)
   that downloads the binary artifacts, stages them
   under `python/mdsmith/_bin/`, builds one wheel per
   platform tag with `python -m build`, and uploads
   via `pypa/gh-action-pypi-publish` using PyPI
   trusted publishing (OIDC). No long-lived token.
7. Create the `jeduden/asdf-mdsmith` repo with
   `bin/list-all`, `bin/download`, `bin/install`, and
   `bin/list-bin-paths`. Add a CI workflow that runs
   `asdf install mdsmith latest` against the most
   recent release and asserts
   `mdsmith version` matches. Open a PR to
   `asdf-vm/asdf-plugins` after one successful
   release cycle.
8. Submit a PR to `mise-plugins/registry` adding
   mdsmith via the `ubi` backend pointing at
   `jeduden/mdsmith` releases.
9. Add `docs/guides/install.md` covering
   `npm i -g mdsmith`, `npx mdsmith`,
   `pip install mdsmith`, `uvx mdsmith`,
   `mise use mdsmith@latest`, `asdf install mdsmith`,
   and the existing direct-download flow. Link it
   from the README and the catalog table in
   [CLAUDE.md](../CLAUDE.md).
10. Add a post-release smoke test job that runs in
    one clean container per channel
    (`node:lts-alpine`, `python:3.12-slim`,
    a mise base image) and asserts
    `mdsmith version` prints the expected tag.

## Acceptance Criteria

- [ ] Pushing a `vX.Y.Z` tag publishes
      `mdsmith@X.Y.Z` and the five platform
      subpackages on npm.
- [ ] The same tag publishes `mdsmith==X.Y.Z` wheels
      for the five supported platform tags on PyPI.
- [ ] The same tag still produces the existing
      GitHub release assets and `.vsix`.
- [ ] `npm i -g mdsmith && mdsmith version` prints
      `mdsmith vX.Y.Z` on linux-x64, linux-arm64,
      darwin-x64, darwin-arm64, and win32-x64.
- [ ] `pip install mdsmith==X.Y.Z && mdsmith version`
      and `uvx mdsmith@X.Y.Z version` print
      `mdsmith vX.Y.Z` on the same five platforms.
- [ ] `mise use mdsmith@X.Y.Z && mdsmith version`
      prints `mdsmith vX.Y.Z`.
- [ ] `asdf plugin add mdsmith` followed by
      `asdf install mdsmith X.Y.Z` prints
      `mdsmith vX.Y.Z`.
- [ ] The `.vsix` from the `vscode` job has its
      internal `package.json` `version` equal to
      `X.Y.Z`.
- [ ] CI on `main` fails when any tracked manifest
      has a version other than `0.0.0-dev`.
- [ ] The new `docs/guides/install.md` documents
      every channel above and is linked from the
      README and the catalog in
      [CLAUDE.md](../CLAUDE.md).
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues.
