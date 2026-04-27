---
name: placeholder-grammar
summary: >-
  How the placeholder vocabulary lets rules treat template tokens as
  opaque rather than flagging them as content violations.
---
# Placeholder grammar

Placeholder grammar is an opt-in vocabulary that lets rules treat
template content as opaque. A rule with a `placeholders:` setting
skips diagnostics for content that matches a configured token.

## Token vocabulary

| Token name            | Matches                                                          |
|-----------------------|------------------------------------------------------------------|
| `var-token`           | `{identifier}` interpolation placeholders (`{title}`, `{a.b.c}`) |
| `heading-question`    | A heading whose text is exactly `?`                              |
| `placeholder-section` | A heading whose text is exactly `...`                            |
| `cue-frontmatter`     | CUE constraint expressions in front-matter values                |

## The `placeholders:` setting

Any opt-in rule accepts `placeholders:` as a list of token names.
When empty (default), rule behavior is unchanged.

```yaml
kinds:
  proto:
    rules:
      first-line-heading:
        placeholders: [heading-question]
      cross-file-reference-integrity:
        placeholders: [var-token]
      required-structure:
        placeholders: [cue-frontmatter]
```

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

See also: docs/background/concepts/placeholder-grammar.md
