---
title: Release Candidates
summary: >-
  Every merge to `main` publishes a `vX.Y.Z-rc.N`
  GitHub pre-release — the minor after the latest
  stable tag, with N counting up per merge — carrying
  the binaries, `.vsix`, and Obsidian zip, signed and
  attested like a stable release. No registry, no
  approval, never marked latest.
---
# Release Candidates

`.github/workflows/release-candidate.yml` runs on every
push to `main` (and on manual dispatch from `main`;
a dispatch from any other branch skips every job). It
cuts a GitHub **pre-release** so each merge can be
installed and tested before a stable release.

The version is the minor after the latest stable tag,
plus `-rc.N`. `mdsmith-release rc-version` reads the
repository tags. It finds the highest plain `vX.Y.Z`
tag and bumps the minor. N is one above the highest
existing `-rc.N` for that minor. After `v0.55.1`, the
first merge gets `v0.56.0-rc.1` and the next gets
`v0.56.0-rc.2`. Once `v0.56.0` ships, the series moves
on to `v0.57.0-rc.1`.

The candidate carries the same assets as a stable
release, except the Flatpak bundle: the five binaries
(PGO-built), the `.vsix`, the Obsidian zip, the SBOM,
`checksums.txt`, SLSA attestations, and a cosign
bundle. It uses the same draft-then-publish flow, so
each candidate is an immutable release. Its notes span
from the last stable tag.

Only GitHub Releases receives a candidate. The
workflow publishes nothing to npm, PyPI, the
Marketplace, Open VSX, Homebrew, Scoop, or WinGet. It
uses only `GITHUB_TOKEN` and its own OIDC token. It
targets neither the `release` nor the
`release-approval` environment, and
`release-gate-guard` keeps it that way. No approval is
needed.

A candidate never becomes "latest". The pre-release
flag keeps it out of `/releases/latest`, which
`action.yml`, mise, and the direct-download commands
resolve. `go install ...@latest` skips semver
pre-releases. The `pages.yml` version fallback runs
`git describe --exclude 'v*-*'`. To try a candidate,
name its tag:
`go install github.com/jeduden/mdsmith/cmd/mdsmith@v0.56.0-rc.2`,
or download its assets from the release page.

Each candidate is signed under its own workflow. The
cosign certificate names `release-candidate.yml`. The
stable verify command pins `release.yml`, so it
rejects a candidate. To check a candidate, put
`release-candidate.yml` in the
`--certificate-identity-regexp` pattern.

`concurrency: { group: release-candidate }` runs one
candidate at a time, so two runs never compute the
same `-rc.N`. A burst of merges queues only the newest
run, so it yields one candidate for the newest commit.

A stable release can still ship while a candidate
builds. The run picked `v0.56.0-rc.5`; then `v0.56.0`
lands. Just before publishing,
`mdsmith-release check-rc --discard-draft` re-reads the
tags. If the version is no longer the next candidate,
it deletes the run's draft and skips the publish. The
run stays green with a notice, and the next merge cuts
`v0.57.0-rc.1`.

After the publish step, the `pages` job calls
`pages.yml` to redeploy mdsmith.dev. The deploy runs
`mdsmith-release sync-releases`, so the site's
[release notes page](https://mdsmith.dev/releases/)
lists the new candidate. Candidates appear below the
stable releases there, each collapsed, with its notes.

The build steps copy `release.yml`'s `build`, `vscode`,
and `obsidian` jobs. Keep the two files in step.
