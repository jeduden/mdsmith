---
title: uvx
summary: >-
  `uvx mdsmith` runs the PyPI wheel through uv with no
  persistent install, execing the bundled binary from
  uv's cache.
mechanism: toolchain
artifact: cli
command: uvx mdsmith check .
audience: Ephemeral runs via uv
platforms: [python]
channelurl: https://pypi.org/project/mdsmith/
weight: 5
---
# uvx

Release page: <https://pypi.org/project/mdsmith/>

`uvx` runs the mdsmith wheel through uv without a
persistent install. uv resolves the platform wheel,
caches it, and execs the bundled binary. The run
leaves no global state behind. Pin a version with
`uvx mdsmith@X.Y.Z`.
