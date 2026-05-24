---
title: Extract Markdown as data
weight: 35
summary: >-
  When a Markdown file's payload is prose, put it in
  the body under H2 sections — not in YAML
  frontmatter. `mdsmith extract` projects body
  structure into a JSON tree the same way it
  projects frontmatter, so the file stays editable
  as Markdown.
---
# Extract Markdown as data

`mdsmith extract` projects a schema-conformant
Markdown file into a JSON / YAML / msgpack tree. Two
parts of the file feed the tree:

- **Frontmatter** — decoded YAML, written under a
  `frontmatter` key.
- **Body sections** — H1 / H2 / H3 headings and the
  content under them, projected as siblings.

This guide is about when to use which.

## The principle

Frontmatter is for the file's **metadata**: `title`,
`kind`, `status`, dates, cross-references, the
fields a non-prose tool (a workflow, a release
script, a status badge) reads alongside the prose.

Body sections are for the file's **payload**: the
prose, paragraphs, lists, and code blocks the file
exists to hold. If the value is a sentence or two of
copy, it belongs under a heading — not in a folded
YAML scalar.

The trap is to reach for frontmatter for everything
because it's structured. A 60-character `tagline` in
frontmatter and the same 60 characters in a
`## Tagline` body section produce identical JSON;
the body version is shorter to edit, diffs cleanly
when wrapped, and is lintable as Markdown.

## Worked example

A product-copy file with a tagline, a lead, and one
per-surface description.

### Frontmatter-heavy (the trap)

```markdown
---
title: Product copy
tagline: >-
  Mark down your ideas; smith them into shipping
  docs.
lead: >-
  A lint-and-fix tool that keeps your Markdown
  consistent across every surface — READMEs, docs
  site, editor extensions.
vscode-description: >-
  Inline diagnostics, fix-on-save, and instant
  navigation for Markdown in VS Code.
---
# Product copy

This file holds the tagline, lead, and VS Code
description. Edit a field above and re-run the
sync.
```

Three folded scalars (`>-`); the body is bookkeeping;
line breaks inside the scalars are cosmetic
(folded-strip collapses them to spaces); a leading
punctuation character in any value would force
double-quotes.

### Body-structured (the principle)

```markdown
---
title: Product copy
---
# Product copy

## Tagline

Mark down your ideas; smith them into shipping
docs.

## Lead

A lint-and-fix tool that keeps your Markdown
consistent across every surface — READMEs, docs
site, editor extensions.

## VS Code

Inline diagnostics, fix-on-save, and instant
navigation for Markdown in VS Code.
```

With a matching schema in `.mdsmith.yml`:

```yaml
kinds:
  product-copy:
    schema:
      sections:
        - heading: { regex: '^Tagline$' }
        - heading: { regex: '^Lead$' }
        - heading: { regex: '^VS Code$' }
          bind: vscode-description
```

`mdsmith extract product-copy --format json` emits
the same shape both encodings would produce:

```json
{
  "frontmatter": { "title": "Product copy" },
  "tagline": { "text": "Mark down your ideas; smith them into shipping docs." },
  "lead": { "text": "A lint-and-fix tool that keeps your Markdown consistent across every surface — READMEs, docs site, editor extensions." },
  "vscode-description": { "text": "Inline diagnostics, fix-on-save, and instant navigation for Markdown in VS Code." }
}
```

The body version costs nothing at the projection
layer and is the editable artifact.

## When frontmatter is the right call

- **Short scalars where YAML's typing earns its
  keep**: booleans (`draft: true`), dates
  (`published: 2026-05-24`), enums
  (`status: "✅"`), numbers.
- **Metadata other tools read**: `title`, `kind`,
  `weight`, `tags` — anything Hugo's
  frontmatter, a release script, or a status
  dashboard consumes directly.
- **Fields that participate in `<?catalog?>`
  directives**: catalog templating reads
  frontmatter keys (`{title}`, `{summary}`).
- **Strict, machine-controlled values**: a
  generated version stamp, a hash, a per-file
  identifier — values an automated tool writes
  and a human should not edit by hand.

Prose paragraphs, multi-line copy, anything wider
than one line, and anything that benefits from
Markdown formatting (code, emphasis, links) all
belong in the body.

## `bind:` patterns

`bind:` renames the JSON key that a heading or
content entry projects under. Use it when the
human-readable heading and the consumer-friendly key
don't match.

- **Heading-to-key rename.** `## VS Code` slugs to
  `vs-code` by default. Set `bind: vscode-description`
  on the section so the JSON consumer reads
  `vscode-description` (matching the field name in
  the consuming code or manifest).
- **Collapse a wrapper.** `bind: ""` on a parent
  scope hoists its children into the grandparent
  scope. Use it when a heading exists for human
  reading but should not nest in the data tree.
- **Repeating sections.** A section with
  `repeat: {min, max}` and a placeholder-bearing
  heading projects as an array; combine with
  `bind:` to rename the array key.

See [the section-schema reference][secref] for the
full grammar.

[secref]: ../reference/section-schema.md

## See also

- [`mdsmith extract`][extract] — the CLI reference,
  including default projection rules per content
  entry type (code → `code`, list → `items`,
  table → `rows`, paragraph → `text`).
- [Schemas guide][schemas] — declaring the kind
  schema that doubles as the extraction contract.

[extract]: ../reference/cli/extract.md
[schemas]: schemas.md
