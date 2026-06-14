---
title: GitHub Actions
summary: >-
  A composite action at the repository root downloads the
  checksum-verified release binary for the runner's OS and
  architecture, puts `mdsmith` on `PATH`, and runs the
  command in its `args` input; referenced as
  `uses: jeduden/mdsmith@<ref>`.
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
- uses: jeduden/mdsmith@v0
  with:
    version: latest   # a release tag like v0.41.0, or latest
    args: check .     # omit to only put mdsmith on PATH
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

For a locked-down supply chain, pin `uses:` to a release
tag or a commit SHA, the way this repository pins every
third-party action it consumes.

The short `uses: jeduden/mdsmith@v0` form needs two
things. A tagged release must ship this `action.yml`. The
floating `v0` tag must then move onto it. Until that
happens, pin the action to a commit SHA or use `@main`.
You can also skip the action and run the release binary in
a `run:` step. That repeats by hand the download and
verify steps the action automates.

Because no published tag installs the action yet, this
channel sets `unlisted: true` in its frontmatter, so
`sync-channels` keeps it out of the website install picker
and the install-guide table excludes it by glob. The
`action.yml` and this doc stay; only the user-facing
listings wait for a release to carry the action. Drop both
once `uses: jeduden/mdsmith@v0` resolves.
