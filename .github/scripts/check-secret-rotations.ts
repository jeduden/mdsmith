#!/usr/bin/env bun
/**
 * Open a GitHub issue when a long-lived secret is due for rotation.
 *
 * Each tracked secret lives in its own file under
 * `docs/development/secret-rotations/`. The script globs that
 * directory, parses each file's YAML front matter, and computes
 * (today - lastRotated). If the elapsed time exceeds
 * `periodDays` minus the reminder window (30 days), the script
 * opens a single labelled issue per file with a stable title so
 * reruns are idempotent.
 *
 * The script does not close issues directly. The reminder issue
 * body points the maintainer at the `Record Secret Rotation`
 * workflow, which opens a PR that updates `lastRotated` and
 * includes `Closes #N`. Merging that PR records the rotation date
 * AND closes the reminder in one step. The next scheduled run
 * sees the new date and stays quiet until the next expiry window.
 *
 * Exit codes:
 * - 0: ran cleanly (regardless of whether any issue was opened)
 * - 1: doc missing / malformed / `gh` failure / unknown entry kind
 */

import { $, Glob } from "bun";
import { basename, resolve } from "node:path";
import { parse as yamlParse } from "yaml";

const REPO_ROOT = resolve(import.meta.dir, "..", "..");
const ROTATIONS_DIR = resolve(REPO_ROOT, "docs/development/secret-rotations");
const REMINDER_WINDOW_DAYS = 30;
const ISSUE_LABEL = "secret-rotation";
// Optional issue assignee. Read from REMINDER_ASSIGNEE so the
// workflow can override it explicitly without hardcoding a name
// in this file. The empty default skips the `--assignee` flag
// entirely on `gh issue create`: assignment is best-effort
// (newly-opened issues without an assignee are still visible to
// the secret-rotation label subscribers), and an unassignable
// value (e.g. an org login that GitHub rejects) would otherwise
// fail the entire create call. The workflow sets
// REMINDER_ASSIGNEE to `${{ github.repository_owner }}` by
// default; setting it to the empty string disables assignment.
const ASSIGNEE = process.env.REMINDER_ASSIGNEE ?? "";

interface RotationEntry {
  title: string;
  lastRotated: string;
  periodDays: number;
  provider: string;
  issuerUrl: string;
  usedBy: string;
  scope: string;
}

export class SystemExit extends Error {}

/** Validate that `s` is a real calendar date in YYYY-MM-DD form.
 * Round-trips the parsed components so normalized invalid dates
 * (e.g. `2026-02-31` parsing to March 3) are rejected. */
export function isIsoDate(s: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(s)) return false;
  const parsed = new Date(`${s}T00:00:00Z`);
  if (Number.isNaN(parsed.getTime())) return false;
  const year = String(parsed.getUTCFullYear()).padStart(4, "0");
  const month = String(parsed.getUTCMonth() + 1).padStart(2, "0");
  const day = String(parsed.getUTCDate()).padStart(2, "0");
  return `${year}-${month}-${day}` === s;
}

/** Compute today's date in UTC. The cron schedule is UTC, so the
 * computed due-state must match the workflow's wall clock. */
export function utcToday(): Date {
  const now = new Date();
  return new Date(Date.UTC(
    now.getUTCFullYear(),
    now.getUTCMonth(),
    now.getUTCDate(),
  ));
}

/** Days between two UTC midnights, truncated to integer. */
export function daysBetween(later: Date, earlier: Date): number {
  const msPerDay = 24 * 60 * 60 * 1000;
  return Math.floor((later.getTime() - earlier.getTime()) / msPerDay);
}

/** Extract the YAML front matter block from a markdown file. */
export function parseFrontMatter(text: string, path: string): Record<string, unknown> {
  if (!text.startsWith("---\n")) {
    throw new SystemExit(`${path}: no front matter (must start with '---\\n')`);
  }
  const end = text.indexOf("\n---\n", 4);
  if (end === -1) {
    throw new SystemExit(`${path}: unterminated front matter`);
  }
  const parsed = yamlParse(text.slice(4, end));
  if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new SystemExit(`${path}: front matter is not a mapping`);
  }
  return parsed as Record<string, unknown>;
}

export type DueState = "ok" | "due" | "overdue";

/** A signed days-until-due. Positive while the rotation is still
 * in the future, zero on the due date, negative once past due.
 * Callers format the value differently per status: "due in N
 * days" / "due today" / "OVERDUE by N days" (negate). */
export interface DueResult {
  status: DueState;
  daysUntilDue: number;
}

export function dueState(today: Date, lastRotated: Date, periodDays: number): DueResult {
  const dueOn = new Date(lastRotated);
  dueOn.setUTCDate(dueOn.getUTCDate() + periodDays);
  const daysUntilDue = daysBetween(dueOn, today);
  if (daysUntilDue < 0) return { status: "overdue", daysUntilDue };
  if (daysUntilDue <= REMINDER_WINDOW_DAYS) return { status: "due", daysUntilDue };
  return { status: "ok", daysUntilDue };
}

/** Return the absolute GitHub URL of this repository. */
export function repoUrl(): string {
  const server = (process.env.GITHUB_SERVER_URL ?? "https://github.com").replace(/\/+$/, "");
  const repo = process.env.GITHUB_REPOSITORY ?? "jeduden/mdsmith";
  return `${server}/${repo}`;
}

interface OpenIssue { number: number; title: string }

async function existingOpenIssue(title: string): Promise<number | null> {
  // Narrow the candidate set server-side via GitHub search.
  // `in:title "<phrase>"` matches issues whose title contains the
  // quoted phrase, so a repo with hundreds of unrelated
  // secret-rotation-labelled issues never pushes the real match
  // past the --limit ceiling. The exact-string check below catches
  // GitHub search's tokenized/fuzzy behavior — we only treat the
  // issue as existing if its title is byte-for-byte identical.
  const out = await $`gh issue list \
    --state open \
    --label ${ISSUE_LABEL} \
    --search ${`in:title "${title.replace(/"/g, "")}"`} \
    --json number,title \
    --limit 100`.text();
  const issues = JSON.parse(out || "[]") as OpenIssue[];
  for (const issue of issues) {
    if (issue.title === title) return issue.number;
  }
  return null;
}

let labelEnsured = false;
async function ensureLabel(): Promise<void> {
  if (labelEnsured) return;
  await $`gh label create ${ISSUE_LABEL} \
    --force \
    --color C5DEF5 \
    --description ${"Long-lived secret is due (or overdue) for rotation"}`.quiet();
  labelEnsured = true;
}

export function issueBody(entry: RotationEntry, fileBasename: string, due: DueResult): string {
  let headline: string;
  if (due.status === "overdue") {
    // daysUntilDue is negative once past due; negate for the
    // English "OVERDUE by N days" reading.
    headline = `\`${entry.title}\` is OVERDUE by ${-due.daysUntilDue} days.`;
  } else if (due.daysUntilDue === 0) {
    headline = `\`${entry.title}\` is due today.`;
  } else {
    headline = `\`${entry.title}\` is due in ${due.daysUntilDue} days.`;
  }
  const fileUrl = `${repoUrl()}/blob/main/docs/development/secret-rotations/${fileBasename}`;
  const reminderUrl = `${repoUrl()}/blob/main/.github/workflows/secret-rotation-reminder.yml`;
  const recordUrl = `${repoUrl()}/actions/workflows/record-secret-rotation.yml`;
  // Table padding is intentionally minimal: GitHub-flavored
  // markdown collapses extra whitespace, so visually aligning
  // columns in the source has no effect on the rendered issue
  // and just creates noisy diffs whenever a value's length
  // changes.
  return [
    `${headline}`,
    ``,
    `| Field | Value |`,
    `|---|---|`,
    `| Provider | ${entry.provider} |`,
    `| Issuer URL | <${entry.issuerUrl}> |`,
    `| Used by | ${entry.usedBy} |`,
    `| Scope | ${entry.scope} |`,
    `| lastRotated | ${entry.lastRotated} |`,
    `| Period (days) | ${entry.periodDays} |`,
    ``,
    `Rotation procedure:`,
    `${fileUrl}`,
    ``,
    `After rotating the credential at the issuer, do not`,
    `hand-edit the front matter or close this issue.`,
    `Instead, run the **Record Secret Rotation** workflow:`,
    `${recordUrl}`,
    ``,
    `Pick \`${entry.title}\` from the dropdown and click \`Run workflow\`.`,
    `The workflow opens a PR that updates \`lastRotated\``,
    `and includes \`Closes #\` referencing this issue, so`,
    `the merge both records the rotation and closes this`,
    `reminder in one step.`,
    ``,
    `This reminder was opened automatically by ${reminderUrl}.`,
    "",
  ].join("\n");
}

const REQUIRED_FRONT_MATTER_KEYS: (keyof RotationEntry)[] = [
  "title", "lastRotated", "periodDays", "provider", "issuerUrl", "usedBy", "scope",
];

/** Validate a single rotation entry's front matter and project it
 * into a `RotationEntry`. Throws `SystemExit` with a path-prefixed
 * message on any structural failure. Separated from `loadRotations`
 * so the validation rules are reachable from unit tests without
 * filesystem fixtures. */
export function validateRotationEntry(fm: Record<string, unknown>, path: string): RotationEntry {
  for (const key of REQUIRED_FRONT_MATTER_KEYS) {
    if (!(key in fm)) {
      throw new SystemExit(`${path}: front matter missing \`${key}\``);
    }
  }
  const lastStr = String(fm.lastRotated);
  if (!isIsoDate(lastStr)) {
    throw new SystemExit(
      `${path}: \`lastRotated\` is not a valid ISO-8601 date (${JSON.stringify(lastStr)})`,
    );
  }
  const periodDays = Number(fm.periodDays);
  if (!Number.isInteger(periodDays)) {
    throw new SystemExit(
      `${path}: \`periodDays\` is not an integer (${JSON.stringify(fm.periodDays)})`,
    );
  }
  // A zero or negative period would compute a due date on or
  // before lastRotated, so every run would treat the secret as
  // overdue and the reminder workflow would never go quiet.
  // Reject the value at load time with a clear pointer to the
  // bad file rather than silently spamming issues.
  if (periodDays <= 0) {
    throw new SystemExit(
      `${path}: \`periodDays\` must be a positive integer (got ${periodDays})`,
    );
  }
  return {
    title: String(fm.title),
    lastRotated: lastStr,
    periodDays,
    provider: String(fm.provider),
    issuerUrl: String(fm.issuerUrl),
    usedBy: String(fm.usedBy),
    scope: String(fm.scope),
  };
}

/** Load every per-secret rotation file from ROTATIONS_DIR. */
async function loadRotations(): Promise<{ entry: RotationEntry; fileBasename: string }[]> {
  const glob = new Glob("*.md");
  const entries: { entry: RotationEntry; fileBasename: string }[] = [];
  for await (const rel of glob.scan({ cwd: ROTATIONS_DIR })) {
    const fileBasename = basename(rel);
    const path = resolve(ROTATIONS_DIR, rel);
    const text = await Bun.file(path).text();
    const fm = parseFrontMatter(text, path);
    entries.push({ entry: validateRotationEntry(fm, path), fileBasename });
  }
  // Sort for deterministic iteration order regardless of FS ordering.
  entries.sort((a, b) => a.entry.title.localeCompare(b.entry.title));
  return entries;
}

async function main(): Promise<number> {
  const rotations = await loadRotations();
  if (rotations.length === 0) {
    throw new SystemExit(`${ROTATIONS_DIR}: no per-secret files found`);
  }

  const today = utcToday();
  const opened: string[] = [];
  const skipped: string[] = [];

  for (const { entry, fileBasename } of rotations) {
    const lastRotated = new Date(`${entry.lastRotated}T00:00:00Z`);
    const due = dueState(today, lastRotated, entry.periodDays);
    if (due.status === "ok") continue;
    const title = `Rotate ${entry.title} (lastRotated ${entry.lastRotated})`;
    if (await existingOpenIssue(title) !== null) {
      skipped.push(entry.title);
      continue;
    }
    await ensureLabel();
    const body = issueBody(entry, fileBasename, due);
    // Omit --assignee when ASSIGNEE is empty: passing an empty
    // string would either error or assign nobody depending on
    // gh version. Issue creation must succeed even when the
    // workflow chooses not to assign anyone (e.g. the repo moved
    // under an org login that GitHub rejects as an assignee).
    if (ASSIGNEE) {
      await $`gh issue create \
        --title ${title} \
        --body ${body} \
        --label ${ISSUE_LABEL} \
        --assignee ${ASSIGNEE}`.quiet();
    } else {
      await $`gh issue create \
        --title ${title} \
        --body ${body} \
        --label ${ISSUE_LABEL}`.quiet();
    }
    opened.push(entry.title);
  }

  if (opened.length) {
    console.log(`opened secret-rotation reminders for: ${opened.join(", ")}`);
  }
  if (skipped.length) {
    console.log(`existing open reminders (skipped): ${skipped.join(", ")}`);
  }
  if (!opened.length && !skipped.length) {
    console.log("no secrets due for rotation");
  }
  return 0;
}

// Gate the auto-run on `import.meta.main` so test files that
// import this module for its pure exports do not also fire the
// shell-out paths in `main()` as a side effect of the import.
if (import.meta.main) {
  try {
    process.exit(await main());
  } catch (err) {
    if (err instanceof SystemExit) {
      process.stderr.write(`${err.message}\n`);
      process.exit(1);
    }
    throw err;
  }
}
