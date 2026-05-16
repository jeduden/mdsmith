---
title: PyPI
summary: >-
  One platform-tagged wheel per supported host,
  published via OIDC Trusted Publishing.
registry: pypi.org
credential: OIDC Trusted Publishing
job: pypi
channelurl: https://pypi.org/project/mdsmith/
weight: 2
---
# PyPI

Release page: <https://pypi.org/project/mdsmith/>

The PyPI channel publishes one wheel per supported
host. Platform tags:

- `manylinux_2_17_x86_64`
- `manylinux_2_17_aarch64`
- `macosx_*_x86_64`
- `macosx_*_arm64`
- `win_amd64`

Each wheel bundles the prebuilt mdsmith binary
under `mdsmith/_bin/`. An `mdsmith` console script
execs the binary in place: `os.execv` on POSIX,
`subprocess.run` on Windows.

The package is wheel-only — no source distribution.
That means `pip install mdsmith` never runs Python
build code. It also never invokes a compiler on the
user's host.

The `pypi` job in `release.yml` uses
`pypa/gh-action-pypi-publish`. The action exchanges
the workflow's GitHub OIDC token for a short-lived
PyPI upload credential. Configure the trusted
publisher on PyPI before the first tag. See the
`OIDC Trusted Publishing` section in
[`release.md`](../release.md).
