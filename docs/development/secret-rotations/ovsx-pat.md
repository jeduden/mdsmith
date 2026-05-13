---
title: OVSX_PAT
summary: >-
  Open VSX publisher token. Drives the `ovsx publish`
  step.
lastRotated: "2026-05-12"
periodDays: 335
provider: Open VSX
issuerUrl: "https://open-vsx.org/user-settings/tokens"
usedBy: "release.yml (Publish to Open VSX)"
scope: "Publish to the jeduden namespace"
releaseEnvScoped: true
---
# OVSX_PAT

Sign in to
[open-vsx.org/user-settings/tokens](https://open-vsx.org/user-settings/tokens)
(authentication goes through GitHub OAuth) and
generate a token scoped to the `jeduden` namespace.
Open VSX does not force expiry, so the only deadline
is the local cadence in `period-days`.

Store the value as the `OVSX_PAT` secret on the
`release` GitHub environment.
