---
title: SCOOP_BUCKET_DISPATCH_TOKEN
summary: >-
  GitHub fine-grained PAT for dispatching a manifest
  bump to the jeduden/scoop-mdsmith bucket. Gated by
  the `release` environment.
lastRotated: "2026-06-05"
periodDays: 335
provider: GitHub
issuerUrl: "https://github.com/settings/personal-access-tokens"
usedBy: "release.yml (notify-scoop-bucket)"
scope: "Contents: read+write (jeduden/scoop-mdsmith only)"
releaseEnvScoped: true
---
# SCOOP_BUCKET_DISPATCH_TOKEN

Generated at the
[GitHub fine-grained tokens page][gh-pat]. This token
is scoped to the `release` environment, so it is
unreadable until the release run's single approval.
The bucket at `jeduden/scoop-mdsmith` self-bumps daily
via its `checkver` schedule, so a missing token only
delays an immediate bump. The next scheduled run
covers it.

Settings on issuance:

- **Resource owner:** jeduden.
- **Repository access:** Only select repositories →
  `jeduden/scoop-mdsmith`.
- **Repository permissions:**
  - Contents: Read and write
  - Metadata: Read (automatic)
- **Expiration:** 1 year.

Store as the `SCOOP_BUCKET_DISPATCH_TOKEN` secret on
the `release` [environment][environments]. The
reminder workflow opens an issue 30 days before
expiry; rotate then to keep dispatches immediate.

[gh-pat]: https://github.com/settings/personal-access-tokens
[environments]: https://github.com/jeduden/mdsmith/settings/environments
