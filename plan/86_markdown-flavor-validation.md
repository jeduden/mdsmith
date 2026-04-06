---
id: 86
title: Markdown flavor validation
status: "🔲"
summary: >-
  New rule MDS034 that validates Markdown files against
  a declared flavor (CommonMark, GFM, Goldmark, etc.)
  and reports unsupported syntax with auto-fix where
  possible.
---
# Markdown flavor validation

## Goal

Let users declare a target Markdown flavor. mdsmith
then flags syntax that the target renderer will not
understand. This prevents silent rendering failures
when files move between tools.

## Background

### Flavors and syntax differences

Nine common flavors exist. CommonMark is the strict
baseline. GFM adds tables, task lists, strikethrough,
and autolinks. Goldmark supports GFM-like features
plus optional footnotes, heading IDs, and math.

Other flavors add more features. PHP Markdown Extra
has footnotes, heading IDs, and more. Pandoc adds
math, citations, and sub/superscript. MyST targets
Sphinx with roles and directives.

Twelve features vary across flavors: tables, task
lists, strikethrough, autolinks, footnotes, heading
IDs, math, and five others.

### Detection approach

The main parser uses only CommonMark block/inline
parsers. Enabling extensions there would change the
AST for all 33 existing rules. Regex-only detection
produces false positives inside code blocks and inline
code spans.

**Chosen: dual parser.** MDS034 re-parses with a
second goldmark parser that has all extensions enabled.
The main parse tree stays untouched.

Goldmark has built-in extensions for 7 features. Five
custom detection-only extensions (~1250 LOC total with
tests) cover the rest. All 12 features detected via
AST -- no regex fallback. Each custom extension
follows the PI block parser pattern
([internal/lint/pi_parser.go](../internal/lint/pi_parser.go),
[internal/lint/pi.go](../internal/lint/pi.go)).

## Design

### Configuration

```yaml
rules:
  markdown-flavor:
    flavor: gfm  # commonmark | gfm | goldmark
```

Category: `meta`. Disabled by default (opt-in).

### Dual parser

```go
p := goldmark.New(
    goldmark.WithExtensions(
        extension.Table,
        extension.Strikethrough,
        extension.TaskList,
        extension.Footnote,
        extension.DefinitionList,
        extension.Linkify,
        &SuperscriptExt{},
        &SubscriptExt{},
        &MathBlockExt{},
        &MathInlineExt{},
        &AbbreviationExt{},
    ),
    goldmark.WithParserOptions(
        parser.WithAttribute(),
    ),
)
```

mdsmith clones configurable rules per file
([internal/rule/clone.go](../internal/rule/clone.go)).
Storing the parser on the rule struct would rebuild it
per clone. Cache the parser in package-level shared
state instead (e.g. a `sync.Once` singleton or a
flavor-keyed map guarded by a mutex).

### Custom extensions

Five extensions in
`internal/rules/markdownflavor/ext/`:

- **SuperscriptExt**: inline `^text^`, ~150 LOC
- **SubscriptExt**: inline `~text~`, distinguishes
  single `~` from strikethrough `~~`, ~170 LOC
- **MathBlockExt**: block `$$...$$` fence, ~160 LOC
- **MathInlineExt**: inline `$...$` with Pandoc-style
  flanking (opening `$` preceded by whitespace, not
  followed by whitespace), ~200 LOC
- **AbbreviationExt**: block `*[term]: expansion`
  parser + paragraph transformer that marks every
  inline occurrence of the term, ~400 LOC

Detection-only (no HTML rendering needed).

### Error messages

Each message names the feature and the flavor:
`{feature} is not supported by {flavor}`. Fixable
features include a suggestion (e.g. "wrap in angle
brackets"). Severity: `warning`.

### Auto-fix

Fixable: autolinks (wrap in `<url>`), heading IDs
(remove `{#id}`), strikethrough (remove `~~`), task
lists (`- [ ]` to `- `), superscript (remove `^`),
subscript (remove `~`). Non-fixable: tables,
footnotes, definition lists, math, abbreviations.

## Tasks

1. Add feature enum and flavor registry in
   `internal/rules/markdownflavor/features.go`
2. Write `SuperscriptExt` inline parser in
   `internal/rules/markdownflavor/ext/superscript.go`
3. Write `SubscriptExt` inline parser in
   `internal/rules/markdownflavor/ext/subscript.go`
4. Write `MathBlockExt` block parser in
   `internal/rules/markdownflavor/ext/mathblock.go`
5. Write `MathInlineExt` inline parser in
   `internal/rules/markdownflavor/ext/mathinline.go`
6. Write `AbbreviationExt` block parser + paragraph
   transformer in
   `internal/rules/markdownflavor/ext/abbreviation.go`
7. Add tests for all five custom extensions
8. Build dual parser with built-in + custom extensions
9. Add AST-based detectors for all 12 features
10. Implement `rule.go` with `Check()` and `Fix()`
11. Implement `rule.Configurable` for MDS034: add
    `ApplySettings` and `DefaultSettings` for `flavor`
12. Register as MDS034 in category `meta`
13. Add test fixtures in
    `internal/rules/MDS034-markdown-flavor/`
14. Add rule README and update docs

## Acceptance Criteria

- [ ] `flavor: commonmark` flags tables, task lists,
      strikethrough, and autolinks
- [ ] `flavor: gfm` accepts GFM features, flags
      footnotes, definition lists, math
- [ ] `flavor: goldmark` accepts Goldmark defaults,
      flags Pandoc-only features
- [ ] Error messages name the unsupported feature and
      the configured flavor
- [ ] `mdsmith fix` auto-fixes fixable features
- [ ] Non-fixable features produce diagnostics only
- [ ] Invalid flavor name produces a config error
- [ ] Rule is disabled by default (opt-in)
- [ ] Rule does not conflict with MDS012
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
