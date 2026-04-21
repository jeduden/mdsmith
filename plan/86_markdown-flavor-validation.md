---
id: 86
title: Markdown flavor validation
status: "🔳"
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
and bare-URL autolinks. For MDS034, `goldmark` is
an mdsmith-defined profile: Goldmark plus tables,
task lists, strikethrough, bare-URL autolinks, and
heading IDs, but not optional footnote,
definition-list, or math extensions.

Other flavors add more features. PHP Markdown Extra
has footnotes, heading IDs, and more. Pandoc adds
math, citations, and sub/superscript. MyST targets
Sphinx with roles and directives.

Twelve features vary across flavors:

- tables
- task lists
- strikethrough
- bare-URL autolinks
- footnotes
- heading IDs
- inline math
- block math
- definition lists
- abbreviations
- superscript
- subscript

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
- **MathInlineExt**: inline `$...$` matching Pandoc
  `tex_math_dollars`
  (<https://pandoc.org/MANUAL.html#extension-tex_math_dollars>):
  opening `$` is allowed when the next character is
  not whitespace; closing `$` is allowed when the
  previous character is not whitespace and the next
  character is not a digit. Allows `($x$)` and
  `foo $x+1$ bar`, rejects `$ x $` and `$20`,
  ~200 LOC
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

Fixable features and their fixes:

- bare-URL autolinks with a scheme: wrap in `<url>`.
  `www.`-only URLs become `[url](http://url)` since
  `<www.example.com>` is not a valid CommonMark
  autolink
- heading IDs: remove `{#id}`
- strikethrough: remove `~~`
- task lists: for `-`, `*`, or `+` bullets with
  `[ ]`, `[x]`, or `[X]`, remove the task marker
  and keep the bullet (e.g. `- [ ]` to `- `,
  `* [x]` to `* `)
- superscript: remove `^`
- subscript: remove `~`

Non-fixable: tables, footnotes, definition lists,
math, abbreviations.

## Tasks

- [x] Add feature enum and flavor registry in
  `internal/rules/markdownflavor/features.go`
- [x] Write `SuperscriptExt` inline parser in
  `internal/rules/markdownflavor/ext/superscript.go`
- [x] Write `SubscriptExt` inline parser in
  `internal/rules/markdownflavor/ext/subscript.go`
- [x] Write `MathBlockExt` block parser in
  `internal/rules/markdownflavor/ext/mathblock.go`
- [x] Write `MathInlineExt` inline parser in
  `internal/rules/markdownflavor/ext/mathinline.go`
- [ ] Write `AbbreviationExt` block parser + paragraph
  transformer in
  `internal/rules/markdownflavor/ext/abbreviation.go`
- [ ] Add tests for all five custom extensions
  <!-- Tests for Super/Subscript and Math block/inline
  landed with their extensions; abbreviation tests
  arrive with that extension. -->

- [x] Build dual parser with built-in extensions (custom
  extensions pending)
- [x] Add AST-based detectors for the seven built-in
  features. Covered here: tables, task lists,
  strikethrough, bare-URL autolinks, footnotes,
  definition lists, and heading IDs. The five custom
  features remain pending.
- [x] Implement `rule.go` with `Check()`; `Fix()` is
  pending
- [x] Implement `rule.Configurable` for MDS034: add
  `ApplySettings` and `DefaultSettings` for `flavor`
- [x] Implement `rule.Defaultable` (`EnabledByDefault`
  returns `false`) so the rule is opt-in
- [x] Register as MDS034 in category `meta`
- [x] Add test fixtures in
  `internal/rules/MDS034-markdown-flavor/` for the seven
  built-in features
- [x] Add rule README and update docs

## Acceptance Criteria

- [ ] `flavor: commonmark` flags tables, task lists,
      strikethrough, bare-URL autolinks, footnotes,
      definition lists, heading IDs, superscript,
      subscript, math blocks, math inline, and
      abbreviations
      <!-- Partial: first seven flagged. Superscript,
      subscript, math blocks, math inline, and
      abbreviations remain pending (custom
      extensions). -->
- [ ] `flavor: gfm` accepts tables, task lists,
      strikethrough, and bare-URL autolinks; flags
      footnotes, definition lists, heading IDs,
      superscript, subscript, math blocks, math
      inline, and abbreviations
      <!-- Partial: acceptance verified; flagging
      verified for footnotes, definition lists, and
      heading IDs. The five custom-extension features
      remain pending. -->
- [ ] `flavor: goldmark` accepts tables, task lists,
      strikethrough, bare-URL autolinks, and heading
      IDs; flags footnotes, definition lists,
      superscript, subscript, math blocks, math
      inline, and abbreviations
      <!-- Partial: acceptance verified; flagging
      verified for footnotes and definition lists.
      The five custom-extension features remain
      pending. -->
- [x] Error messages name the unsupported feature and
      the configured flavor
- [ ] `mdsmith fix` auto-fixes fixable features
      <!-- Fix pipeline not implemented in this
      slice. -->
- [x] Non-fixable features produce diagnostics only
- [x] Invalid flavor name produces a config error
- [x] Rule is disabled by default (opt-in)
- [x] With `flavor: commonmark`, MDS034 reports bare
      URLs as unsupported autolinks
- [x] With `flavor: gfm` or `flavor: goldmark`,
      MDS034 treats bare URLs as supported syntax and
      does not emit a flavor diagnostic for them
- [x] MDS034 does not emit a duplicate bare-URL
      diagnostic when the configured flavor supports
      bare URLs, even if MDS012 still enforces its
      own bare-URL style rule
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
