---
title: Coexist with Prettier
summary: >-
  Prettier owns whitespace and line wrapping; mdsmith owns
  lint, generated sections, and cross-file checks. Run both
  in a single pre-commit hook with the order Prettier last.
---
# Coexist with Prettier

Prettier is a formatter. mdsmith is a linter that also
auto-fixes the deterministic formatting it owns
(whitespace, heading style, code fences, list indent,
and table padding). Both tools can rewrite tables and
list spacing, so coexistence works by giving one tool
the final say — Prettier, run last.

## Who owns what

| Concern                              | Owner    |
|--------------------------------------|----------|
| Final paragraph wrapping             | Prettier |
| Final table alignment                | Prettier |
| Trailing whitespace, hard tabs       | mdsmith  |
| Heading style (atx vs. setext)       | mdsmith  |
| Fenced-code style and language tag   | mdsmith  |
| Bare URLs                            | mdsmith  |
| Generated sections (catalog, toc)    | mdsmith  |
| Cross-file link and anchor integrity | mdsmith  |
| Readability budgets                  | mdsmith  |

Both tools rewrite table padding. Prettier's `--prose-wrap
preserve` (or default) leaves a mdsmith-aligned table
intact. Run mdsmith first, Prettier last, and the result
is stable on a second pass.

## Pre-commit recipe

With `lint-staged` and `husky`:

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

With a plain Git hook (`.husky/pre-commit`) — POSIX sh, NUL-
delimited so filenames with spaces survive, and a no-op on
an empty stage so neither tool falls back to a full-repo
rewrite:

```sh
list=$(git diff --cached --name-only --diff-filter=ACMR -z -- '*.md')
[ -z "$list" ] && exit 0
printf '%s' "$list" | xargs -0 mdsmith fix --
printf '%s' "$list" | xargs -0 git add --
printf '%s' "$list" | xargs -0 npx prettier --write --
printf '%s' "$list" | xargs -0 git add --
```

## CI

```yaml
- name: prettier check
  run: npx prettier --check '**/*.md'
- name: mdsmith check
  run: mdsmith check .
```

Order does not matter on read-only CI — both jobs only
report violations. Run them in parallel for speed.

## See also

- [Auto-fix](../features/auto-fix.md) — the scope of
  what `mdsmith fix` rewrites.
- [Migrate from markdownlint](migrate-from-markdownlint.md)
  — if you used markdownlint + Prettier before.
