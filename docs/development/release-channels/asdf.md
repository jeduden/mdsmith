---
title: asdf
summary: >-
  The `jeduden/asdf-mdsmith` plugin installs the
  checksum-verified prebuilt binary; the short form
  awaits the asdf-plugins registry entry.
mechanism: pull
artifact: cli
command: asdf plugin add mdsmith https://github.com/jeduden/asdf-mdsmith.git
audience: Repos standardized on asdf
platforms: [macos, linux]
channelurl: https://github.com/jeduden/asdf-mdsmith
weight: 9
---
# asdf

Release page: <https://github.com/jeduden/asdf-mdsmith>

The `jeduden/asdf-mdsmith` plugin lists released
versions and installs the prebuilt binary for the
host platform. It verifies the download against the
release `checksums.txt`. The explicit-URL form works
today. The short `asdf plugin add mdsmith` form needs
the registry entry tracked in
[plan/145](../../../plan/145_asdf-mise-registry-submissions.md).
