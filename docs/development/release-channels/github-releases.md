---
title: GitHub Releases
summary: >-
  Per-platform mdsmith binaries plus the .vsix, the
  checksum file, and a Sigstore signature, attached
  to a tag-named release.
registry: github.com/jeduden/mdsmith/releases
credential: GITHUB_TOKEN + OIDC
job: release
channelurl: https://github.com/jeduden/mdsmith/releases
weight: 5
---
# GitHub Releases

Release page: <https://github.com/jeduden/mdsmith/releases>

The GitHub Releases channel attaches every artifact
the build matrix produced to a release named after
the tag:

- One mdsmith binary per platform
  (`mdsmith-linux-amd64`, `mdsmith-linux-arm64`,
  `mdsmith-darwin-amd64`, `mdsmith-darwin-arm64`,
  `mdsmith-windows-amd64.exe`)
- The mdsmith VS Code extension `.vsix`
- `checksums.txt` — SHA-256 of every binary
- `checksums.txt.bundle` — cosign keyless signature
  on `checksums.txt`

This channel is the documented fallback for any
other channel being unavailable. It is also the
source mise's `ubi` backend reads to install
mdsmith without a registry plugin.

The release is triggered from the Actions
"Run workflow" UI, not a pushed tag; the `release`
job creates the tag itself (`tag_name` +
`target_commitish`) at the dispatched commit as part
of the draft release. See [`release.md`](../release.md)
for the trigger rationale.

The `release` job in `release.yml` uses the
workflow's default `GITHUB_TOKEN` for the
`softprops/action-gh-release` upload. It mints a
short-lived OIDC token for
`actions/attest-build-provenance` — one attestation
per binary and per `.vsix`. It mints another OIDC
token for `cosign sign-blob` against
`checksums.txt`. The signing certificate binds the
signature to this exact workflow file at the tag
that triggered it. See [`release.md`](../release.md)
for the end-user verification commands.
