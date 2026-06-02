---
title: mise
summary: >-
  mise's `ubi` backend installs mdsmith from GitHub
  release assets; the short `mise use mdsmith` form
  awaits a registry entry.
mechanism: pull
artifact: cli
command: mise use -g ubi:jeduden/mdsmith@latest
audience: Repos using mise; works today via GitHub releases
platforms: [macos, linux]
channelurl: https://mise.jdx.dev/
weight: 8
---
# mise

Release page: <https://mise.jdx.dev/>

mise installs mdsmith through its `ubi` backend, which
reads the GitHub release assets directly. The
`ubi:jeduden/mdsmith` form works today with no
registry entry. The short `mise use mdsmith@latest`
form needs the registry entry tracked in
[plan/145](../../../plan/145_asdf-mise-registry-submissions.md).
