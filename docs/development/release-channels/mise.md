---
title: mise
summary: >-
  mise's `github` backend installs mdsmith from GitHub
  release assets; the short `mise use mdsmith` form
  awaits a registry entry.
mechanism: pull
artifact: cli
command: mise use github:jeduden/mdsmith
audience: Repos using mise; works today via GitHub releases
platforms: [macos, linux]
channelurl: https://mise.jdx.dev/
weight: 8
---
# mise

Release page: <https://mise.jdx.dev/>

mise installs mdsmith through its `github` backend,
which reads the GitHub release assets directly. The
`github:jeduden/mdsmith` form works today with no
registry entry. The older `ubi:jeduden/mdsmith` form
still resolves, but mise has deprecated the `ubi`
backend. The short `mise use mdsmith@latest`
form needs the registry entry tracked in
[plan/145](../../../plan/145_asdf-mise-registry-submissions.md).
