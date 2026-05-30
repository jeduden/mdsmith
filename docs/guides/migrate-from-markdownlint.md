---
title: Migrating from markdownlint
summary: >-
  Move a project from markdownlint-cli or markdownlint-cli2
  to mdsmith — the rule mapping, the config rewrite, and
  the markdownlint-to-mdsmith rule correspondence.
---
# Migrating from markdownlint

mdsmith covers all 52 active markdownlint rules under
different IDs. The CLI shape is similar (`mdsmith check
.` mirrors `markdownlint .`), the auto-fix story is
strictly broader, and the config moves from
`.markdownlint.yaml` to `.mdsmith.yml`. This guide is
the on-ramp.

## What stays the same

- Per-file Markdown style checks (heading style,
  line-length, list indent, fenced-code style, etc.).
- Run in CI with a non-zero exit code on violations.
- Per-rule disable via config.

## What changes

- Rule IDs are `MDSxxx`, not `MDxxx`. The
  [linter comparison](../background/markdown-linters.md)
  documents rule-for-rule coverage.
- mdsmith adds cross-file integrity, generated sections,
  file kinds, and readability budgets that markdownlint
  does not have.
- Config lives in `.mdsmith.yml` with deep-merged layers
  (defaults → convention → kinds → overrides) rather
  than the flat markdownlint JSON / YAML.

## Convert `.markdownlint.yaml`

A typical markdownlint config looks like:

```yaml
default: true
MD013:
  line_length: 100
MD024:
  siblings_only: true
MD033: false
```

The equivalent `.mdsmith.yml`:

```yaml
rules:
  line-length:
    max: 100
  no-duplicate-headings:
    siblings-only: true
  no-inline-html: false
```

Rule names use kebab-case in mdsmith. Run `mdsmith init`
to scaffold a starting config with the defaults
expanded; then trim it to only the rules you want to
override.

## Rule mapping (high traffic)

| markdownlint | mdsmith                              | Notes                                                                 |
| ------------ | ------------------------------------ | --------------------------------------------------------------------- |
| MD001        | `heading-increment`                  | Same semantics                                                        |
| MD003        | `heading-style`                      | atx / setext discriminator                                            |
| MD004        | `list-marker-style`                  | `*` / `-` / `+`                                                       |
| MD007        | `list-indent`                        | Spaces per nesting level                                              |
| MD009        | `no-trailing-spaces`                 | Same                                                                  |
| MD010        | `no-hard-tabs`                       | Same                                                                  |
| MD012        | `no-multiple-blanks`                 | `max:` argument                                                       |
| MD013        | `line-length`                        | `max:` + exclusion lists                                              |
| MD018-MD020  | `atx-heading-whitespace`             | Unified into one rule                                                 |
| MD022        | `blank-line-around-headings`         | Same                                                                  |
| MD024        | `no-duplicate-headings`              | `siblings-only:` flag                                                 |
| MD025        | `single-h1`                          | One top-level heading per file                                        |
| MD026        | `no-trailing-punctuation-in-heading` | Same                                                                  |
| MD031        | `blank-line-around-fenced-code`      | Same                                                                  |
| MD032        | `blank-line-around-lists`            | Same                                                                  |
| MD033        | `no-inline-html`                     | Allow-list argument                                                   |
| MD034        | `no-bare-urls`                       | Same                                                                  |
| MD040        | `fenced-code-language`               | Same                                                                  |
| MD041        | `first-line-heading`                 | `level:` argument                                                     |
| MD046        | `fenced-code-style`                  | backtick / tilde                                                      |
| MD047        | `single-trailing-newline`            | Same                                                                  |
| MD054        | `link-style` (opt-in)                | `links.style.link-image-style` six-toggle map; all default to allowed |

See [linter comparison](../background/markdown-linters.md)
for the full coverage table.

## Run both in parallel for one PR

Keep your existing `.markdownlint.yaml` in place. Add
the mdsmith CI job. Compare reports on a representative
PR. Once mdsmith reports a strict superset of the
violations you care about, retire the markdownlint job.

```yaml
- name: mdsmith
  run: |
    go install github.com/jeduden/mdsmith/cmd/mdsmith@latest
    mdsmith check .
```

## See also

- [Linter comparison](../background/markdown-linters.md)
  — feature-by-feature breakdown.
- [Conventions](../reference/conventions.md) — pin a
  flavor preset that matches a markdownlint default.
- [Coexist with Prettier](coexist-with-prettier.md) —
  if you already pair markdownlint with Prettier.
