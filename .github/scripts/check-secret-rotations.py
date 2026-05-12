#!/usr/bin/env python3
"""Open a GitHub issue when a long-lived secret is due for rotation.

The source of truth is the `rotations:` table in the front matter of
`docs/development/secret-rotations.md`. For each entry the script
computes (today - last-rotated). If that exceeds period-days minus
the reminder window (30 days), the script opens a single open issue
per secret with a stable title so reruns are idempotent.

The script does not close issues directly. The reminder issue
body points the maintainer at the `Record Secret Rotation`
workflow (`.github/workflows/record-secret-rotation.yml`), which
opens a PR that updates `last-rotated` and includes `Closes #N`.
Merging that PR records the rotation date AND closes this
reminder in one step. The next scheduled run sees the new date
and stays quiet until the next expiry window.

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


def _repo_url() -> str:
    """Return the absolute GitHub URL of this repository.

    GitHub Actions runners always populate GITHUB_SERVER_URL (e.g.
    https://github.com) and GITHUB_REPOSITORY (e.g. owner/name).
    Falling back to a hard-coded github.com URL keeps local runs
    (e.g. via `gh workflow run --workflow=...`) working when the
    envvars are absent.
    """
    server = os.environ.get("GITHUB_SERVER_URL", "https://github.com").rstrip("/")
    repo = os.environ.get("GITHUB_REPOSITORY", "jeduden/mdsmith")
    return f"{server}/{repo}"


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


def _ensure_label() -> None:
    """Idempotently create the `secret-rotation` label.

    The first scheduled run of this script in a fresh repo would 422
    on `gh issue create --label ...` because the label does not yet
    exist. `gh label create --force` creates-or-updates the label so
    subsequent issue creation always succeeds. Subsequent runs are
    no-ops (the label just gets re-described, which is fine).
    """
    _gh(
        [
            "label",
            "create",
            ISSUE_LABEL,
            "--force",
            "--color",
            "C5DEF5",
            "--description",
            "Long-lived secret is due (or overdue) for rotation",
        ]
    )


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
    repo_url = _repo_url()
    # Link to the doc without a heading anchor: GitHub's slug rules
    # (lowercase, hyphenated, with the em-dash stripped) do not match
    # what a naive name-to-slug derivation produces, so an anchored
    # link would 404 mid-page. The doc is short — a reader scrolling
    # to the `{name}` section finds it instantly.
    doc_url = f"{repo_url}/blob/main/docs/development/secret-rotations.md"
    reminder_url = f"{repo_url}/blob/main/.github/workflows/secret-rotation-reminder.yml"
    record_url = f"{repo_url}/actions/workflows/record-secret-rotation.yml"
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

Rotation procedure for the `{name}` section:
{doc_url}

After rotating the credential at the issuer, do not
hand-edit the front matter or close this issue.
Instead, run the **Record Secret Rotation** workflow:
{record_url}

Pick `{name}` from the dropdown and click `Run workflow`.
The workflow opens a PR that updates `last-rotated`
and includes `Closes #` referencing this issue, so
the merge both records the rotation and closes this
reminder in one step.

This reminder was opened automatically by {reminder_url}.
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
    label_ensured = False

    for entry in rotations:
        if not isinstance(entry, dict):
            raise SystemExit(
                f"rotation entry is not a mapping: {entry!r}"
            )
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
        if not label_ensured:
            # Bootstrap the label on first issue creation. Skipping
            # this would 422 on a fresh repo where the label has not
            # been created yet.
            _ensure_label()
            label_ensured = True
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
