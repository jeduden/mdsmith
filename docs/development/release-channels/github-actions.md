---
title: GitHub Actions
summary: >-
  A composite action at the repository root downloads the
  checksum-verified release binary for the runner's OS and
  architecture, puts `mdsmith` on `PATH`, and runs the
  command in its `args` input; referenced as
  `uses: jeduden/mdsmith@<commit-sha>`.
mechanism: pull
artifact: cli
command: "uses: jeduden/mdsmith@v0"
audience: Linting Markdown inside GitHub Actions CI
platforms: [linux, macos, windows]
channelurl: https://github.com/jeduden/mdsmith
weight: 15
unlisted: true
---
# GitHub Actions

Release page: <https://github.com/jeduden/mdsmith>

The repository root carries an `action.yml`, so a workflow
step runs mdsmith with:

```yaml
- uses: jeduden/mdsmith@<commit-sha>  # v0.41.0
  with:
    version: v0.41.0   # mdsmith release to install, or latest
    args: check .      # omit to only put mdsmith on PATH
```

The composite action reads `$RUNNER_OS` and
`$RUNNER_ARCH`. It maps them to the matching release
asset — `mdsmith-linux-amd64`, `mdsmith-darwin-arm64`,
`mdsmith-windows-amd64.exe`, and the rest. It downloads
that asset over HTTPS. Then it verifies the SHA-256
against the release's `checksums.txt` before it adds the
binary to `PATH`.

macOS runners fall back to `shasum -a 256`, since they
ship no GNU `sha256sum`. Windows runners get the one
`windows-amd64` build. Any other Windows architecture
fails with a clear error, not a 404.

Three inputs drive it. `version` selects the release —
`latest` (the default) or a tag such as `v0.41.0`. `args`,
when non-empty, is split on whitespace and passed to
`mdsmith`; an empty `args` only installs the binary so a
later step can call it. `working-directory` sets the
directory the `args` command runs in. The action exposes
one output, `version`, the string `mdsmith version` prints.

Pin `uses:` to a full-length commit SHA for the strongest
guarantee: a SHA can never move. mdsmith publishes
immutable releases, so a release-tag pin like `@v0.41.0`
is a safe, reproducible alternative. Only the floating
`@v0` tag moves by design. GitHub recommends the SHA form,
and this repository uses it for every third-party action.
Keep the version in a trailing comment, as `# v0.41.0`
above.

The action still verifies the downloaded binary's SHA-256
against the release `checksums.txt`. So the action and the
binary it fetches are both pinned by digest, not by a
movable name.

No released commit carries the action yet. Pin to a commit
SHA from this branch, or from `main` once it merges, to use
it today. After the next release, pin to that release's
commit. The convenience tag `@v0` comes later: it must be
created and moved onto a release that ships `action.yml`.

You can also skip the action entirely. Run the release
binary in a `run:` step. That repeats by hand the download
and verify steps the action automates.

Because no published tag installs the action yet, this
channel sets `unlisted: true` in its frontmatter, so
`sync-channels` keeps it out of the website install picker
and the install-guide table excludes it by glob. The
`action.yml` and this doc stay; only the user-facing
listings wait for a release to carry the action. Drop both
once `uses: jeduden/mdsmith@v0` resolves.
