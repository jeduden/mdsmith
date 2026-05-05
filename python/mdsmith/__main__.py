"""Entry point for ``python -m mdsmith`` and the ``mdsmith`` console
script. Locates the prebuilt binary inside the installed wheel and
execs it with the user's argv. The wheel is platform-tagged, so a
matching binary is guaranteed to exist on every supported host.
"""

from __future__ import annotations

import os
import sys
from pathlib import Path


def binary_path() -> Path:
    """Return the absolute path of the bundled mdsmith binary.

    Wheels ship the binary under ``mdsmith/_bin/mdsmith`` (or
    ``mdsmith/_bin/mdsmith.exe`` on Windows). Raise ``FileNotFoundError``
    if the wheel was unpacked outside the expected layout — that
    only happens when the wrong wheel was installed for the host
    platform, in which case ``pip install`` should have refused.
    """
    here = Path(__file__).resolve().parent
    name = "mdsmith.exe" if os.name == "nt" else "mdsmith"
    candidate = here / "_bin" / name
    if not candidate.is_file():
        raise FileNotFoundError(
            f"mdsmith: bundled binary not found at {candidate}. "
            "The installed wheel does not match this platform; "
            "reinstall mdsmith and ensure pip picks the right wheel "
            "(see https://github.com/jeduden/mdsmith/releases for "
            "direct downloads)."
        )
    return candidate


def main() -> None:
    """Exec the bundled binary with this process's argv.

    On POSIX platforms ``os.execv`` replaces the current process so
    signal handling, stdout/stderr buffering, and exit codes match
    a direct invocation. Windows has no ``execv`` semantics, so
    ``subprocess.run`` is used and its exit code is propagated.
    """
    bin_path = binary_path()
    argv = [str(bin_path), *sys.argv[1:]]
    if os.name == "nt":
        import subprocess

        result = subprocess.run(argv, check=False)
        raise SystemExit(result.returncode)
    os.execv(str(bin_path), argv)


if __name__ == "__main__":
    main()
