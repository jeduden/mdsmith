---
title: Open VSX
summary: >-
  The same `.vsix` republished to Open VSX so
  VSCodium, Cursor, Theia, and Gitpod can install
  it.
registry: open-vsx.org
credential: OVSX_PAT
job: vscode
channelurl: https://open-vsx.org/extension/jeduden/mdsmith
weight: 4
---
# Open VSX

Release page: <https://open-vsx.org/extension/jeduden/mdsmith>

Open VSX is the registry VSCodium, Cursor, Theia,
and Gitpod query. Publishing the same `.vsix` to
both Marketplace and Open VSX lets every IDE in
that family install mdsmith with its native
extension command.

Auth uses `OVSX_PAT`, a long-lived token issued
from
[open-vsx.org/user-settings/tokens](https://open-vsx.org/user-settings/tokens).
Sign-in is via GitHub OAuth. Open VSX does not
force token expiry. The only deadline is the local
rotation cadence — see
[`secret-rotations.md`](../secret-rotations.md).
The secret is stored on the `release` GitHub
environment, same as `VSCE_PAT`.

The `vscode` job in `release.yml` reuses the exact
`.vsix` produced for the Marketplace publish. The
Open VSX, Marketplace, and GitHub-release
artifacts are byte-identical.
