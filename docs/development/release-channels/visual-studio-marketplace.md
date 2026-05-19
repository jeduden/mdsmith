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

Release page:
<https://marketplace.visualstudio.com/items?itemName=jeduden.mdsmith>

The Marketplace channel publishes the mdsmith VS
Code extension to
[marketplace.visualstudio.com](https://marketplace.visualstudio.com).
The published artifact is a `.vsix` from
`@vscode/vsce package --no-dependencies`. That flag
strips dev dependencies. The client receives the
compiled extension. Under `dist/cli/` it also gets
the `@mdsmith/cli` shim and a prebuilt binary for
every supported platform. The extension picks the
right one at startup, re-using that shim's resolver.

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

The job depends on the `build` job, not `npm`. It
downloads the release binary artifacts and runs
`mdsmith-release build-npm` — the same generator the
npm channel uses — to stage all platform binaries,
which `build.ts` bundles under `dist/cli/`. So the
`.vsix` no longer waits on the npm publish. See the
[release pipeline doc](../release.md#job-topology)
for the full dependency chain.
