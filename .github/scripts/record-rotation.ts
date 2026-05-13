#!/usr/bin/env bun
/**
 * Update the `last-rotated` date for one secret in
 * `docs/development/secret-rotations/`. In place.
 *
 * Invoked from `.github/workflows/record-secret-rotation.yml`
 * after the maintainer has rotated a credential at its issuer.
 * The workflow then commits the result on a fresh branch and
 * opens a PR so CODEOWNERS and the mdsmith lint job both gate
 * the change.
 *
 * Usage:
 *     bun run record-rotation <ENTRY_TITLE> <YYYY-MM-DD>
 *
 * `ENTRY_TITLE` is the `title:` field in one of the
 * per-secret files under
 * `docs/development/secret-rotations/` (e.g. `VSCE_PAT`),
 * never a credential value. The script never reads, prints,
 * or stores any credential material — `entryTitle` flows only
 * as a lookup key.
 *
 * Exit codes:
 * - 0: success — front matter updated, or the date was already
 *   set to the requested value (no-op)
 * - 1: doc malformed, title not found, or date not valid
 *   ISO-8601
 */

import { Glob } from "bun";
import { basename, resolve } from "node:path";
import { parse as yamlParse } from "yaml";

const REPO_ROOT = resolve(import.meta.dir, "..", "..");
const ROTATIONS_DIR = resolve(REPO_ROOT, "docs/development/secret-rotations");

class SystemExit extends Error {}

interface SplitFrontMatter {
  /** The literal "---\n" opener. */
  opening: string;
  /** The YAML between the two "---\n" fences, fences NOT included. */
  yamlBlock: string;
  /** The closing "\n---\n" fence plus the rest of the document. */
  closingPlusBody: string;
}

function splitFrontMatter(text: string, path: string): SplitFrontMatter {
  if (!text.startsWith("---\n")) {
    throw new SystemExit(`${path}: no front matter`);
  }
  const end = text.indexOf("\n---\n", 4);
  if (end === -1) {
    throw new SystemExit(`${path}: unterminated front matter`);
  }
  return {
    opening: text.slice(0, 4),
    yamlBlock: text.slice(4, end),
    closingPlusBody: text.slice(end),
  };
}

/** Rewrite the `lastRotated:` line in the YAML block to the new
 * date. The structural rewrite is a regex on the source so
 * unrelated formatting (key order, quoting style, comments) is
 * preserved. We expect one entry per file, so the matcher does
 * not need to disambiguate multiple `lastRotated:` keys.
 */
function updateLastRotated(yamlBlock: string, date: string): string {
  // Captures:
  //   group(1): the `lastRotated:` key
  //   group(2): the current value (quoted or bare)
  const pattern = /(^lastRotated:\s*)("?[^"\n]*"?)/m;
  let matched = 0;
  const rewritten = yamlBlock.replace(pattern, (_, preamble) => {
    matched++;
    return `${preamble}"${date}"`;
  });
  if (matched === 0) {
    throw new SystemExit(`could not locate \`lastRotated:\` line`);
  }
  return rewritten;
}

function isIsoDate(s: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(s)) return false;
  const parsed = new Date(`${s}T00:00:00Z`);
  return !Number.isNaN(parsed.getTime());
}

/** Find the per-secret file whose front-matter `title` matches
 * `entryTitle`. Returns the absolute file path and the known
 * titles for the error message if no match. */
async function findEntry(entryTitle: string): Promise<{ path: string; titles: string[] }> {
  const glob = new Glob("*.md");
  const titles: string[] = [];
  let match: string | null = null;
  for await (const rel of glob.scan({ cwd: ROTATIONS_DIR })) {
    const path = resolve(ROTATIONS_DIR, rel);
    const text = await Bun.file(path).text();
    // Cheap parse: only need the title for matching, but use the
    // YAML parser anyway to avoid hand-rolled string handling.
    const fmEnd = text.indexOf("\n---\n", 4);
    if (fmEnd === -1) continue;
    const fm = yamlParse(text.slice(4, fmEnd));
    if (fm !== null && typeof fm === "object" && !Array.isArray(fm)) {
      const title = (fm as { title?: unknown }).title;
      if (typeof title === "string") {
        titles.push(title);
        if (title === entryTitle) match = path;
      }
    }
  }
  if (match === null) {
    titles.sort();
    throw new SystemExit(`unknown title '${entryTitle}'; known: ${titles.join(", ")}`);
  }
  return { path: match, titles };
}

async function main(argv: string[]): Promise<number> {
  if (argv.length !== 4) {
    process.stderr.write("usage: bun run record-rotation <ENTRY_TITLE> <YYYY-MM-DD>\n");
    return 1;
  }
  const entryTitle = argv[2]!;
  const dateStr = argv[3]!;
  if (!isIsoDate(dateStr)) {
    process.stderr.write(`invalid date '${dateStr}': not a valid ISO-8601 date\n`);
    return 1;
  }
  const { path } = await findEntry(entryTitle);
  const text = await Bun.file(path).text();
  const { opening, yamlBlock, closingPlusBody } = splitFrontMatter(text, path);
  const updated = updateLastRotated(yamlBlock, dateStr);
  if (updated === yamlBlock) {
    console.log(`${entryTitle} (${basename(path)}) already at ${dateStr}; no change`);
    return 0;
  }
  await Bun.write(path, opening + updated + closingPlusBody);
  console.log(`updated ${entryTitle} (${basename(path)}).last-rotated -> ${dateStr}`);
  return 0;
}

try {
  process.exit(await main(Bun.argv));
} catch (err) {
  if (err instanceof SystemExit) {
    process.stderr.write(`${err.message}\n`);
    process.exit(1);
  }
  throw err;
}
