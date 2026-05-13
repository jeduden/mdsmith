#!/usr/bin/env bun
/**
 * Open a GitHub issue when a long-lived secret is due for rotation.
 *
 * Each tracked secret lives in its own file under
 * `docs/development/secret-rotations/`. The script globs that
 * directory, parses each file's YAML front matter, and computes
 * (today - last-rotated). If the elapsed time exceeds
 * `period-days` minus the reminder window (30 days), the script
 * opens a single labelled issue per file with a stable title so
 * reruns are idempotent.
 *
 * The script does not close issues directly. The reminder issue
 * body points the maintainer at the `Record Secret Rotation`
 * workflow, which opens a PR that updates `last-rotated` and
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
const ASSIGNEE = "jeduden";

interface RotationEntry {
  title: string;
  lastRotated: string;
  periodDays: number;
  provider: string;
  issuerUrl: string;
  usedBy: string;
  scope: string;
}

class SystemExit extends Error {}

/** Compute today's date in UTC. The cron schedule is UTC, so the
 * computed due-state must match the workflow's wall clock. */
function utcToday(): Date {
  const now = new Date();
  return new Date(Date.UTC(
    now.getUTCFullYear(),
    now.getUTCMonth(),
    now.getUTCDate(),
  ));
}

/** Days between two UTC midnights, truncated to integer. */
function daysBetween(later: Date, earlier: Date): number {
  const msPerDay = 24 * 60 * 60 * 1000;
  return Math.floor((later.getTime() - earlier.getTime()) / msPerDay);
}

/** Extract the YAML front matter block from a markdown file. */
function parseFrontMatter(text: string, path: string): Record<string, unknown> {
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

type DueState = "ok" | "due" | "overdue";

function dueState(today: Date, lastRotated: Date, periodDays: number): { status: DueState; days: number } {
  const dueOn = new Date(lastRotated);
  dueOn.setUTCDate(dueOn.getUTCDate() + periodDays);
  // daysBetween(dueOn, today) is positive when dueOn is still in
  // the future, zero on the due date, negative once past due.
  const days = daysBetween(dueOn, today);
  if (days < 0) return { status: "overdue", days };
  if (days <= REMINDER_WINDOW_DAYS) return { status: "due", days };
  return { status: "ok", days };
}

/** Return the absolute GitHub URL of this repository. */
function repoUrl(): string {
  const server = (process.env.GITHUB_SERVER_URL ?? "https://github.com").replace(/\/+$/, "");
  const repo = process.env.GITHUB_REPOSITORY ?? "jeduden/mdsmith";
  return `${server}/${repo}`;
}

interface OpenIssue { number: number; title: string }

async function existingOpenIssue(title: string): Promise<number | null> {
  const out = await $`gh issue list \
    --state open \
    --label ${ISSUE_LABEL} \
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

function issueBody(entry: RotationEntry, fileBasename: string, status: DueState, days: number): string {
  const headline = status === "overdue"
    ? `\`${entry.title}\` is OVERDUE by ${-days} days.`
    : `\`${entry.title}\` is due in ${days} days.`;
  const fileUrl = `${repoUrl()}/blob/main/docs/development/secret-rotations/${fileBasename}`;
  const reminderUrl = `${repoUrl()}/blob/main/.github/workflows/secret-rotation-reminder.yml`;
  const recordUrl = `${repoUrl()}/actions/workflows/record-secret-rotation.yml`;
  return [
    `${headline}`,
    ``,
    `| Field         | Value                                            |`,
    `|---------------|--------------------------------------------------|`,
    `| Provider      | ${entry.provider}                                       |`,
    `| Issuer URL    | <${entry.issuerUrl}>                                   |`,
    `| Used by       | ${entry.usedBy}                                        |`,
    `| Scope         | ${entry.scope}                                          |`,
    `| Last rotated  | ${entry.lastRotated}                                           |`,
    `| Period (days) | ${entry.periodDays}                                         |`,
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
    `The workflow opens a PR that updates \`last-rotated\``,
    `and includes \`Closes #\` referencing this issue, so`,
    `the merge both records the rotation and closes this`,
    `reminder in one step.`,
    ``,
    `This reminder was opened automatically by ${reminderUrl}.`,
    "",
  ].join("\n");
}

/** Load every per-secret rotation file from ROTATIONS_DIR. */
async function loadRotations(): Promise<{ entry: RotationEntry; fileBasename: string }[]> {
  const glob = new Glob("*.md");
  const required: (keyof RotationEntry)[] = [
    "title", "lastRotated", "periodDays", "provider", "issuerUrl", "usedBy", "scope",
  ];
  const entries: { entry: RotationEntry; fileBasename: string }[] = [];
  for await (const rel of glob.scan({ cwd: ROTATIONS_DIR })) {
    const fileBasename = basename(rel);
    const path = resolve(ROTATIONS_DIR, rel);
    const text = await Bun.file(path).text();
    const fm = parseFrontMatter(text, path);
    for (const key of required) {
      if (!(key in fm)) {
        throw new SystemExit(`${path}: front matter missing \`${key}\``);
      }
    }
    const title = String(fm.title);
    const lastStr = String(fm.lastRotated);
    if (!/^\d{4}-\d{2}-\d{2}$/.test(lastStr)) {
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
    entries.push({
      entry: {
        title,
        lastRotated: lastStr,
        periodDays,
        provider: String(fm.provider),
        issuerUrl: String(fm.issuerUrl),
        usedBy: String(fm.usedBy),
        scope: String(fm.scope),
      },
      fileBasename,
    });
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
    const { status, days } = dueState(today, lastRotated, entry.periodDays);
    if (status === "ok") continue;
    const title = `Rotate ${entry.title} (last rotated ${entry.lastRotated})`;
    if (await existingOpenIssue(title) !== null) {
      skipped.push(entry.title);
      continue;
    }
    await ensureLabel();
    const body = issueBody(entry, fileBasename, status, days);
    await $`gh issue create \
      --title ${title} \
      --body ${body} \
      --label ${ISSUE_LABEL} \
      --assignee ${ASSIGNEE}`.quiet();
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

try {
  process.exit(await main());
} catch (err) {
  if (err instanceof SystemExit) {
    process.stderr.write(`${err.message}\n`);
    process.exit(1);
  }
  throw err;
}
