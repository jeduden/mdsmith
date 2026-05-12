#!/usr/bin/env python3
"""Update the `last-rotated` date for a secret in the front matter.

Invoked from `.github/workflows/record-secret-rotation.yml` after the
maintainer has rotated a credential at its issuer. Writes the new
date into `docs/development/secret-rotations.md`, in place. The
workflow then commits the result on a fresh branch and opens a PR
so CODEOWNERS and the mdsmith lint job both gate the change.

Usage:
    python3 record-rotation.py <SECRET_NAME> <YYYY-MM-DD>

The argument is the secret's name (e.g. `VSCE_PAT`), never its
value. The script never reads, prints, or stores credential
material — `entry_name` flows only as a lookup key for the
rotations table.

Exit codes:
- 0: success — front matter updated, or the date was already set
  to the requested value (no-op)
- 1: doc malformed, name not found, or date not valid ISO-8601
"""

from __future__ import annotations

import datetime as dt
import re
import sys
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parents[2]
ROTATION_DOC = REPO_ROOT / "docs" / "development" / "secret-rotations.md"


def _split_front_matter(text: str) -> tuple[str, str, str]:
    """Return (leading_marker, front_matter_yaml, rest_of_doc)."""
    if not text.startswith("---\n"):
        raise SystemExit(f"{ROTATION_DOC}: no front matter")
    end = text.find("\n---\n", 4)
    if end == -1:
        raise SystemExit(f"{ROTATION_DOC}: unterminated front matter")
    return text[:4], text[4:end], text[end:]


def _update_last_rotated(yaml_block: str, entry_name: str, date: str) -> str:
    """Rewrite the YAML to set last-rotated for `entry_name` to `date`.

    The structural rewrite uses pyyaml to validate that `entry_name`
    exists in the rotations list. The actual textual edit uses a
    regex on the source so unrelated formatting (key order, quoting
    style, comments) is preserved.
    """
    parsed = yaml.safe_load(yaml_block)
    rotations = parsed.get("rotations") if isinstance(parsed, dict) else None
    if not isinstance(rotations, list):
        raise SystemExit("front matter has no `rotations:` list")
    matched = next(
        (r for r in rotations if isinstance(r, dict) and r.get("name") == entry_name),
        None,
    )
    if matched is None:
        names = ", ".join(
            r.get("name", "?") for r in rotations if isinstance(r, dict)
        )
        raise SystemExit(f"unknown name {entry_name!r}; known: {names}")

    # Match the YAML block that begins with `- name: <SECRET_NAME>`
    # and rewrite the `last-rotated:` line directly under it.
    # Captures:
    #   group(1): the `- name: NAME\n` line + indented preamble
    #   group(2): the value of last-rotated
    pattern = re.compile(
        r"(- name:\s*" + re.escape(entry_name) + r"\b[^\n]*\n"  # name line
        r"(?:[ \t]+[^\n]*\n)*?"                                  # other keys before last-rotated
        r"[ \t]+last-rotated:\s*)"                               # last-rotated key
        r'("?[^"\n]*"?)'                                         # current value (quoted or bare)
    )
    new_block, count = pattern.subn(rf'\g<1>"{date}"', yaml_block, count=1)
    if count == 0:
        raise SystemExit(
            f"could not locate `last-rotated:` line under `- name: {entry_name}`"
        )
    return new_block


def main(argv: list[str]) -> int:
    if len(argv) != 3:
        sys.stderr.write("usage: record-rotation.py <SECRET_NAME> <YYYY-MM-DD>\n")
        return 1
    entry_name, date_str = argv[1], argv[2]
    try:
        dt.date.fromisoformat(date_str)
    except ValueError as exc:
        sys.stderr.write(f"invalid date {date_str!r}: {exc}\n")
        return 1
    text = ROTATION_DOC.read_text(encoding="utf-8")
    marker, fm_yaml, rest = _split_front_matter(text)
    updated = _update_last_rotated(fm_yaml, entry_name, date_str)
    if updated == fm_yaml:
        sys.stdout.write(f"{entry_name} already at {date_str}; no change\n")
        return 0
    ROTATION_DOC.write_text(marker + updated + rest, encoding="utf-8")
    sys.stdout.write(f"updated {entry_name}.last-rotated -> {date_str}\n")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
