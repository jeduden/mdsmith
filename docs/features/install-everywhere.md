---
title: "Installs everywhere"
summary: >-
  One version-stamped Go binary ships through go install, npm, pip,
  uvx, mise, asdf, and GitHub Releases — with no postinstall network
  call, so locked-down CI installs offline.
icon: package
link: "/guides/install/"
weight: 15
---
# Installs everywhere

A linter that is hard to install does not get adopted. mdsmith is
one static Go binary, and every channel ships that same binary.

Install it with `go install`, `npm` or `npx @mdsmith/cli`, `pip`,
`uvx`, or `pipx`, through `mise` and `asdf`, or by direct
download from a GitHub Release. The editor extension comes from
the VS Code Marketplace and Open VSX.

The package wrappers carry no `postinstall` network call. A
locked-down or air-gapped CI runner installs the pinned version
offline, and the binary it runs is byte-for-byte the released
artifact.

See the [install guide](../guides/install.md) for the per-channel
commands and version pinning.
