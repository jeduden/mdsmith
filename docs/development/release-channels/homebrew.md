---
title: Homebrew
summary: >-
  The `jeduden/homebrew-mdsmith` tap installs the
  checksum-verified prebuilt binary for macOS or Linux
  on Intel or arm64.
mechanism: pull
artifact: cli
command: brew install jeduden/mdsmith/mdsmith
audience: macOS and Linux via Homebrew
platforms: [macos, linux]
channelurl: https://github.com/jeduden/homebrew-mdsmith
weight: 7
---
# Homebrew

Release page: <https://github.com/jeduden/homebrew-mdsmith>

The `jeduden/homebrew-mdsmith` tap installs the
prebuilt binary for macOS or Linux, on Intel or
arm64. Each formula verifies the download against the
release `checksums.txt`. Upgrade with
`brew upgrade mdsmith`. The `notify-homebrew-tap` job
in `release.yml` dispatches a bump on each release.
