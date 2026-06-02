---
title: PyPI
summary: >-
  One platform-tagged wheel per supported host,
  published via OIDC Trusted Publishing.
mechanism: push
artifact: cli
command: pip install mdsmith
audience: Python projects and Python-only CI images
platforms: [python]
registry: pypi.org
credential: OIDC Trusted Publishing
job: pypi
channelurl: https://pypi.org/project/mdsmith/
weight: 4
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

## `pyproject.toml` rationale

`mdsmith-release sync-messaging` rewrites the
`[project].description` field. It pulls the value
from the source of truth at
[`docs/brand/messaging.md`](../../brand/messaging.md).
The sync re-emits the file through a TOML library.
The file does not carry inline rationale comments.
The two comments worth keeping live here.

### `license-files = ["LICENSE"]`

PEP 639 and hatchling. Ship the root `LICENSE`
inside the wheel so the MIT notice reaches PyPI
consumers. The notice also carries third-party
attribution for `internal/punkt/`, vendored from
neurosnap/sentences.

The release tooling stages `/LICENSE` next to
`pyproject.toml` before `python -m build`. See
`stagePythonTree` in
[`internal/release/buildwheels.go`](../../../internal/release/buildwheels.go).

### `tool.hatch.build.targets.wheel.artifacts`

`cmd/mdsmith-release build-wheels` stages the
prebuilt binary at `mdsmith/_bin/mdsmith[.exe]`.
The stage happens before `python -m build`. The
file is not checked in. Hatchling drops it
otherwise. `artifacts` lists both possible binary
names. The wheel contains whichever one
`build-wheels` staged.
