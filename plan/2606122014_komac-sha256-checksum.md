---
id: 2606122014
title: "Add SHA256 checksum verification for komac download in release.yml"
status: "🔲"
summary: >-
  S001 from the 2026-06-12 full-repo audit: the winget-submit job
  downloads the komac binary with curl and executes it without any
  SHA256 or signature verification, while every other binary download
  in the repository has a hardcoded sha256sum -c check.
model: sonnet
---
# Add SHA256 checksum verification for komac download in release.yml

## Goal

Close finding S001 from the [2026-06-12 full-repo audit
report](../docs/security/2026-06-12-full-repo-audit/report.md).

The winget-submit job in `.github/workflows/release.yml:1015-1022`
downloads the komac binary with curl and marks it executable. It then
runs the binary with `WINGET_PR_TOKEN` in scope and no checksum check.
A compromised GitHub release asset at the pinned tag can substitute a
malicious binary that executes with the PR token.

Every other binary download in the repository uses `sha256sum -c`:

- tinygo in `ci.yml:563-568`
- VHS, ttyd, ffmpeg in `record-demo.yml`
- mdsmith in `setup-mdsmith-pinned-version/action.yml`

The winget-submit job is inside the `release` environment, which
requires human approval. The binary is fetched and executed within
that privileged step with no post-download integrity check.

## Tasks

- [ ] Find the exact komac version pinned in `release.yml` (e.g.
  `v1.x.y`) and compute the SHA256 of the Linux amd64 binary for
  that release.
- [ ] Add a `sha256sum -c` check in `release.yml` immediately after
  the curl line, following the same pattern as tinygo in
  `ci.yml:563-568`:
  `echo "<sha256>  /usr/local/bin/komac" | sha256sum -c`
- [ ] Verify the workflow step still passes in CI on the next release
  dispatch (or in a dry-run branch).
- [ ] Optionally: add a cosign signature verification step using the
  komac release sigstore bundle, if one is published.

## Acceptance Criteria

- The winget-submit job verifies the komac binary's SHA256 before
  executing it.
- The hardcoded hash matches the komac binary for the pinned version.
- The pattern matches the one used by other binary downloads in the
  repository (echo + sha256sum -c, not a separate file).
