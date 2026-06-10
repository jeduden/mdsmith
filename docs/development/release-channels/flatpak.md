---
title: Flatpak
summary: >-
  A single-file `.flatpak` bundle built in CI from the
  x86_64 Linux release binary and attached to each
  GitHub release, installed by file with host
  filesystem access for the linter.
mechanism: push
artifact: cli
command: curl -LO https://github.com/jeduden/mdsmith/releases/latest/download/mdsmith-x86_64.flatpak && flatpak install ./mdsmith-x86_64.flatpak
audience: Sandboxed Linux x86_64 desktops via Flatpak
platforms: [linux]
registry: github.com/jeduden/mdsmith/releases
credential: GITHUB_TOKEN + OIDC
job: flatpak
channelurl: https://github.com/jeduden/mdsmith/releases
weight: 13
---
# Flatpak

Release page: <https://github.com/jeduden/mdsmith/releases>

The Flatpak channel ships mdsmith as a single-file
`.flatpak` bundle. The bundle is attached to each GitHub
release. Its app id is `io.github.jeduden.mdsmith`, and it
is **x86_64 only**. The commands below download the latest
`mdsmith-x86_64.flatpak`, install it, and run it:

```bash
curl -LO https://github.com/jeduden/mdsmith/releases/latest/download/mdsmith-x86_64.flatpak
flatpak install ./mdsmith-x86_64.flatpak
flatpak run io.github.jeduden.mdsmith check .
```

The first install also pulls the
`org.freedesktop.Platform` 24.08 runtime from Flathub.
That happens only if the host lacks it. Only the runtime
comes from Flathub; the mdsmith binary is baked into the
bundle. The bundle records Flathub as its runtime source,
so the install can offer it.

Flatpak sandboxes every app. So the manifest declares
`--filesystem=host`. A linter must read whatever files the
user points it at. Those files can live anywhere. A
narrower grant would hide them.

The app is one prebuilt binary, so the manifest has no
compile step. A `file` source installs the x86_64 binary
as `/app/bin/mdsmith`. That source is a local path, not a
release-download URL. The bundle is built from the freshly
built binary, before the release that hosts it exists.

The `flatpak` job in `release.yml` chains off `build`, not
`release`. So the bundle is ready before the draft release
freezes. An immutable, published release rejects new
assets, so the timing matters.

The job stages the manifest and the binary with
[`build-flatpak`](../release-tooling.md). It runs
`flatpak-builder` against the freedesktop 24.08 runtime
and SDK. Then `flatpak build-bundle` packs the result into
one file. The job also verifies the bundle installs and
reports the tag's version. Then it uploads the bundle as
an artifact.

The `release` job attaches that artifact to the draft. The
bundle is named `mdsmith-x86_64.flatpak`. So it matches
the `mdsmith-*` glob the release job already uses. That
glob drives `checksums.txt`, the SLSA build-provenance
attestation, and the cosign signature. All three cover the
bundle, the same as the raw binaries.

Only x86_64 ships. `flatpak-builder` targets the runner's
native architecture. Cross-building aarch64 under
emulation is not worth it for this channel. aarch64 Linux
hosts use the binary, npm, or PyPI channels.

Auth: none of its own. The bundle rides the `release`
job's `GITHUB_TOKEN` upload and OIDC signing. That is the
same path the other release binaries take. There is no
separate publisher token to rotate.
