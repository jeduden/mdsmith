---
id: 88
title: TOC directive migration aid
status: "🔲"
summary: >-
  New rule MDS035 that flags renderer-specific
  table-of-contents directives (`[TOC]`,
  `[[_TOC_]]`, `[[toc]]`, `${toc}`) which render
  as literal text on CommonMark / goldmark
  instead of expanding into a TOC. The
  diagnostic points authors at mdsmith's
  `<?catalog?>` directive for the file-index use
  case; heading-level TOCs have no direct
  mdsmith equivalent.
---
# TOC directive migration aid

## Goal

Catch renderer-specific TOC directives that do
not expand into a TOC on CommonMark or
goldmark. The diagnostic tells authors which
use case has a mdsmith equivalent and which
does not.

## Context

Four TOC directive variants appear in the wild:

- `[TOC]` — Python-Markdown, MultiMarkdown,
  Pandoc (with `--toc`)
- `[[_TOC_]]` — GitLab Flavored Markdown,
  Azure DevOps
- `[[toc]]` — markdown-it-toc-done-right,
  VitePress
- `${toc}` — some VitePress configurations

None are standardized TOC directives in
CommonMark, GFM, or goldmark. `[TOC]` is still
valid CommonMark syntax — a shortcut reference
link — so it only renders as literal text when
no matching `[TOC]: <url>` definition is
present; with a definition, it renders as a
link. The other three tokens are not valid
link-reference shorthand and always render as
literal text. On those renderers no token in
this set expands into a generated TOC.

Concrete render outputs when no link reference
is present:

- `[TOC]` renders as the literal string `[TOC]`
  (goldmark emits a "no matching link
  reference" fallback, which is verbatim text)
- `[[_TOC_]]` renders as `[[_TOC_]]` inside a
  paragraph
- `[[toc]]` renders as `[[toc]]` inside a
  paragraph
- `${toc}` renders as `${toc}` inside a
  paragraph

The author intended a generated table of
contents; the reader sees the directive token
instead. This is a visible failure, not a
silent one, but it is still a failure worth
catching at lint time.

### Heading TOC vs file index

The flagged directives and
[`<?catalog?>`][catalog] solve different
problems:

| Directive      | Generates                                 | Input              |
|----------------|-------------------------------------------|--------------------|
| `[TOC]` et al. | Table of **headings in the current file** | Current doc        |
| `<?catalog?>`  | Table of **other files** matching a glob  | Glob + frontmatter |

[catalog]: ../internal/rules/MDS019-catalog/README.md

`<?catalog?>` is the right replacement only
when a directive is used on an index page to
list sibling or child documents (e.g. a wiki
homepage with `[[_TOC_]]` listing all pages in
the space). For in-document heading TOCs — the
more common case — mdsmith has no built-in
generator; the author must either drop the
directive or maintain a manual list.

### Why this rule, not MDS034

MDS034 ([plan 86](86_markdown-flavor-validation.md))
validates syntax support against a declared
flavor. TOC directives are not "flavor features"
— they are per-renderer conventions with no
canonical spec and no fix path that applies to
every call site. A dedicated opt-in rule with a
diagnostic tailored to the use-case distinction
above is a better fit than folding them into
MDS034's fix pipeline.

### Scope

Flag only the four directives above. Do not try
to auto-generate a `<?catalog?>` block — the
right glob and front-matter fields depend on the
project and are not knowable from the TOC call
site. The diagnostic is informational and names
both the file-index case (points to MDS019) and
the heading-TOC case (no equivalent).

## Design

### Detection

Line-level regex on the raw source, scoped to
paragraph nodes (skip code blocks, HTML blocks,
and inline code spans). Four patterns:

- `^\[TOC\]\s*$`
- `^\[\[_TOC_\]\]\s*$`
- `^\[\[toc\]\]\s*$`
- `^\$\{toc\}\s*$`

Goldmark parses `[TOC]` as a link reference node
and `[[_TOC_]]` / `[[toc]]` as text inside a
paragraph. AST detection would require
per-variant walkers; raw-line regex is simpler
and avoids false positives by restricting the
match to paragraph-only regions.

#### Link reference exception for `[TOC]`

`[TOC]` is syntactically a valid CommonMark
shortcut reference link. If the document
contains a matching link reference definition
(`[TOC]: <url>`), `[TOC]` resolves to a
legitimate link and must not be flagged.

Before emitting a diagnostic for the `[TOC]`
pattern, consult the goldmark parser context's
link reference map for a definition with the
label `TOC` (case-insensitive, per the
[CommonMark matching rules][cm-refs]). If one
exists, suppress the diagnostic.

[cm-refs]: https://spec.commonmark.org/0.31.2/#matches

The other three patterns do not have this
ambiguity: `[[_TOC_]]`, `[[toc]]`, and `${toc}`
do not form valid link references in CommonMark
and always render as literal text in a
paragraph. No exception handling is needed for
them.

### Configuration

Rule `toc-directive`, category `meta`, disabled
by default (opt-in) — consistent with MDS034's
opt-in posture. No settings.

### Error message

Templated on the matched directive token, so
each variant produces its own message. Given a
matched token `{directive}`:

```text
unsupported TOC directive `{directive}`; mdsmith has no heading TOC equivalent; use `<?catalog?>` for file indexes (MDS019)
```

Example messages the rule would emit for each
of the four patterns:

```text
unsupported TOC directive `[TOC]`; mdsmith has no heading TOC equivalent; use `<?catalog?>` for file indexes (MDS019)
unsupported TOC directive `[[_TOC_]]`; mdsmith has no heading TOC equivalent; use `<?catalog?>` for file indexes (MDS019)
unsupported TOC directive `[[toc]]`; mdsmith has no heading TOC equivalent; use `<?catalog?>` for file indexes (MDS019)
unsupported TOC directive `${toc}`; mdsmith has no heading TOC equivalent; use `<?catalog?>` for file indexes (MDS019)
```

The leading word is lowercase. No trailing
punctuation, per [CLAUDE.md](../CLAUDE.md).
Both the matched token and the replacement
`<?catalog?>` are backticked so they read as
quoted directives, not prose.

Severity: `warning`.

### No auto-fix

The rule is detection-only. Whether the right
replacement is `<?catalog?>`, a manually
maintained list, or deletion depends on intent
that is not recoverable from the directive
alone.

## Tasks

1. Create `internal/rules/MDS035-toc-directive/`
   with `rule.go`, `README.md`
2. Implement paragraph-scoped line scanning for
   the four directive patterns
3. For the `[TOC]` pattern, consult the goldmark
   parser context's link reference definition
   map; suppress the diagnostic when a label
   `TOC` (case-insensitive) is defined
4. Implement `rule.Defaultable` with
   `EnabledByDefault` returning `false`
5. Register as MDS035 in category `meta`
6. Add good/bad fixtures with front-matter
   specifying the expected diagnostics, including
   a good fixture that has `[TOC]: https://x` as
   a reference definition alongside a `[TOC]`
   line
7. Document the rule in the flavor comparison
   table in
   [docs/background/markdown-linters.md](../docs/background/markdown-linters.md)

## Acceptance Criteria

- [ ] `[TOC]` on its own line produces a
      diagnostic that names both the heading-TOC
      gap and the `<?catalog?>` file-index
      alternative
- [ ] `[[_TOC_]]` on its own line produces the
      same diagnostic
- [ ] `[[toc]]` on its own line produces the
      same diagnostic
- [ ] `${toc}` on its own line produces the
      same diagnostic
- [ ] `[TOC]` inside a fenced code block
      produces no diagnostic
- [ ] `[TOC]` inside an inline code span
      produces no diagnostic
- [ ] `[TOC]` used as legitimate link text
      (with a matching `[TOC]: url` definition)
      produces no diagnostic
- [ ] Rule is disabled by default (opt-in)
- [ ] No auto-fix is applied
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
