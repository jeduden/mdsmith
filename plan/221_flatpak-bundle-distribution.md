---
id: 221
title: >-
  Ship mdsmith as a self-hosted Flatpak bundle
status: "🔳"
summary: >-
  Build a single-file x86_64 `.flatpak` bundle in CI from
  the freshly built Linux release binary and attach it to
  each GitHub release, with host filesystem access for the
  linter. Fully in-repo: no Flathub submission and no
  publisher token.
model: opus
depends-on: []
---
# Ship mdsmith as a self-hosted Flatpak bundle

## Goal

Let Linux users install mdsmith with `flatpak install`.
The release workflow already attaches per-platform
binaries to every `vX.Y.Z` GitHub release. This plan adds
a single-file `.flatpak` bundle to that asset set.

## Background

Flathub is not an option: it forbids `--filesystem=host`,
which a linter needs to read the files it checks. So the
bundle is self-hosted — built in CI and attached to the
GitHub release, installed by file with `flatpak install
./mdsmith-x86_64.flatpak`.

It is **x86_64 only**. `flatpak-builder` targets the
runner's native architecture, and cross-building aarch64
under emulation is not worth it for this channel. aarch64
Linux hosts use the binary, npm, or PyPI channels.

The bundle must reach the release while it is still a
**draft**: an immutable, published release rejects new
assets. So the build runs off `build` (like `vscode`),
not after `release`.

## In-repo work (this PR)

1. [`mdsmith-release build-flatpak`](../internal/release/flatpak.go)
   stages a flatpak-builder manifest with a local `path:`
   source plus the x86_64 Linux binary it references, so
   the bundle builds with no published download URL.
2. A `flatpak` job (`needs: [build]`) in
   [release.yml](../.github/workflows/release.yml) builds
   the x86_64 bundle with `flatpak-builder` +
   `flatpak build-bundle`, verifies it installs and that
   `mdsmith version` matches the tag, and uploads it as an
   artifact. `release` (`needs: [build, vscode, flatpak]`)
   attaches `mdsmith-x86_64.flatpak` to the draft, so the
   existing `mdsmith-*` checksum, SLSA attestation, and
   cosign steps cover it like the raw binaries.
3. The channel doc
   [flatpak.md](../docs/development/release-channels/flatpak.md)
   and the [install guide](../docs/guides/install.md)
   Flatpak section.

## Tasks

1. Add `build-flatpak` with a unit test that asserts the
   manifest's local `path:` source and that it stages the
   binary next to the manifest.
2. Wire the subcommand into
   [main.go](../cmd/mdsmith-release/main.go).
3. Add the `flatpak` job and make `release` depend on it.
4. Add the channel doc and the install-guide section.

## Acceptance Criteria

- [x] `build-flatpak` stages a manifest pinning the
      x86_64 Linux binary by local path and copies that
      binary next to it.
- [ ] `mdsmith check .` passes, including the regenerated
      release-channel catalog.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
