---
title: VSCE_PAT
summary: >-
  Visual Studio Marketplace publisher PAT issued by
  Azure DevOps. Drives the `vsce publish` step.
lastRotated: "2026-05-12"
periodDays: 335
provider: Azure DevOps
issuerUrl: "https://dev.azure.com"
usedBy: "release.yml (Publish to Visual Studio Marketplace)"
scope: "Marketplace > Manage"
releaseEnvScoped: true
---
# VSCE_PAT

The Marketplace runs on Azure. PATs are minted at
[dev.azure.com](https://dev.azure.com) under **User
settings → Personal access tokens**.

Settings on issuance:

- **Organization:** the org that owns the `jeduden`
  Marketplace publisher namespace.
- **Expiration:** the maximum Azure allows (currently
  1 year).
- **Scopes:** Custom defined → **Marketplace →
  Manage**. Nothing else.

Store the value as the `VSCE_PAT` secret on the
`release` GitHub environment.
[Environment settings page.][env-settings]
The verification step in `release.yml` fails the job
early if the secret is missing or empty.

[env-settings]: https://github.com/jeduden/mdsmith/settings/environments
