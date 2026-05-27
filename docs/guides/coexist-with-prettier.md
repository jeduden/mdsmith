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
`prettier --write` in the same pre-commit hook.
Under both tools' defaults, a second pass produces
no diff.

Prettier owns the final table layout. mdsmith owns
the formatting rules Prettier does not touch:
heading style, fenced-code style and language,
bare URLs, and trailing whitespace. mdsmith also
owns generated sections, cross-file links, and
readability budgets. Paragraph wrapping is manual
under default Prettier. Only `proseWrap: "always"`
makes Prettier wrap for you.

## Quick start

```json
{
  "lint-staged": {
    "**/*.md": [
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

| Concern                                     | Owner    |
| ------------------------------------------- | -------- |
| Paragraph wrap (with `proseWrap: "always"`) | Prettier |
| Final table alignment                       | Prettier |
| Final list-item indentation                 | Prettier |
| Trailing whitespace, hard tabs              | mdsmith  |
| Heading style (atx vs. setext)              | mdsmith  |
| Fenced-code style and language tag          | mdsmith  |
| Bare URLs                                   | mdsmith  |
| Generated sections (catalog, toc)           | mdsmith  |
| Cross-file link and anchor integrity        | mdsmith  |
| Readability budgets                         | mdsmith  |

The two tools overlap on GFM table padding and
list-item indentation. With `proseWrap: "always"`,
they also overlap on paragraph wrapping. Both
rewrite those bytes, so order matters.

With both tools at default settings, Prettier's
table formatter matches mdsmith's `table-format`
rule for plain ASCII tables. Edge cases can
differ: header alignment markers (`:---:`),
double-width characters (CJK, emoji), or code
spans containing pipes. Customizing `table-format`
(`pad`, `separator-style`, `style`) also breaks
the convergence.

## Do you need to change your Prettier config?

No, unless you change `proseWrap` from its default.

Prettier and mdsmith defaults align. Both target
80 columns (`printWidth` and `rules.line-length.max`
in `.mdsmith.yml`). Both indent with two spaces
(`tabWidth` and `rules.list-indent.spaces`).
Prettier's default `proseWrap: "preserve"` leaves
existing line breaks alone. mdsmith's `line-length`
rule reports long lines but does not rewrap them.
At defaults, neither tool reflows paragraphs. Long
lines are yours to wrap by hand.

If you set `proseWrap: "always"`, Prettier rewraps
paragraphs to `printWidth`. Keep `printWidth` no
larger than `rules.line-length.max`; both default
to 80. Otherwise Prettier wraps lines past
mdsmith's limit, and mdsmith cannot auto-fix them.
You would have to wrap them by hand.

## Generated sections

Prettier sees mdsmith's `<?...?>` directive markers
as CommonMark HTML blocks (processing
instructions) and preserves them. Under the
default `proseWrap: "preserve"`, Prettier also
leaves the content inside generated bodies alone.
If you set `proseWrap: "always"` and see Prettier
rewrap inside generated bodies, add the affected
files to `.prettierignore`. The next `mdsmith fix`
regenerates the body from the directive source.
The worst case is a one-commit round-trip. Never
hand-edit content between `<?directive?>` and
`<?/directive?>` markers.

## Plain Git hook

Without `lint-staged`, write the hook by hand.
Place this script at `.husky/pre-commit` (under
husky) or `.git/hooks/pre-commit` (plain Git
hooks). Make sure it is executable (`chmod +x`):

```sh
#!/bin/sh
set -e
tmp=$(mktemp "${TMPDIR:-/tmp}/mdsmith-prettier.XXXXXX") || exit 1
trap 'rm -f "$tmp"' EXIT
trap 'rm -f "$tmp"; exit 130' INT TERM
git diff --cached --name-only --diff-filter=ACMR -z -- '**/*.md' > "$tmp"
[ -s "$tmp" ] || exit 0
xargs -0 mdsmith fix -- < "$tmp"
xargs -0 git add -- < "$tmp"
xargs -0 npx prettier --write -- < "$tmp"
xargs -0 git add -- < "$tmp"
```

POSIX shell syntax with two near-universal
extensions: `mktemp` and `xargs -0`. Linux, macOS,
BSD, and busybox all support both.

The NUL-delimited file list lives in a temp file.
POSIX command substitution strips NUL bytes from
`$(...)`, which would break the filenames-with-
spaces guarantee. `set -e` aborts the commit on
the first failed step. The split `trap` cleans up
the temp file on every exit. On Ctrl+C or SIGTERM,
the trap also exits with 130, so the user can
interrupt the hook.

Two caveats this hook does not handle:

- Partial staging via `git add -p` is not
  preserved. The final `git add -- "$file"` stages
  the entire working tree of each `.md` file. Any
  hunks the user did not stage get pushed into the
  index. Use `lint-staged` (which stashes unstaged
  hunks) if partial-staging matters to your
  workflow.
- `npx prettier` fetches Prettier from the npm
  registry if it is not already in `node_modules`.
  Pre-install Prettier locally, or hard-code the
  path to your project binary. This avoids a
  network lookup on first run and keeps the hook
  working in offline or air-gapped CI.

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

Keep Prettier. It owns final table layout, and
(with `proseWrap: "always"`) paragraph wrapping.
The question is whether you also need mdsmith.

Keep mdsmith if the repo relies on any of:

- The formatting rules Prettier does not enforce:
  ATX vs. setext heading style, fenced-code
  language tags, bare-URL warnings, trailing-
  whitespace and hard-tab cleanup.
- Generated sections (`<?catalog?>`, `<?include?>`,
  `<?toc?>`, `<?build?>`).
- Cross-file link or anchor integrity checks.
- Per-file kinds, schemas, or readability budgets.
- Release-gating on Markdown metrics.

If none apply, Prettier alone covers the
formatting the project needs.

## See also

- [Auto-fix](../features/auto-fix.md) — what
  `mdsmith fix` rewrites.
- [Migrate from markdownlint](migrate-from-markdownlint.md)
  — if you used markdownlint + Prettier before.
