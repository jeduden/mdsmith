---
title: GitHub Actions
summary: >-
  A composite action at the repository root downloads the
  checksum-verified release binary for the runner's OS and
  architecture and puts `mdsmith` on `PATH`; published to
  the GitHub Marketplace and pinned by commit SHA or
  release tag.
mechanism: push
artifact: cli
command: "uses: jeduden/mdsmith@vX.Y.Z"
audience: Linting Markdown inside GitHub Actions CI
platforms: [linux, macos, windows]
registry: github.com/marketplace
credential: GITHUB_TOKEN
job: release
channelurl: https://github.com/marketplace/actions/mdsmith
weight: 15
unlisted: true
---
# GitHub Actions

Release page: <https://github.com/marketplace/actions/mdsmith>

The repository root carries an `action.yml`, so a workflow
step runs mdsmith with:

```yaml
- uses: jeduden/mdsmith@<commit-sha>  # vX.Y.Z
  with:
    version: latest    # which mdsmith release to install (a tag, or latest)
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
immutable releases, so a release-tag pin like `@vX.Y.Z`
(any release that ships `action.yml`) is a safe,
reproducible alternative. GitHub recommends the SHA form,
and this repository uses it for every third-party action.
Keep the version in a trailing comment, as `# vX.Y.Z`
above.

The action still verifies the downloaded binary's SHA-256
against the release `checksums.txt`. So the action and the
binary it fetches are both pinned by digest, not by a
movable name.

The action publishes to the GitHub Marketplace through a
release. The `release` job drafts a release at the tagged
commit, which carries this `action.yml`. Publishing that
draft adds or updates the Marketplace listing.

The first listing is a manual, one-time step. The
maintainer accepts the Marketplace Developer Agreement and
enables it on a release. A unique action `name` and
`branding` are required; `action.yml` sets both.

The first release that ships this `action.yml` makes the
Marketplace listing and its release tags resolve. Until
then, pin to a commit SHA on `main`.

You can also skip the action entirely. Run the release
binary in a `run:` step. That repeats by hand the download
and verify steps the action automates.

While the listing is pending, the channel stays hidden. It
sets `unlisted: true`. `sync-channels` then keeps it out of
the install picker and the "Available on" strip. The
install-guide and release-pipeline tables exclude it by
glob. Once the listing resolves, flip `unlisted`, drop both
glob exclusions, and set `command` to that release's tag.
