---
title: Visual Studio Marketplace
summary: >-
  The mdsmith VS Code extension `.vsix`, published
  via a long-lived Marketplace publisher PAT.
registry: marketplace.visualstudio.com
credential: VSCE_PAT
job: vscode
channelurl: https://marketplace.visualstudio.com/items?itemName=jeduden.mdsmith
weight: 3
---
# Visual Studio Marketplace

The Marketplace channel publishes the mdsmith VS
Code extension to
[marketplace.visualstudio.com](https://marketplace.visualstudio.com).
The published artifact is a `.vsix` produced by
`@vscode/vsce package --no-dependencies`. The flag
strips dev dependencies from the bundle. The client
receives the compiled extension and the host
platform's mdsmith binary, nothing more.

Auth uses `VSCE_PAT`, a long-lived Azure DevOps
personal access token scoped to `Marketplace >
Manage`. The secret is stored on the `release`
GitHub environment. A workflow run outside an
approved release cannot read it.

Azure caps PATs at 365 days. Rotation is tracked in
[`secret-rotations.md`](../secret-rotations.md)
with a 335-day period — a 30-day buffer below the
cap.

The `vscode` job in `release.yml` calls
`vsce publish` with `--packagePath` pointing at the
same `.vsix` Open VSX and the GitHub release
attach. All three artifacts have identical SHA-256
sums.

The job depends on the `npm` job: the `.vsix`
bundles the host-platform binary by running
`bun install` against `@mdsmith/cli` at the release
version, so the npm platform packages must publish
first. See the [release pipeline
doc](../release.md#job-topology) for the full
dependency chain.
