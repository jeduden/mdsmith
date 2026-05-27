---
title: Coexist with Prettier
summary: >-
  mdsmith works alongside an existing Prettier setup.
  Keep the Prettier config and run `mdsmith fix`
  before `prettier --write` in the same pre-commit
  hook.
---
# Coexist with Prettier

If your project already runs Prettier on Markdown,
adding mdsmith does not require changing the Prettier
setup. Run `mdsmith fix` before `prettier --write`
in the same pre-commit hook. A second run of either
tool then produces no diff. Prettier handles
paragraph wrapping and the final table layout;
mdsmith handles the formatting rules Prettier does
not touch, plus generated sections, cross-file
links, and readability budgets.

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
order matters. Prettier's table formatter produces
the same column widths mdsmith's `table-format` rule
does; once Prettier runs last on those constructs, a
second pass changes nothing.

## Do you need to change your Prettier config?

Mostly no. Default Prettier and default mdsmith
align: both target 80 columns, both indent with two
spaces, both normalize unordered list markers to `-`.
Prettier's default `proseWrap: "preserve"` leaves the
line breaks mdsmith's `line-length` rule sized in
place; `proseWrap` controls paragraph reflow, not
table layout.

Two exceptions:

- If `line-length.max` is above 80, raise Prettier's
  `printWidth` to the same number. Otherwise Prettier
  rewraps lines mdsmith still considers within
  budget and every commit produces churn.
- If `list-marker-style` (MDS045, opt-in) is enabled,
  set `style: dash` to match Prettier's default.

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

For projects not on `husky` / `lint-staged`, the same
ordering works in a hand-written `.husky/pre-commit`:

```sh
list=$(git diff --cached --name-only --diff-filter=ACMR -z -- '*.md')
[ -z "$list" ] && exit 0
printf '%s' "$list" | xargs -0 mdsmith fix --
printf '%s' "$list" | xargs -0 git add --
printf '%s' "$list" | xargs -0 npx prettier --write --
printf '%s' "$list" | xargs -0 git add --
```

POSIX sh; NUL-delimited so filenames with spaces
survive; exits early on an empty stage so neither
tool falls back to a full-repo rewrite.

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
mdsmith does not implement. The question is whether
the project needs mdsmith.

Keep mdsmith if the repo relies on any of:

- Generated sections (`<?catalog?>`, `<?include?>`,
  `<?toc?>`, `<?build?>`).
- Cross-file link or anchor integrity checks.
- Per-file kinds, schemas, or readability budgets.
- Release-gating on Markdown metrics.

Drop mdsmith if none of those apply and the only
formatting need is paragraph wrapping. Prettier
covers that.

## See also

- [Auto-fix](../features/auto-fix.md) — what
  `mdsmith fix` rewrites.
- [Migrate from markdownlint](migrate-from-markdownlint.md)
  — if you used markdownlint + Prettier before.
