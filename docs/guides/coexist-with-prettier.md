---
title: Coexist with Prettier
summary: >-
  Run mdsmith alongside Prettier by ordering
  `mdsmith fix` before `prettier --write` in the
  same pre-commit hook.
---
# Coexist with Prettier

If your project already runs Prettier on Markdown,
adding mdsmith does not require changing the
Prettier setup. Run `mdsmith fix` before
`prettier --write` in the same pre-commit hook. A
second run of either tool then produces no diff.

Prettier handles paragraph wrapping and the final
table layout. mdsmith handles the formatting rules
Prettier does not touch, plus generated sections,
cross-file links, and readability budgets.

## Quick start

```json
{
  "lint-staged": {
    "*.md": [
      "mdsmith fix",
      "prettier --write"
    ]
  }
}
```

mdsmith first, Prettier last, in one `lint-staged`
array.

## Which tool owns what

When a check fails or a fixer rewrites something
unexpected, the table below shows which tool to
configure:

| Concern                              | Owner    |
| ------------------------------------ | -------- |
| Final paragraph wrapping             | Prettier |
| Final table alignment                | Prettier |
| Trailing whitespace, hard tabs       | mdsmith  |
| Heading style (atx vs. setext)       | mdsmith  |
| Fenced-code style and language tag   | mdsmith  |
| Bare URLs                            | mdsmith  |
| Generated sections (catalog, toc)    | mdsmith  |
| Cross-file link and anchor integrity | mdsmith  |
| Readability budgets                  | mdsmith  |

The two tools overlap on GFM table padding and
list-item indentation. Both rewrite those bytes, so
order matters. With both tools at default settings,
Prettier's table formatter produces the same column
widths mdsmith's `table-format` rule does. Once
Prettier runs last on those constructs, a second
pass changes nothing. Customizing `table-format`
(`pad`, `separator-style`, `style`) can break that
convergence; check those settings first if a stable
hook starts producing diffs.

## Do you need to change your Prettier config?

Mostly no. Default Prettier and default mdsmith line
up: both target 80 columns and both indent with two
spaces. Prettier's default `proseWrap: "preserve"`
controls paragraph reflow only. It leaves alone
existing line breaks that keep mdsmith's
`line-length` rule passing. mdsmith only checks line
length; it does not wrap paragraphs. mdsmith also
does not normalize list markers by default, so
Prettier owns those rewrites without contention.

One value to keep aligned: Prettier's `printWidth`
and the `max` setting of mdsmith's `line-length`
rule (`rules.line-length.max` in `.mdsmith.yml`).
Both default to 80. If `printWidth` exceeds that
`max`, Prettier may merge wrapped lines into ones
mdsmith then flags as too long. mdsmith does not
auto-fix line length, so the rewrap would have to be
manual. Keep the two values equal.

## Generated sections

Prettier does not parse mdsmith's `<?...?>` directive
markers and may rewrap text inside generated bodies.
Add affected files to `.prettierignore` if that
shows up in diffs. Generated bodies regenerate from
the directive source on the next `mdsmith fix`, so
the worst case is a one-commit round-trip. Never
hand-edit content between `<?directive?>` and
`<?/directive?>` markers.

## Plain Git hook

Without `lint-staged`, write the hook by hand. Place
this script at `.husky/pre-commit` (under husky) or
`.git/hooks/pre-commit` (plain Git hooks):

```sh
tmp=$(mktemp) || exit 1
trap 'rm -f "$tmp"' EXIT INT TERM
git diff --cached --name-only --diff-filter=ACMR -z -- '*.md' > "$tmp"
[ -s "$tmp" ] || exit 0
xargs -0 mdsmith fix -- < "$tmp"
xargs -0 git add -- < "$tmp"
xargs -0 npx prettier --write -- < "$tmp"
xargs -0 git add -- < "$tmp"
```

POSIX shell syntax with two near-universal
extensions: `mktemp` and `xargs -0`. Linux, macOS,
BSD, and busybox all support both. The NUL-delimited
file list lives in a temp file. POSIX command
substitution strips NUL bytes from `$(...)`, which
would break the filenames-with-spaces guarantee. The
hook exits early on an empty stage so neither tool
falls back to a full-repo rewrite.

## CI check

Both tools have read-only modes for CI:

```yaml
- name: prettier check
  run: npx prettier --check '**/*.md'
- name: mdsmith check
  run: mdsmith check .
```

Order does not matter here. Both jobs only report
violations; run them in parallel.

## When to drop mdsmith

Keep Prettier. It owns paragraph re-wrap, which
mdsmith does not implement.

Drop mdsmith only if the repo uses none of:

- Generated sections (`<?catalog?>`, `<?include?>`,
  `<?toc?>`, `<?build?>`).
- Cross-file link or anchor integrity checks.
- Per-file kinds, schemas, or readability budgets.
- Release-gating on Markdown metrics.

If none apply, Prettier alone covers paragraph
wrapping.

## See also

- [Auto-fix](../features/auto-fix.md) — what
  `mdsmith fix` rewrites.
- [Migrate from markdownlint](migrate-from-markdownlint.md)
  — if you used markdownlint + Prettier before.
