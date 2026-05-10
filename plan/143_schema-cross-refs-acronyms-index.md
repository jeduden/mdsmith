---
id: 143
title: Schema cross-references, acronyms, and index
status: "🔲"
model: sonnet
depends-on: [146]
summary: >-
  Add three top-level schema blocks on top of the
  engine from plan 146: `cross-references:` for
  text patterns whose matches must resolve to
  real headings, `acronyms:` for first-use
  detection with a known-safe allowlist, and
  `index:` for an optional JSON side-output that
  `mdsmith fix` regenerates.
---
# Schema cross-references, acronyms, and index

## Goal

Cover the three remaining capabilities from
the S-7 sketch in the
[mdbase research](../docs/research/mdbase-vs-mdsmith/learn-from-mdbase.md).
Text references that must resolve to real
section headings. First-use acronym detection.
An index side-output that downstream tooling
consumes instead of re-parsing.

## Background

Plans 146 and 142 cover what fits inside one
section. The remaining S-7 capabilities span
the document:

- Cross-references resolve across sections.
  "Step 4" in `## If different` must point at
  a real `### Step 4` somewhere.
- Acronyms have a first-use rule that needs
  document-wide state.
- The index is a per-document JSON output
  that downstream tooling reads — runbook
  navigators, RFC tables, dashboards.

## Non-Goals

- Wikilink resolution. Tracked as L-1.
- A backlinks subcommand. Tracked as L-4
  (plan 138).
- A persistent index for cross-document
  references. P-1 / P-2.

## Design

### Cross-references

```yaml
schema:
  cross-references:
    - pattern: "\\bStep (\\d+)\\b"
      must-match: "Step {n}"
      skip-lines-matching: "^> "
```

The validator walks the document's text nodes
once per pattern. For each match, the captured
group fills `{n}` (or `{slug}`) in the
`must-match:` template; the resulting heading
slug must exist in the document. Unresolved
references emit a diagnostic at the source
position.

`skip-lines-matching:` skips lines whose raw
text matches a regex. The intended use is
blockquoted stale text and version-history
notes that mention old step numbers.

### Acronyms

```yaml
schema:
  acronyms:
    known-safe: [API, HTTP, TLS, JSON]
    scope: ["Check", "Expected"]
```

A scoped pass walks the named scopes and flags
any all-caps token (length 2-6, alphanumeric,
not in `known-safe`) on its first appearance
without a parenthesized expansion. "OIDC" on
first use without "(OpenID Connect)" produces a
diagnostic; "OIDC (OpenID Connect)" passes.

`scope:` is a list of section names (matching
the `heading:` text in plan 146's section
tree); the rule applies only inside those
scopes. Omitting `scope:` applies it
document-wide.

### Index side-output

```yaml
schema:
  index:
    output: ".runbook-index.json"
    include: [step-map, cross-ref-graph, word-counts]
```

`mdsmith fix` writes the index alongside the
source. `mdsmith check` does not write the
file (consistent with check's read-only
contract). The index is regenerable, so it is
safe to gitignore.

`include:` is a closed enum:

- `step-map` — `{section-slug: [child-slugs]}`
- `cross-ref-graph` — `{ref: target-slug}`
- `word-counts` — `{section-slug: int}`
- `headings` — flat list of
  `{level, text, slug, line}`

The set is closed so downstream tooling can
parse the file without a schema reference.
Adding an entry is one schema-engine change
plus one struct field.

### Composition

The three blocks are independent. A schema can
ship cross-references without an index, an
index without acronyms, and so on. Each block
runs in its own pass after the section-tree
walk completes.

## Tasks

1. Extend `internal/schema/Schema` with
   `CrossReferences []CrossRef`, `Acronyms`,
   and `Index`.
2. Implement cross-reference validation. One
   AST text-node walk per pattern. Resolve
   captured groups against the document's
   heading slugs via mdsmith's existing slug
   helper. Apply `skip-lines-matching:`.
3. Implement acronym tracking. Tokenizer
   shared with
   [`internal/lint`](../internal/lint).
   Per-scope first-use state. A scope of
   `["Check"]` matches every section whose
   `heading:` is `Check`.
4. Implement index emission. Triggered only
   by `mdsmith fix`. Path is relative to the
   document. Atomic write.
5. Document every block in the
   [MDS020 README](../internal/rules/MDS020-required-structure/README.md)
   and `docs/guides/schemas.md`.
6. Add fixtures: a document with valid and
   broken cross-references; a document with
   safe and flagged acronym uses; a document
   that should produce an index file.
   Verify the index JSON shape against a
   golden file.

## Acceptance Criteria

- [ ] A `cross-references:` entry flags an
      unresolved reference (e.g. "Step 7"
      with no `Step 7` heading anywhere in
      the document).
- [ ] A `cross-references:` entry passes a
      reference to an existing heading.
- [ ] `skip-lines-matching:` skips
      blockquoted lines from the resolution
      check.
- [ ] An `acronyms:` block flags a first-use
      without expansion and passes a
      `known-safe` entry.
- [ ] An `acronyms:` block scoped to one
      heading does not fire on text outside
      that scope.
- [ ] `mdsmith fix` writes the index file
      named in `index.output:`.
- [ ] `mdsmith check` does not write the
      index file even when `index:` is set.
- [ ] The index JSON matches the documented
      shape for every `include:` entry.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues.
