---
summary: >-
  How the placeholder vocabulary lets rules treat template tokens as
  opaque rather than flagging them as content violations.
---
# Placeholder grammar

Placeholder grammar is an opt-in vocabulary of named tokens that rules
can treat as opaque. A rule with a `placeholders:` setting consults the
shared helper and suppresses diagnostics for content that matches a
configured token. This lets template files coexist with linting: rules
that apply to real content still run, but placeholder tokens do not
trigger false positives.

## Token vocabulary

| Token name            | Matches                                                          |
|-----------------------|------------------------------------------------------------------|
| `var-token`           | `{identifier}` interpolation placeholders (`{title}`, `{a.b.c}`) |
| `heading-question`    | A heading whose text is exactly `?`                              |
| `placeholder-section` | A heading whose text is exactly `...`                            |
| `cue-frontmatter`     | CUE constraint expressions in front-matter values                |

The vocabulary is closed. Adding a token requires one change in
`internal/placeholders/placeholders.go` plus per-rule opt-ins; no rule
hardcodes token names in its own logic.

## The `placeholders:` setting contract

Any rule that opts in exposes `placeholders:` as a setting through the
[`Configurable`](../../../internal/rule/rule.go) interface. The value
is a list of token names:

```yaml
kinds:
  proto:
    rules:
      first-line-heading:
        placeholders: [heading-question]
      heading-increment:
        placeholders: [heading-question, placeholder-section]
      cross-file-reference-integrity:
        placeholders: [var-token]
      paragraph-readability:
        placeholders: [var-token]
      paragraph-structure:
        placeholders: [var-token]
      required-structure:
        placeholders: [cue-frontmatter]
```

When `placeholders:` is empty (the default), every rule produces the
same diagnostics it does today — there is no behavioral change.

## How rules opt in

A rule adds `Placeholders []string` to its struct and calls the helper
from `internal/placeholders`:

- `ContainsBodyToken(text, tokens)` — returns true if text matches any
  configured body token. Used by heading and paragraph rules to skip
  checks on placeholder content.
- `MaskBodyTokens(text, tokens)` — replaces matched tokens with
  neutral text for analysis (e.g. `{title}` → `word`). Used by
  paragraph-readability and paragraph-structure to check non-placeholder
  parts of the text.
- `HasCUEFrontmatter(tokens)` — returns true if `cue-frontmatter` is
  configured. Used by required-structure to skip CUE front-matter
  validation.

## Opt-in rules

| Rule ID | Rule name                        | Useful tokens                                          |
|---------|----------------------------------|--------------------------------------------------------|
| MDS003  | `heading-increment`              | `heading-question`, `placeholder-section`, `var-token` |
| MDS004  | `first-line-heading`             | `heading-question`, `var-token`, `placeholder-section` |
| MDS018  | `no-emphasis-as-heading`         | `var-token`, `heading-question`, `placeholder-section` |
| MDS020  | `required-structure`             | `cue-frontmatter`                                      |
| MDS023  | `paragraph-readability`          | `var-token`, `heading-question`, `placeholder-section` |
| MDS024  | `paragraph-structure`            | `var-token`, `heading-question`, `placeholder-section` |
| MDS027  | `cross-file-reference-integrity` | `var-token`, `heading-question`, `placeholder-section` |
