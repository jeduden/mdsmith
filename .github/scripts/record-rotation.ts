#!/usr/bin/env bun
/**
 * Update the `lastRotated` date for one secret in
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

export class SystemExit extends Error {}

export interface SplitFrontMatter {
  /** The literal "---\n" opener. */
  opening: string;
  /** The YAML between the two "---\n" fences, fences NOT included. */
  yamlBlock: string;
  /** The closing "\n---\n" fence plus the rest of the document. */
  closingPlusBody: string;
}

export function splitFrontMatter(text: string, path: string): SplitFrontMatter {
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
 * unrelated formatting (key order, comments, blank lines) is
 * preserved. We expect one entry per file, so the matcher does
 * not need to disambiguate multiple `lastRotated:` keys.
 *
 * Quoting style is normalized to double-quoted regardless of
 * what the source had: dates are bare ISO-8601 strings that YAML
 * could parse either as a string or a date depending on the
 * parser, and double-quoting forces the string interpretation
 * the rest of the toolchain expects.
 *
 * The value matcher accepts double-quoted, single-quoted, or
 * unquoted bare values. Any trailing inline comment (e.g.
 * `lastRotated: 2026-05-12 # rotated after incident`) is left
 * intact because the match stops at the first whitespace or `#`
 * after the value.
 */
export function updateLastRotated(yamlBlock: string, date: string, path: string): string {
  // Group 1: optional leading indent + the `lastRotated:` key +
  // its trailing spaces. Match continues with the value, which
  // is dropped wholesale:
  //   - `"..."` — double-quoted string
  //   - `'...'` — single-quoted string
  //   - `[^\s#"']\S*` — bare value (one non-space/comment/quote
  //     leading char, then any non-space chars). Stops at the
  //     first whitespace or `#`, so a trailing inline comment is
  //     preserved by the surrounding text outside the match.
  // Leading whitespace is tolerated so a future YAML formatter
  // that indents top-level keys does not break the rewrite.
  const pattern = /(^[ \t]*lastRotated:[ \t]*)(?:"[^"\n]*"|'[^'\n]*'|[^\s#"'][^\s#]*)/m;
  let matched = 0;
  const rewritten = yamlBlock.replace(pattern, (_, preamble) => {
    matched++;
    return `${preamble}"${date}"`;
  });
  if (matched === 0) {
    throw new SystemExit(`${path}: could not locate \`lastRotated:\` line`);
  }
  return rewritten;
}

export function isIsoDate(s: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(s)) return false;
  const parsed = new Date(`${s}T00:00:00Z`);
  if (Number.isNaN(parsed.getTime())) return false;
  // Round-trip: `new Date("2026-02-31T00:00:00Z")` normalizes to
  // March 3, so a regex-passing string can still be an invalid
  // calendar date. Re-emit the parsed components in the same
  // YYYY-MM-DD shape and demand a byte-for-byte match.
  const year = String(parsed.getUTCFullYear()).padStart(4, "0");
  const month = String(parsed.getUTCMonth() + 1).padStart(2, "0");
  const day = String(parsed.getUTCDate()).padStart(2, "0");
  return `${year}-${month}-${day}` === s;
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
  // Bun.argv is `[runtime, script, ...userArgs]`; we need exactly
  // two user args (title, date). Accept argv.length >= 4 rather
  // than strict equality so wrapper invocations that append a
  // trailing sentinel (`bun run -- ...`, future tooling, etc.)
  // are tolerated; extras are ignored.
  if (argv.length < 4) {
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
  const updated = updateLastRotated(yamlBlock, dateStr, path);
  if (updated === yamlBlock) {
    console.log(`${entryTitle} (${basename(path)}) lastRotated already at ${dateStr}; no change`);
    return 0;
  }
  await Bun.write(path, opening + updated + closingPlusBody);
  console.log(`updated ${entryTitle} (${basename(path)}) lastRotated -> ${dateStr}`);
  return 0;
}

// Gate the auto-run on `import.meta.main` so test files that
// import this module for its pure exports do not also fire
// `main()` (which touches the filesystem) on import.
if (import.meta.main) {
  try {
    process.exit(await main(Bun.argv));
  } catch (err) {
    if (err instanceof SystemExit) {
      process.stderr.write(`${err.message}\n`);
      process.exit(1);
    }
    throw err;
  }
}
