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

Let users declare a target Markdown flavor so mdsmith
can flag syntax that the target renderer will not
understand, preventing silent rendering failures when
files move between tools.

## Background

### Common Markdown flavors

Different renderers support different supersets of
Markdown. The most common flavors, ordered roughly by
adoption:

| Flavor | Base | Key extensions | Used by |
|---|---|---|---|
| CommonMark | --- | strict baseline spec | many renderers |
| GFM | CommonMark | tables, task lists, strikethrough, autolinks | GitHub, VS Code |
| Goldmark (default) | CommonMark | tables, strikethrough, task lists, linkify | Hugo, mdsmith |
| PHP Markdown Extra | original | footnotes, abbreviations, definition lists, fenced code, tables, heading IDs | WordPress plugins |
| Pandoc | CommonMark | footnotes, definition lists, math, citations, superscript, subscript, heading IDs | academic, Pandoc |
| MyST | CommonMark | directives, roles, cross-references, math | Sphinx, Jupyter Book |
| MultiMarkdown | original | metadata, footnotes, citations, math, tables, cross-references | MultiMarkdown app |
| GitLab | GFM | math blocks, diagrams (Mermaid), footnotes, inline diff | GitLab |
| R Markdown | Pandoc | code chunks, inline R, output formats | RStudio, knitr |

### Syntax differences that matter

These are the concrete features that differ across
flavors and can be detected at the AST or text level:

| Feature | CommonMark | GFM | Goldmark | Pandoc | PHP Extra |
|---|---|---|---|---|---|
| Tables | no | yes | yes | yes | yes |
| Task lists | no | yes | yes | no | no |
| Strikethrough `~~x~~` | no | yes | yes | yes | no |
| Autolinks (bare URLs) | no | yes | yes | no | no |
| Footnotes `[^1]` | no | no | ext | yes | yes |
| Definition lists | no | no | no | yes | yes |
| Math `$x$` / `$$x$$` | no | partial | ext | yes | no |
| Heading IDs `{#id}` | no | no | ext | yes | yes |
| Abbreviations | no | no | no | no | yes |
| Superscript `^x^` | no | no | no | yes | no |
| Subscript `~x~` | no | no | no | yes | no |
| Pipe tables only | --- | yes | yes | yes | yes |
| Grid tables | no | no | no | yes | no |

"ext" means available as a Goldmark extension but not
enabled by default.

### Detection approach

mdsmith's main parser (`internal/lint/file.go`)
uses only default CommonMark block/inline parsers
plus the custom PI parser. No goldmark extensions
are enabled -- not even tables or strikethrough.
This means the current AST represents pure CommonMark;
extension syntax (tables, footnotes, etc.) appears
as plain paragraph text.

Three approaches were evaluated:

**A. Enable all extensions in the main parser.**
Precise AST detection, but changes the parse tree
for every rule. A pipe-table that currently parses
as a paragraph would become a `Table` node, risking
breakage in MDS001-MDS033. Rejected.

**B. Regex-only on raw source.** Zero parser impact,
but produces false positives inside fenced code
blocks, inline code spans, and HTML comments. Would
need to re-implement code-block-awareness. Fragile
for ambiguous syntax like `:` (definition lists) and
`$` (math). Rejected as primary approach.

**C. Dual parser (chosen).** MDS034 creates a second
goldmark parser with all available extensions enabled
and re-parses the source. The main parse tree used by
other rules is untouched. This gives precise AST
detection without risk to existing rules.

For the four features without a goldmark built-in
extension, three approaches were compared:

1. **Third-party extensions.** Existing packages
   (`litao91/goldmark-mathjax`, `bowman2001/
   goldmark-supersubscript`, `zmtcreative/
   gm-abbreviations`) are abandoned or have zero
   adoption. Hugo's `hugo-goldmark-extensions` is
   actively maintained but tightly coupled to Hugo's
   rendering pipeline. None are suitable as
   dependencies.

2. **Custom goldmark extensions (chosen for 4).**
   Based on the existing PI block parser (~160 LOC),
   the cost per extension is:
   - Inline delimiter (`^sup^`, `~sub~`): ~200 LOC
     with tests
   - Block fence (`$$...$$`): ~200 LOC with tests
   - Inline delimiter with flanking rules (`$...$`):
     ~250 LOC with tests (Pandoc-style: `$` must be
     preceded by whitespace/start-of-line and not
     followed by whitespace)
   - Block def + inline (`*[abbr]: ...`): ~400 LOC
     with tests

3. **Regex on raw source (chosen for 1).**
   Abbreviation definitions (`*[abbr]: ...`) are
   unambiguous at line-start. We only need to detect
   the definition line to flag "you're using
   abbreviation syntax" -- not every inline occurrence.
   A full abbreviation extension (~400 LOC) would need
   block-level definitions plus inline text
   replacement, which is disproportionate for
   detection-only.

Math-inline cannot use regex: `$` is common in prose
(`costs $5`), environment variables (`$PATH`), and
code. A regex cannot distinguish `$x+y$` from
`costs $5 and $10` without delimiter-state tracking.
A parser extension with Pandoc-style flanking rules
solves this correctly.

Final detection strategy per feature:

| Feature | Goldmark built-in | Detection |
|---|---|---|
| Tables | `extension.Table` | AST |
| Strikethrough | `extension.Strikethrough` | AST |
| Task lists | `extension.TaskList` | AST |
| Footnotes | `extension.Footnote` | AST |
| Definition lists | `extension.DefinitionList` | AST |
| Heading IDs | `parser.WithAttribute()` | AST |
| Autolinks | `extension.Linkify` | AST |
| Superscript | custom inline ext | AST |
| Subscript | custom inline ext | AST |
| Math block | custom block ext | AST |
| Math inline | custom inline ext | AST |
| Abbreviations | none | regex |

Custom extensions for superscript, subscript,
math-block, and math-inline (4 total, ~850 LOC with
tests). Regex only for abbreviation definitions --
the definition syntax `*[abbr]:` is unambiguous at
block level, and detecting usage sites would require
full abbreviation expansion (not needed for
flavor-checking).

The dual-parser cost is one extra parse per file, only
when MDS034 is enabled. Since MDS034 is opt-in and
disabled by default, most users pay zero cost.

### Scoping: which flavors to support initially

Start with the three most common targets:

- **commonmark** -- strict CommonMark spec
- **gfm** -- GitHub Flavored Markdown
- **goldmark** -- Goldmark with default extensions

These cover the vast majority of use cases. Additional
flavors (Pandoc, PHP Extra, MyST) can be added later
by extending the feature table.

## Design

### New rule: MDS034 `markdown-flavor`

Category: `meta`. Enabled by default: no (opt-in).

#### Configuration

```yaml
rules:
  markdown-flavor:
    flavor: gfm  # commonmark | gfm | goldmark
    # Future: allow per-feature overrides
    # allow:
    #   - footnotes
    # deny:
    #   - autolinks
```

#### Flavor feature registry

Internal data structure mapping each flavor to its
supported feature set:

```go
// internal/rules/markdownflavor/features.go

type Feature string

const (
    Tables        Feature = "tables"
    TaskLists     Feature = "task-lists"
    Strikethrough Feature = "strikethrough"
    Autolinks     Feature = "autolinks"
    Footnotes     Feature = "footnotes"
    DefinitionLists Feature = "definition-lists"
    MathInline    Feature = "math-inline"
    MathBlock     Feature = "math-block"
    HeadingIDs    Feature = "heading-ids"
    Abbreviations Feature = "abbreviations"
    Superscript   Feature = "superscript"
    Subscript     Feature = "subscript"
)

type Flavor struct {
    Name     string
    Features map[Feature]bool
}
```

Each flavor is a set of booleans. Checking is: walk
the document, identify which features are used, report
any feature where `flavor.Features[f] == false`.

#### Dual parser setup

MDS034 builds a second goldmark parser in its
`Check()` method with all built-in extensions plus
four custom detection-only extensions:

```go
p := goldmark.New(
    goldmark.WithExtensions(
        // Built-in extensions
        extension.Table,
        extension.Strikethrough,
        extension.TaskList,
        extension.Footnote,
        extension.DefinitionList,
        extension.Linkify,
        // Custom detection-only extensions
        &SuperscriptExt{},
        &SubscriptExt{},
        &MathBlockExt{},
        &MathInlineExt{},
    ),
    goldmark.WithParserOptions(
        parser.WithAttribute(),
    ),
)
```

This parser is created once per rule instance (not
per file) and reused across calls.

#### Custom goldmark extensions (4)

Four small detection-only extensions, ~850 LOC total
with tests. They live in
`internal/rules/markdownflavor/ext/` and follow
the same pattern as the existing PI block parser
(`internal/lint/pi.go`):

**SuperscriptExt** -- inline delimiter parser for
`^text^`. Implements `parser.InlineParser` with
trigger byte `^`. Produces `KindSuperscript` AST
nodes. ~100 LOC parser + ~50 LOC node definition.

**SubscriptExt** -- inline delimiter parser for
`~text~`. Implements `parser.InlineParser` with
trigger byte `~`. Must distinguish single `~` from
double `~~` (strikethrough). ~120 LOC parser + ~50
LOC node definition.

**MathBlockExt** -- block parser for `$$...$$`
fences. Implements `parser.BlockParser` with trigger
byte `$`. Produces `KindMathBlock` AST nodes.
Modeled on goldmark's built-in fenced-code-block
parser. ~110 LOC parser + ~50 LOC node definition.

**MathInlineExt** -- inline delimiter parser for
`$...$`. Implements `parser.InlineParser` with
trigger byte `$`. Uses Pandoc-style flanking rules:
opening `$` must not be followed by whitespace,
closing `$` must not be preceded by whitespace, and
the opening `$` must be preceded by whitespace,
start-of-line, or punctuation. This disambiguates
`$x+y$` (math) from `costs $5` (currency).
~150 LOC parser + ~50 LOC node definition.

These extensions only need to parse (produce AST
nodes with source positions). They do not need to
render -- mdsmith never renders HTML.

#### AST detectors (11 features)

Walk the extension-aware AST and collect node
locations:

- Tables: `ast.KindTable`
- Strikethrough: `extast.KindStrikethrough`
- Task lists: `extast.KindTaskCheckBox`
- Footnotes: `extast.KindFootnote` and
  `extast.KindFootnoteBacklink`
- Definition lists: `extast.KindDefinitionList`
- Heading attributes: heading nodes with non-empty
  `Attribute()` list
- Autolinks: `extast.KindLinkify` (bare URL nodes
  created by the linkify extension)
- Superscript: custom `KindSuperscript`
- Subscript: custom `KindSubscript`
- Math block: custom `KindMathBlock`
- Math inline: custom `KindMathInline`

Each detector returns `(line, column)` pairs from
the node segment positions.

#### Regex detector (1 feature)

Only abbreviation definitions use regex:

- **Abbreviations** (`*[abbr]: ...`):
  `^\*\[[^\]]+\]:` at line start. The definition
  syntax is unambiguous at block level. A full
  abbreviation extension (~400 LOC) would need block
  definitions plus inline text replacement --
  disproportionate when we only need to detect "this
  file uses abbreviation syntax." We do not attempt
  to detect inline *usage* of abbreviations (every
  occurrence of the abbreviated text) since flagging
  the definition is sufficient.

The code-fence skip logic reuses the fenced-code-block
line ranges from the main AST.

#### Error messages

Messages follow existing mdsmith conventions: state
the concrete constraint, name the feature.

| Feature | Message |
|---|---|
| Tables | `table syntax is not supported by {flavor}; convert to a list or use a supported renderer` |
| Task lists | `task list (checkbox) syntax is not supported by {flavor}` |
| Strikethrough | `strikethrough (~~text~~) is not supported by {flavor}` |
| Autolinks | `bare URL autolink is not supported by {flavor}; wrap in angle brackets or use [text](url)` |
| Footnotes | `footnote syntax is not supported by {flavor}` |
| Definition lists | `definition list syntax is not supported by {flavor}` |
| Math (inline) | `inline math ($...$) is not supported by {flavor}` |
| Math (block) | `block math ($$...$$) is not supported by {flavor}` |
| Heading IDs | `heading ID attribute ({#id}) is not supported by {flavor}` |
| Abbreviations | `abbreviation definition is not supported by {flavor}` |
| Superscript | `superscript (^text^) is not supported by {flavor}` |
| Subscript | `subscript (~text~) is not supported by {flavor}` |

All messages use severity `warning` (the syntax may
still render as plain text, it just won't render as
intended).

#### Auto-fix

Some features have natural downgrades; others do not.

| Feature | Fixable | Fix strategy |
|---|---|---|
| Autolinks | yes | Wrap bare URL in `<url>` or `[url](url)` |
| Strikethrough | partial | Remove `~~` delimiters, leaving plain text |
| Heading IDs | yes | Remove `{#id}` suffix from heading line |
| Tables | no | No lossless downgrade exists |
| Task lists | partial | Replace `- [ ]` / `- [x]` with `- ` / `- (done) ` |
| Footnotes | no | Requires restructuring (inline the note) -- too complex for auto-fix |
| Definition lists | no | Requires restructuring |
| Math | no | No Markdown-only fallback |
| Abbreviations | no | Requires expanding all uses |
| Superscript | partial | Remove `^` delimiters |
| Subscript | partial | Remove `~` delimiters |

The rule implements `FixableRule` for the fixable
subset. Non-fixable features produce diagnostics only.

### Integration with existing rules

- **MDS012 (no-bare-urls)** already flags bare URLs.
  When `markdown-flavor: { flavor: gfm }` is set,
  autolinks are valid GFM. The flavor rule should not
  conflict: MDS012 is about style preference, MDS034
  is about spec compliance. Document the interaction
  in the rule README.
- **MDS002 (heading-style)** is orthogonal -- both ATX
  and setext are valid in all flavors.

## Tasks

1. Add feature enum and flavor registry in
   `internal/rules/markdownflavor/features.go`
2. Write custom `SuperscriptExt` inline parser and
   `KindSuperscript` AST node in
   `internal/rules/markdownflavor/ext/superscript.go`
3. Write custom `SubscriptExt` inline parser and
   `KindSubscript` AST node in
   `internal/rules/markdownflavor/ext/subscript.go`
   (handle `~` vs `~~` disambiguation)
4. Write custom `MathBlockExt` block parser and
   `KindMathBlock` AST node in
   `internal/rules/markdownflavor/ext/mathblock.go`
5. Write custom `MathInlineExt` inline parser and
   `KindMathInline` AST node in
   `internal/rules/markdownflavor/ext/mathinline.go`
   (Pandoc-style flanking: opening `$` not followed
   by whitespace, closing `$` not preceded by
   whitespace, opening preceded by whitespace or
   punctuation or start-of-line)
6. Add tests for the four custom extensions in
   `internal/rules/markdownflavor/ext/*_test.go`;
   math-inline tests must cover `$x+y$` (math) vs
   `costs $5` (currency) vs `$PATH` (env var)
7. Build dual parser: create a second goldmark parser
   with built-in extensions (`Table`, `Strikethrough`,
   `TaskList`, `Footnote`, `DefinitionList`,
   `Linkify`, `WithAttribute`) plus the four custom
   extensions
8. Add AST-based detectors for 11 features (tables,
   task lists, strikethrough, autolinks, footnotes,
   definition lists, heading attributes, superscript,
   subscript, math block, math inline)
9. Add regex-based detector for abbreviation
   definitions (1 feature) with fenced-code-block
   skip logic
9. Implement `rule.go` with `Check()` that re-parses
   with the dual parser, runs all detectors, and
   reports unsupported features
10. Implement `Fix()` for fixable features (autolinks,
    strikethrough, heading IDs, task lists,
    superscript, subscript)
11. Add `Configurable` interface: `ApplySettings` for
    `flavor` string, validate against known flavors
12. Register rule as MDS034 `markdown-flavor` in
    category `meta`
13. Add test fixtures:
    `internal/rules/MDS034-markdown-flavor/bad/` with
    files using each unsupported feature per flavor,
    `good/` with files using only supported features,
    `fixed/` with expected auto-fix output
14. Add rule README following
    `internal/rules/proto.md` schema
15. Document the rule in `docs/reference/cli.md` and
    add a note to
    `docs/guides/directives/enforcing-structure.md`
    about flavor enforcement

## Acceptance Criteria

- [ ] `markdown-flavor: { flavor: commonmark }` flags
      tables, task lists, strikethrough, and autolinks
- [ ] `markdown-flavor: { flavor: gfm }` accepts
      tables, task lists, strikethrough, autolinks but
      flags footnotes, definition lists, math
- [ ] `markdown-flavor: { flavor: goldmark }` accepts
      Goldmark default extensions, flags Pandoc-only
      features
- [ ] Error messages name the specific unsupported
      feature and the configured flavor
- [ ] `mdsmith fix` auto-fixes autolinks (wraps in
      angle brackets) and heading IDs (removes `{#id}`)
- [ ] `mdsmith fix` auto-fixes strikethrough and task
      lists with plain-text fallbacks
- [ ] Non-fixable features (tables, footnotes, math)
      produce diagnostics only, no fix attempted
- [ ] Invalid flavor name in config produces a clear
      configuration error
- [ ] Rule is disabled by default (opt-in)
- [ ] Rule does not conflict with MDS012
      (no-bare-urls) when both are enabled
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
