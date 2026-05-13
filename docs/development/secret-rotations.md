---
title: Secret Rotations
summary: >-
  Rotation cadence and procedure for the long-lived
  publisher tokens consumed by the release and
  merge-queue workflows. Each tracked secret has its
  own file under `secret-rotations/`; the catalog
  below enumerates them. The scheduled reminder
  workflow consumes the same files and opens a
  GitHub issue when any secret is within 30 days of
  expiry.
---
# Secret Rotations

Every tracked secret has its own file under
[`secret-rotations/`](secret-rotations/) with the
canonical `lastRotated` date in its front matter.
The [`mdsmith-release check-secret-rotations`
subcommand][script] globs that directory; the
monthly [reminder workflow][reminder] runs it. The
subcommand opens a GitHub issue 30 days before any
tracked secret is due, so a human is reminded
without having to remember.

<?catalog
glob: "secret-rotations/*.md"
sort: title
header: |
  | Secret | Provider | Last rotated | Period (days) |
  |--------|----------|--------------|---------------|
row: "| [{title}]({filename}) | {provider} | {lastRotated} | {periodDays} |"
?>
| Secret                                                     | Provider     | Last rotated | Period (days) |
|------------------------------------------------------------|--------------|--------------|---------------|
| [MERGE_QUEUE_TOKEN](secret-rotations/merge-queue-token.md) | GitHub       | 2026-05-12   | 335           |
| [OVSX_PAT](secret-rotations/ovsx-pat.md)                   | Open VSX     | 2026-05-12   | 335           |
| [VSCE_PAT](secret-rotations/vsce-pat.md)                   | Azure DevOps | 2026-05-12   | 335           |
<?/catalog?>

The 335-day default `periodDays` leaves a 30-day
buffer below the 365-day cap Azure enforces on
PATs. Open VSX and GitHub do not force expiry but
follow the same cadence for consistency.

## Rotation order (applies to every secret)

Always rotate in this order. Reversing it produces a
broken release window:

1. **Generate** a new credential at the issuer.
2. **Store** it as the matching GitHub Actions secret
   (in the `release` environment for entries whose
   front-matter `releaseEnvScoped: true`, as a
   plain repo secret otherwise).
3. **Test** by triggering the consuming workflow (a
   tag push for the publisher tokens, a `queue` label
   for the merge-queue token). Confirm it succeeds.
4. **Revoke** the previous credential at the issuer.
5. **Record** the rotation: go to the Actions tab,
   pick **Record Secret Rotation**, click
   **Run workflow**, select the rotated secret and
   (optionally) a date. The workflow updates the
   `lastRotated` field in the matching per-secret
   file and opens a PR with `Closes #N` for the open
   reminder issue. Review and merge the PR —
   CODEOWNERS still gates the change, and the merge
   auto-closes the reminder issue.

## How the reminder runs

[`secret-rotation-reminder.yml`][reminder] runs at
09:00 UTC on the first day of every month
(`cron: 0 9 1 * *`). It also accepts
`workflow_dispatch` for manual runs.

The reminder script does not auto-close issues.
Nobody hand-edits the front matter either. Both
happen through the `Record Secret Rotation`
workflow (step 5 of the rotation order above). The
workflow updates `lastRotated` and opens a PR with
`Closes #N` pointing at the reminder issue. The
merge records the date and closes the reminder. The
next monthly reminder run sees the new date and
stays quiet until the next window.

[script]: ../../internal/release/secretrotations.go
[reminder]: ../../.github/workflows/secret-rotation-reminder.yml
