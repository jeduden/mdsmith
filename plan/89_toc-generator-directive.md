---
id: 89
title: TOC generator directive and MDS035 auto-fix
status: "✅"
summary: >-
  Add a `<?toc?>...<?/toc?>` generated-section
  directive that emits a nested list of the
  current document's headings linked to their
  anchors (MDS038). Upgrade MDS035 (plan 88) to
  auto-fix each detected renderer-specific TOC
  token by replacing it with a `<?toc?>` block,
  which the directive then regenerates on
  `mdsmith fix`.
---
# TOC generator directive and MDS035 auto-fix

## Goal

Give mdsmith a native heading-level TOC
generator so MDS035's detection can be followed
by an actual fix. Today MDS035 only flags
`[TOC]`, `[[_TOC_]]`, `[[toc]]`, `${toc}` and
points at `<?catalog?>`, which is a file-index
directive, not a heading-outline replacement.

## Context

Plan 88 landed MDS035 as detection-only because
mdsmith had no equivalent for in-document
heading TOCs — the common case of those tokens.
`<?catalog?>` lists *other* files matching a
glob, so it is the right replacement only on
index pages, not on the per-document TOCs
authors usually want.

mdsmith already has the generated-section
machinery needed here.
[`<?catalog?>`][catalog] and
[`<?include?>`][include] each read a directive
body, compute content, and emit it between
`<?name ...?>` and `<?/name?>` markers. A rule
keeps the content current on `mdsmith fix`.
See the [generated-section archetype][gensection]
for the shared mechanics.

[catalog]: ../internal/rules/MDS019-catalog/README.md
[include]: ../internal/rules/MDS021-include/README.md
[gensection]: ../docs/background/concepts/generated-section.md

### Why both pieces in one plan

The `<?toc?>` directive is only useful to
MDS035 if MDS035 knows to rewrite its detected
tokens to it; conversely, MDS035's auto-fix is
only possible once `<?toc?>` exists. Splitting
the work across two plans would leave one half
dead code until the other lands.

## Design

### Directive syntax

```text
<?toc
min-level: 2
max-level: 3
?>
- [First heading](#first-heading)
  - [Subheading](#subheading)
- [Second heading](#second-heading)
<?/toc?>
```

Parameters (all optional):

| Name        | Type | Default | Description                                         |
|-------------|------|---------|-----------------------------------------------------|
| `min-level` | int  | `2`     | Lowest heading level to include (1–6)               |
| `max-level` | int  | `6`     | Highest heading level to include (1–6, ≥ min-level) |

`min-level: 2` excludes the document title by
default, matching what Python-Markdown's `[TOC]`
produces with default settings.

### Generated content

A nested unordered list, one item per heading
in source order. Each item links to the same
heading anchor mdsmith already computes for
cross-file heading references.

Do not define a TOC-specific slug algorithm.
Reuse the `slugify` function and the
duplicate-disambiguation logic in
[`internal/rules/crossfilereferenceintegrity/rule.go`][cfri]
(used by `collectHeadingAnchors`). Move it
into a shared package. `internal/mdtext/` is
the natural home. Both rules then call shared
code so anchors stay consistent. Repeated
headings get `-1`, `-2`, … suffixes in source
order, matching the reference-integrity rule.

[cfri]: ../internal/rules/crossfilereferenceintegrity/rule.go

Indentation uses the same per-level indent as
the MDS016 `list-indent` rule's `spaces`
setting (default 2). There is no
`<?listindent?>` directive; indentation is a
property of the emitted output, not a
configurable parameter of `<?toc?>`.

List items respect the heading structure, not
the raw level. Given H2 → H4 → H2, the tree is:

```text
- [h2a](#h2a)
  - [h4](#h4)
- [h2b](#h2b)
```

…not a flat list keyed on absolute level.

### Rule: MDS038 (toc)

Note on numbering: plan 89 originally claimed
MDS036, but plan 51 shipped first and took
MDS036 for `max-section-length`. MDS034 was
reserved for plan 86 (markdown-flavor). MDS037
was taken by plan 50 (duplicated-content).
Plan 89 takes MDS038.

- ID: `MDS038`
- Name: `toc`
- Category: `meta`
- Default: enabled (generated sections are
  enabled by default for the project; users opt
  out by removing the directive)
- Fixable: yes

`Check` diff-compares the body between the
markers against the regenerated output.
`Fix` rewrites the body.

Use the shared `internal/archetype/gensection`
engine that `catalog` already uses. New
directive logic lives in
`internal/rules/toc/`.

### MDS035 auto-fix

MDS035 becomes a `FixableRule`. On `Fix`, for
each matched directive line inside a paragraph:

- `[TOC]` (unresolved), `[[_TOC_]]`, `[[toc]]`,
  `${toc}` → replace the single line with the
  canonical empty generated-section block:

  ```text
  <?toc?>
  <?/toc?>
  ```

  The empty body means "use defaults". MDS035's
  Fix must also insert surrounding blank lines
  when the directive would otherwise fuse into
  adjacent paragraph text.

- `[TOC]` with a matching link reference
  definition → leave untouched (already
  suppressed by Check; must stay out of Fix).

The Fix output is plain `<?toc?>...<?/toc?>`
with an **empty** body. MDS038 runs in a
subsequent pass of the same `mdsmith fix`
invocation (mdsmith already supports
multi-pass fix, used by MDS019/MDS021) and
fills in the heading list. If the one-pass
semantics become brittle, an alternative is for
MDS035 to call into `gensection` directly and
emit populated content; start with multi-pass
and fall back only if needed.

### Interaction with existing rules

- MDS015 only enforces blank lines around
  fenced code blocks, not generated-section
  markers. Blank-line padding around inserted
  `<?toc?>` blocks is MDS035's responsibility
  at Fix time, not MDS015's.
- MDS020 (required-structure): no change; TOC
  blocks are not a required section.
- MDS019 (catalog), MDS021 (include): orthogonal
  — different directive names.

### Out of scope

- Custom link anchor format overrides
  (e.g., non-GitHub slugger). Always GitHub-
  style for the first release; add a
  `slugger:` parameter later if needed.
- Skipping specific headings via frontmatter
  or attribute syntax. Use `min-level` /
  `max-level` for now.
- Rendering `[TOC]` into `<?toc?>` with
  preserved non-default parameters. The four
  renderer-specific tokens have no shared
  parameter surface; always emit the default
  `<?toc?>`.

## Tasks

1. [x] Move `slugify` and the duplicate-anchor
   counter from
   `internal/rules/crossfilereferenceintegrity/`
   into a shared helper in `internal/mdtext/`.
   Update the reference-integrity rule to call
   the shared helper. No behavior change.
2. [x] Create the `<?toc?>` directive in a new
   `internal/rules/toc/` package using the
   shared `internal/archetype/gensection.Engine`
   (same engine as `catalog`) and the shared
   slug helper from task 1. Register as MDS038
   in category `meta`, enabled by default,
   `FixableRule`.
3. [x] Add MDS038 fixtures under
   `internal/rules/MDS038-toc/`: `good/` with a
   correct body, `bad/` with a stale body to
   verify Check, `fixed/` with the expected
   output after Fix. Cover default parameters,
   custom `min-level`/`max-level`, single-level
   docs, and deeply nested structures.
4. [x] Upgrade MDS035 to `FixableRule`: replace
   matched directive lines with the canonical
   empty block `<?toc?>\n<?/toc?>`, inserting
   blank lines above and below as needed; leave
   `[TOC]` untouched when a link-ref definition
   suppresses Check; add `fixed/` fixtures for
   each of the four variants.
5. [x] Update MDS035's diagnostic message to
   point at `<?toc?>` (MDS038) instead of
   `<?catalog?>` (MDS019); wording:
   `unsupported TOC directive \`{token}\`; use \`<?toc?>\` (MDS038)`
6. [x] Update MDS035 README (message + examples)
   and the renderer-portability section in
   [docs/background/markdown-linters.md][lnt].
   (No plan 88 file exists; MDS035 was shipped
   inline.)
7. [x] Update the generated-section archetype doc
   to list `toc` alongside `catalog` and
   `include`.
8. [x] Verify multi-pass `mdsmith fix` end-to-end:
   starting from a document containing `[TOC]`,
   a single `fix` run must yield a populated
   `<?toc?>...<?/toc?>` block. Add an
   integration test asserting this.

[lnt]: ../docs/background/markdown-linters.md

## Acceptance Criteria

- [x] `<?toc?>...<?/toc?>` in a document
      regenerates on `mdsmith fix` with a
      nested list of headings linked to
      GitHub-style slugs
- [x] `min-level` and `max-level` parameters
      gate which headings appear
- [x] `<?toc?>` with a stale body produces
      an MDS038 diagnostic on `check`
- [x] MDS035 `fix` rewrites `[TOC]`,
      `[[_TOC_]]`, `[[toc]]`, and `${toc}`
      on their own line to
      `<?toc?>\n<?/toc?>` blocks
- [x] MDS035 `fix` leaves `[TOC]` untouched
      when a matching link reference
      definition is present
- [x] A single `mdsmith fix` run converts a
      source containing `[TOC]` into a source
      containing a populated `<?toc?>` block
- [x] Merge driver regenerates `<?toc?>`
      bodies on conflict, same as `<?catalog?>`
      (inherited from the shared gensection engine)
- [x] MDS035 diagnostic message names
      `<?toc?>` (MDS038) as the replacement
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports
      no issues
