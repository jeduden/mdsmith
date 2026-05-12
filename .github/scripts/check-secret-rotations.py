#!/usr/bin/env python3
"""Open a GitHub issue when a long-lived secret is due for rotation.

The source of truth is the `rotations:` table in the front matter of
`docs/development/secret-rotations.md`. For each entry the script
computes (today - last-rotated). If that exceeds period-days minus
the reminder window (30 days), the script opens a single open issue
per secret with a stable title so reruns are idempotent.

The script does not close issues. After rotation the human edits the
`last-rotated` date in the doc, merges the change, and closes the
issue. The next scheduled run sees the new date and waits.

Exit codes:
- 0: ran cleanly (regardless of whether any issue was opened)
- 1: doc missing / malformed / `gh` failure / unknown secret kind
"""

from __future__ import annotations

import datetime as dt
import json
import os
import subprocess
import sys
from pathlib import Path

import yaml

REPO_ROOT = Path(__file__).resolve().parents[2]
ROTATION_DOC = REPO_ROOT / "docs" / "development" / "secret-rotations.md"
REMINDER_WINDOW_DAYS = 30
ISSUE_LABEL = "secret-rotation"
ASSIGNEE = "jeduden"


def _front_matter(path: Path) -> dict:
    """Return the YAML front-matter block of a markdown file."""
    text = path.read_text(encoding="utf-8")
    if not text.startswith("---\n"):
        raise SystemExit(f"{path}: no front matter (must start with '---\\n')")
    end = text.find("\n---\n", 4)
    if end == -1:
        raise SystemExit(f"{path}: unterminated front matter")
    parsed = yaml.safe_load(text[4:end])
    if not isinstance(parsed, dict):
        raise SystemExit(f"{path}: front matter is not a mapping")
    return parsed


def _gh(args: list[str]) -> str:
    """Run `gh` and return stdout. Raise SystemExit on non-zero."""
    proc = subprocess.run(
        ["gh", *args], capture_output=True, text=True, check=False
    )
    if proc.returncode != 0:
        sys.stderr.write(
            f"gh {' '.join(args)} failed (exit {proc.returncode}):\n{proc.stderr}"
        )
        raise SystemExit(1)
    return proc.stdout


def _existing_open_issue(title: str) -> int | None:
    """Return the number of an open issue whose title exactly matches."""
    out = _gh(
        [
            "issue",
            "list",
            "--state",
            "open",
            "--label",
            ISSUE_LABEL,
            "--json",
            "number,title",
            "--limit",
            "100",
        ]
    )
    for issue in json.loads(out or "[]"):
        if issue["title"] == title:
            return int(issue["number"])
    return None


def _due_state(today: dt.date, last_rotated: dt.date, period_days: int) -> tuple[str, int]:
    """Return ("ok"|"due"|"overdue", days_until_due).

    The reminder window is REMINDER_WINDOW_DAYS days before the period
    elapses. Anything past the period is overdue.
    """
    due_on = last_rotated + dt.timedelta(days=period_days)
    days = (due_on - today).days
    if days < 0:
        return "overdue", days
    if days <= REMINDER_WINDOW_DAYS:
        return "due", days
    return "ok", days


def _issue_body(entry: dict, status: str, days: int) -> str:
    """Compose the issue body. Wrapped in triple-quoted f-string."""
    name = entry["name"]
    last = entry["last-rotated"]
    period = entry["period-days"]
    provider = entry["provider"]
    issuer_url = entry["issuer-url"]
    used_by = entry["used-by"]
    scope = entry["scope"]
    if status == "overdue":
        headline = f"`{name}` is OVERDUE by {-days} days."
    else:
        headline = f"`{name}` is due in {days} days."
    return f"""\
{headline}

| Field         | Value                                            |
|---------------|--------------------------------------------------|
| Provider      | {provider}                                       |
| Issuer URL    | <{issuer_url}>                                   |
| Used by       | {used_by}                                        |
| Scope         | {scope}                                          |
| Last rotated  | {last}                                           |
| Period (days) | {period}                                         |

The rotation procedure for `{name}` lives in
[`docs/development/secret-rotations.md`](../blob/main/docs/development/secret-rotations.md#{name.lower().replace('_', '-')}).

After rotation:

1. Update the `last-rotated` field for `{name}` in the front matter
   of `docs/development/secret-rotations.md`. Merge the change.
2. Close this issue.

This reminder is opened automatically by
`.github/workflows/secret-rotation-reminder.yml`.
"""


def main() -> int:
    fm = _front_matter(ROTATION_DOC)
    rotations = fm.get("rotations", [])
    if not isinstance(rotations, list) or not rotations:
        raise SystemExit(
            f"{ROTATION_DOC}: front matter has no `rotations:` list"
        )

    today = dt.date.today()
    opened: list[str] = []
    skipped: list[str] = []

    for entry in rotations:
        for key in ("name", "last-rotated", "period-days", "provider", "issuer-url", "used-by", "scope"):
            if key not in entry:
                raise SystemExit(
                    f"rotation entry missing `{key}`: {entry!r}"
                )
        name = entry["name"]
        last_rotated = dt.date.fromisoformat(str(entry["last-rotated"]))
        period_days = int(entry["period-days"])
        status, days = _due_state(today, last_rotated, period_days)
        if status == "ok":
            continue
        title = f"Rotate {name} (last rotated {last_rotated.isoformat()})"
        if _existing_open_issue(title) is not None:
            skipped.append(name)
            continue
        body = _issue_body(entry, status, days)
        _gh(
            [
                "issue",
                "create",
                "--title",
                title,
                "--body",
                body,
                "--label",
                ISSUE_LABEL,
                "--assignee",
                ASSIGNEE,
            ]
        )
        opened.append(name)

    if opened:
        sys.stdout.write(
            f"opened secret-rotation reminders for: {', '.join(opened)}\n"
        )
    if skipped:
        sys.stdout.write(
            f"existing open reminders (skipped): {', '.join(skipped)}\n"
        )
    if not opened and not skipped:
        sys.stdout.write("no secrets due for rotation\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
