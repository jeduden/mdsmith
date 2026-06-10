---
title: Migrate from markdownlint
weight: 70
summary: >-
  Convert a markdownlint config to `.mdsmith.yml` with `mdsmith init
  --from-markdownlint`, review the conversion notes, move inline
  disables into overrides, and run both linters in parallel until
  cutover.
---
# Migrate from markdownlint

mdsmith covers all 52 active markdownlint rules under different ids,
and `mdsmith init --from-markdownlint` rewrites a markdownlint config
into `.mdsmith.yml` mechanically. The CLI shape carries over:
`mdsmith check .` mirrors `markdownlint .`, and both exit non-zero on
violations. The
[rule mapping](../reference/markdownlint-mapping.md) and the converter
read the same per-rule data, so neither can drift from the other.

## 1. Convert the config

Run the converter next to your markdownlint config. It probes
`.markdownlint.jsonc`, `.markdownlint.json`, `.markdownlint.yaml`,
`.markdownlint.yml`, and `.markdownlintrc`; pass
`--from-markdownlint=<path>` to name a file instead.

```bash
mdsmith init --from-markdownlint
```

Given this `.markdownlint.yaml`:

```yaml
default: true
MD013:
  line_length: 100
MD024:
  siblings_only: true
MD033: false
```

the command writes this `.mdsmith.yml`:

```yaml
# Converted from .markdownlint.yaml by mdsmith init --from-markdownlint.
# Rules not listed here keep their mdsmith defaults.
#
# Not converted:
# - MD024 option "siblings_only": no mdsmith equivalent
# - markdownlint enables these checks by default, but the mdsmith
#   analogs are opt-in and use mdsmith's own default settings — review
#   and enable each with "<rule>: true": ambiguous-emphasis (MD037),
#   descriptive-link-text (MD059), emphasis-style (MD049, MD050),
#   horizontal-rule-style (MD035), link-style (MD054), list-marker-style
#   (MD004), no-space-in-code-spans (MD038), no-space-in-link-text
#   (MD039), ordered-list-numbering (MD029), proper-names (MD044),
#   single-h1 (MD025)

front-matter: true
rules:
    line-length:
        max: 100
```

The example shows the converter's contract:

- `MD013.line_length` became `line-length.max`. Option values translate
  whenever mdsmith has the matching setting.
- `MD033: false` produced no entry. `no-inline-html` is already opt-in
  in mdsmith, so there is nothing to disable.
- `MD024.siblings_only` has no mdsmith setting, so the converter kept
  `no-duplicate-headings` at its default and recorded a note instead of
  guessing.

The notes are also echoed on stderr. The converter never invents
settings: anything it cannot translate faithfully lands in the
`# Not converted:` block for review.

## 2. Review the conversion notes

Work through each `# Not converted:` line:

- Untranslated options (like `siblings_only`): run
  `mdsmith help <rule>` to read what the mdsmith rule enforces. In most
  cases the option narrowed a check that mdsmith scopes differently.
- Opt-in analogs: markdownlint enables style rules such as MD004 (list
  markers) by default with a "consistent" policy. The mdsmith analogs
  are opt-in and declare one concrete style, for example
  `list-marker-style: {style: dash}`. Enable the ones your project
  wants and set the style your files already use.
- Tags and `extends`: tag toggles (`whitespace: false`) and `extends:`
  chains are not converted. Disable the individual rules, or inline the
  extended config and convert again.

## 3. Move inline disables into config

mdsmith has no `<!-- markdownlint-disable -->` comment. Replace each
inline disable with an `overrides:` entry — the per-glob layer of
`.mdsmith.yml` — or list whole files under `ignore:`:

```yaml
overrides:
  - glob: ["CHANGELOG.md"]
    rules:
      line-length: false

ignore:
  - "vendor/**"
```

A `.markdownlintignore` file translates to the same `ignore:` list.

## 4. Run both linters on one PR

Keep the markdownlint CI job. Add mdsmith beside it, compare reports on
a representative PR, and retire the markdownlint job once mdsmith
reports everything you care about:

```yaml
- name: mdsmith
  run: |
    go install github.com/jeduden/mdsmith/cmd/mdsmith@latest
    mdsmith check .
```

The migration is done when `mdsmith check .` passes in CI and the
markdownlint job is deleted.

## Rule mapping

The [markdownlint rule mapping](../reference/markdownlint-mapping.md)
lists every markdownlint rule beside the mdsmith rule that covers it,
generated from the rule READMEs' front matter.

## See also

- [`mdsmith init`](../reference/cli/init.md) — flags and exit codes.
- [Markdown linters comparison](../background/markdown-linters.md) —
  feature-by-feature breakdown.
- [Conventions](../reference/conventions.md) — pin a preset that
  matches a markdownlint default.
- [Coexist with Prettier](coexist-with-prettier.md) — if you pair
  markdownlint with Prettier today.
