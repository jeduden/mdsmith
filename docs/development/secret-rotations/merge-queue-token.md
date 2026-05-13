---
title: MERGE_QUEUE_TOKEN
summary: >-
  GitHub fine-grained PAT for the merge-queue action.
  Plain repo secret — not gated by an environment.
lastRotated: "2026-05-12"
periodDays: 335
provider: GitHub
issuerUrl: "https://github.com/settings/personal-access-tokens"
usedBy: "merge-queue.yml (jeduden/merge-queue-action)"
scope: "Contents: read+write; Pull requests: read+write (jeduden/mdsmith only)"
releaseEnvScoped: false
---
# MERGE_QUEUE_TOKEN

Generated at the
[GitHub fine-grained tokens page][gh-pat]. This token
is **not** environment-scoped because
`merge-queue.yml` runs on every PR-label event and
cannot wait on an environment approval per PR. Its
blast radius is branch-protection bypass on `main`,
not registry publishing. The threat model accepts a
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

Store as the `MERGE_QUEUE_TOKEN` repo secret on the
[Actions secrets page][actions-secrets]. The
reminder workflow opens an issue 30 days before
expiry; rotate then to avoid silent merge-queue
breakage.

[gh-pat]: https://github.com/settings/personal-access-tokens
[actions-secrets]: https://github.com/jeduden/mdsmith/settings/secrets/actions
