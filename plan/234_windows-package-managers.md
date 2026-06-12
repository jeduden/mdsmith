---
id: 234
title: Distribute mdsmith on Windows via Scoop and WinGet
status: "✅"
summary: >-
  Publish the prebuilt `mdsmith-windows-amd64.exe` through
  a Scoop bucket and a WinGet manifest, mirroring the
  Homebrew tap and asdf plugin: a checksum-verified binary
  install bumped on each release. Adds two release-channel
  docs (so the install picker and table gain the rows
  automatically), two best-effort release-workflow jobs,
  and the dispatch tokens plus rotation entries they need.
model: sonnet
depends-on: []
---
# Distribute mdsmith on Windows via Scoop and WinGet

## Goal

Give Windows users a one-line package-manager install:
`scoop install mdsmith` or `winget install jeduden.mdsmith`.
Both serve the same checksum-verified release `.exe` as the
manual download.

## Background

Today Windows has only the direct `.exe` download (see the
Windows section of the
[install guide](../docs/guides/install.md)). There is no
package-manager channel, so every Windows install is a
manual download-and-PATH step.

[Plan 130](130_binary-distribution-and-versioning.md) shipped
npm, PyPI, and the VS Code marketplaces and left the OS
package managers — Scoop and Chocolatey among them — out of
scope as follow-ups. Peer linters
already publish to Windows repositories (mise ships via
WinGet, Scoop, and Chocolatey), so this is a known gap, not
a new direction.

The pattern to copy already exists in the repo. The
[Homebrew tap](../docs/development/release-channels/homebrew.md)
and the [asdf plugin](../docs/development/release-channels/asdf.md)
are both `pull` channels that install the prebuilt binary,
verify it against the release `checksums.txt`, and are
bumped on each release by a best-effort dispatch job in
[`release.yml`](../.github/workflows/release.yml) plus a
scheduled self-bump. Scoop and WinGet slot into that same
shape.

## Channel: Scoop

A new `jeduden/scoop-mdsmith` bucket repo holds a
`bucket/mdsmith.json` manifest. It pins the `version`, the
release `.exe` `url`, and its SHA-256 `hash` (from
`checksums.txt`). `bin: mdsmith.exe` exposes the command.
`checkver` and `autoupdate` blocks let the bucket self-bump.

- Install: `scoop bucket add mdsmith https://github.com/jeduden/scoop-mdsmith`
  then `scoop install mdsmith`.
- Release glue: a `notify-scoop-bucket` job in
  [`release.yml`](../.github/workflows/release.yml) mirrors
  the existing `notify-homebrew-tap` job — it fires a
  `repository_dispatch` at the bucket with a fine-grained
  PAT (`Contents: write` on the bucket). It is best-effort:
  the bucket self-bumps daily via `checkver`, so a missing
  token or failed dispatch never blocks a release.
- Channel doc: `docs/development/release-channels/scoop.md`,
  `mechanism: pull`, `platforms: [windows]`.

## Channel: WinGet

A `jeduden.mdsmith` manifest is submitted to
`microsoft/winget-pkgs` by a release job. `komac`
builds the manifest from the release's Windows
binary URL and opens the PR. It computes the
SHA-256 itself. The first version is bootstrapped
by hand with
`mdsmith-release render-winget-manifest`. That
mirrors `render-scoop-manifest` for the Scoop
bucket.

The asset is a bare CLI binary, not an installer, so
the manifest declares `InstallerType: portable` with
`PortableCommandAlias: mdsmith`. WinGet then stores
the binary and links it onto PATH as `mdsmith`. An
`exe` type with `Silent: /S` would instead make WinGet
run `mdsmith-windows-amd64.exe /S`, which the linter
rejects as a bad argument — nothing would install.

- Install: `winget install jeduden.mdsmith`. The short
  form works only after the manifest lands and Microsoft
  moderation merges the PR; until then the GitHub release
  `.exe` is the documented fallback, the same way the asdf
  and mise docs caveat their short forms.
- Release glue: a best-effort `winget-submit` job gated on
  the `release` environment, using a `WINGET_PR_TOKEN` PAT
  that can fork `winget-pkgs` and open a PR. A missing
  token skips the job and never fails the release.
- Channel doc: `docs/development/release-channels/winget.md`,
  `command: winget install jeduden.mdsmith`,
  `platforms: [windows]`, `unlisted: true`. The flag
  keeps WinGet out of the install picker and table
  until the manifest PR merges, since nothing installs
  through WinGet before then; the doc and tooling stay.

## Manifest generation belongs in mdsmith-release

Per the
[release-tooling rule](../docs/development/release-tooling.md),
workflow logic lives in the `mdsmith-release` Go CLI, not
inline shell. Add `render-scoop-manifest` and
`render-winget-manifest` subcommands. Each takes the version
and `checksums.txt` and emits one manifest. Unit-test the
version, URL, and hash substitution red/green.

## Picker, table, and install guide

Both channel docs feed `channels.yaml` through
`sync-channels`. The picker and the install table gain the
Scoop row with no manual edit. WinGet's doc carries
`unlisted: true`, so `sync-channels` drops it from the
picker and the install-table catalog excludes it by glob —
it stays out of both until the manifest PR merges.

Both are CLI binary-download channels. They
should sort among the CLI channels (Homebrew 7, asdf 9,
GitHub Releases 10), ahead of the higher-weighted ones.
Weights only need to be `>= 1` and sort ascending; they need
not be unique or contiguous. So give Scoop and WinGet weights
just above GitHub Releases. Bump the channels now at 11–14
(the two marketplaces, Flatpak, and Obsidian) to make room,
or let weights tie — the stable sort keeps ties in file
order.

Update the Windows section of the
[install guide](../docs/guides/install.md). Replace the
"no package-manager channel yet" lead with the Scoop
one-liner. Keep the manual `.exe` download as the offline
or air-gapped path. The WinGet one-liner is added back
when the manifest PR merges and `unlisted` is dropped.

## Secrets and rotation

Two new tokens need rotation tracking in
[secret-rotations](../docs/development/secret-rotations.md).
They are `SCOOP_BUCKET_DISPATCH_TOKEN` (a plain repo secret,
like the existing tap dispatch token) and `WINGET_PR_TOKEN`.
Add a file per secret under `secret-rotations/`. The
scheduled 30-day reminder then covers both.

## Out of scope

Chocolatey is deferred. It needs a `chocolatey.org` account,
an API key (another rotated secret), and a moderation queue
— more friction than Scoop or WinGet, and not required for
default-Windows reach (WinGet ships with Windows 11).

## Tasks

1. [x] Add `mdsmith-release render-scoop-manifest` with unit
   tests (version, URL, and SHA-256 from `checksums.txt`).
2. [x] Create the `jeduden/scoop-mdsmith` bucket repo with the
   manifest, `checkver`, and `autoupdate`. (external — repo
   now available)
3. [x] Add the `notify-scoop-bucket` job and
   `SCOOP_BUCKET_DISPATCH_TOKEN`; document its rotation.
4. [x] Add `docs/development/release-channels/scoop.md`.
5. [x] Add `mdsmith-release render-winget-manifest` with unit
   tests.
6. [x] Add the `winget-submit` job using `komac` and
   `WINGET_PR_TOKEN`; document its rotation.
7. [x] Add `docs/development/release-channels/winget.md`.
8. [x] Run `sync-channels`; confirm the picker and table show
   the Scoop row, weighted among the CLI channels. WinGet is
   `unlisted` until its manifest PR merges.
9. [x] Update the Windows section of the install guide.

## Acceptance Criteria

All in-repo tasks are complete. `✅` reflects that all in-repo work is
done. The two criteria below remain unchecked because each has an
external gate. The Scoop bucket (`jeduden/scoop-mdsmith`) self-bumps
but lives outside this repo. The `winget-submit` job runs here and
opens the PR, but `winget install jeduden.mdsmith` only works after
Microsoft moderation merges that PR.

- [ ] `scoop install mdsmith` (after `scoop bucket add`)
      installs the released `.exe`, checksum-verified.
      (external — jeduden/scoop-mdsmith repo is live)
- [ ] `winget install jeduden.mdsmith` installs the released
      `.exe` once the manifest PR is merged.
      (external — winget-submit job opens the PR; Microsoft
      moderation must merge it before the command works)
- [x] Manifest generation lives in `mdsmith-release`
      (`render-scoop-manifest`, `render-winget-manifest`),
      not inline workflow shell; the recurring
      `winget-submit` job has `komac` generate the WinGet
      manifest from the binary URL. The manifest is a
      `portable` installer type, so WinGet links the binary
      onto PATH as `mdsmith` instead of executing it.
- [x] A missing `SCOOP_BUCKET_DISPATCH_TOKEN` or
      `WINGET_PR_TOKEN` logs a notice and never fails the
      release.
- [x] The install picker and table show the Scoop row for
      Windows; WinGet carries `unlisted: true` and is held
      out of both until its manifest PR merges.
- [x] secret-rotations tracks both tokens under the 30-day
      reminder.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues.
- [x] `mdsmith check .` passes.
