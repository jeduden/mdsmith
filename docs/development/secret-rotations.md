---
title: Secret Rotations
summary: >-
  Rotation cadence and procedure for the long-lived
  publisher tokens consumed by the release and
  merge-queue workflows. The `rotations:` table is
  machine-parsed by the scheduled reminder workflow,
  which opens a GitHub issue when any secret is
  within 30 days of expiry.
rotations:
  - name: VSCE_PAT
    last-rotated: "2026-05-12"
    period-days: 335
    provider: Azure DevOps
    issuer-url: "https://dev.azure.com"
    used-by: "release.yml (Publish to Visual Studio Marketplace)"
    scope: "Marketplace > Manage"
  - name: OVSX_PAT
    last-rotated: "2026-05-12"
    period-days: 335
    provider: Open VSX
    issuer-url: "https://open-vsx.org/user-settings/tokens"
    used-by: "release.yml (Publish to Open VSX)"
    scope: "Publish to the jeduden namespace"
  - name: MERGE_QUEUE_TOKEN
    last-rotated: "2026-05-12"
    period-days: 335
    provider: GitHub
    issuer-url: "https://github.com/settings/personal-access-tokens"
    used-by: "merge-queue.yml (jeduden/merge-queue-action)"
    scope: "Contents: read+write; Pull requests: read+write (jeduden/mdsmith only)"
---
# Secret Rotations

Three long-lived tokens still ship outside OIDC
Trusted Publishing. The front matter table above is
machine-parsed by
[`.github/scripts/check-secret-rotations.py`][script].
The [`secret-rotation-reminder.yml`][reminder]
workflow runs it on the first of each month. The
script opens a GitHub issue when any secret is
within 30 days of expiry, so a human is reminded
without having to remember.

The period is **335 days, not 365**: Azure caps PATs
at 365 days, so a 30-day buffer leaves a window to
rotate without an outage. The other two providers
have no forced cap but share the same cadence for
consistency.

## Rotation order (applies to every secret)

Always rotate in this order. Reversing it produces a
broken release window:

1. **Generate** a new credential at the issuer.
2. **Store** it as the matching GitHub Actions secret
   (in the `release` environment for `VSCE_PAT` /
   `OVSX_PAT`, as a plain repo secret for
   `MERGE_QUEUE_TOKEN`).
3. **Test** by triggering the consuming workflow (a
   tag push for the publisher tokens, a `queue` label
   for the merge-queue token). Confirm it succeeds.
4. **Revoke** the previous credential at the issuer.
5. **Update** the `last-rotated` date in the front
   matter of this file. Commit and merge — the
   reminder workflow uses that date to compute the
   next due date.
6. **Close** the open `secret-rotation` issue (the
   reminder workflow does not auto-close).

## VSCE_PAT — Visual Studio Marketplace

The Marketplace runs on Azure. PATs are minted at
[`https://dev.azure.com`][azure-pat] under
**User settings → Personal access tokens**.

Settings on issuance:

- **Organization:** the org that owns the `jeduden`
  Marketplace publisher namespace.
- **Expiration:** 1 year (the maximum Azure allows).
- **Scopes:** Custom defined → **Marketplace →
  Manage**. Nothing else.

Store the value as the `VSCE_PAT` secret on the
`release` environment at
<https://github.com/jeduden/mdsmith/settings/environments>.
The verification step in `release.yml` fails the job
early if the secret is missing or empty, so a
misconfigured rotation surfaces before the publish
step runs.

## OVSX_PAT — Open VSX

Sign in to <https://open-vsx.org/user-settings/tokens>
(authentication goes through GitHub OAuth) and
generate a token scoped to the `jeduden` namespace.
Open VSX does not force expiry, so the only deadline
is the local cadence in this file's front matter.

Store the value as the `OVSX_PAT` secret on the
`release` environment.

## MERGE_QUEUE_TOKEN — GitHub fine-grained PAT

Generated at
<https://github.com/settings/personal-access-tokens>.
This token is **not** environment-scoped because
`merge-queue.yml` runs on every PR-label event and
cannot wait on an environment approval per PR. Its
blast radius is branch-protection bypass on `main`,
not registry publishing — the threat model accepts a
broader exposure here in exchange for the merge
queue continuing to work without manual approvals.

Settings on issuance:

- **Resource owner:** jeduden.
- **Repository access:** Only select repositories →
  `jeduden/mdsmith`.
- **Repository permissions:**
  - Contents: Read and write
  - Pull requests: Read and write
  - Metadata: Read (automatic)
- **Expiration:** 1 year.

Store as the `MERGE_QUEUE_TOKEN` repo secret at
<https://github.com/jeduden/mdsmith/settings/secrets/actions>.
The reminder workflow opens an issue 30 days before
expiry; rotate then to avoid silent merge-queue
breakage.

## How the reminder runs

[`secret-rotation-reminder.yml`][reminder] is wired
to `schedule: { cron: "0 9 1 * *" }`, so it runs at
09:00 UTC on the first day of every month. It can
also be triggered manually via
`workflow_dispatch`.

The script does not auto-close issues. After a
rotation completes, edit the `last-rotated` date in
the front matter above, merge the change, and close
the issue. The next monthly run sees the new date
and waits.

[script]: ../../.github/scripts/check-secret-rotations.py
[reminder]: ../../.github/workflows/secret-rotation-reminder.yml
[azure-pat]: https://dev.azure.com
