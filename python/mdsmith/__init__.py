"""mdsmith — Markdown linter / formatter (Python distribution).

This package is a thin wrapper around the prebuilt mdsmith Go binary
that ships inside each platform-specific wheel under
``mdsmith/_bin/``. Importing the package does not start the binary;
:func:`mdsmith.binary_path` returns its filesystem path and
:func:`mdsmith.main` execs it with the user's argv.
"""

from .__main__ import binary_path, main

__all__ = ["binary_path", "main"]
