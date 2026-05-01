---
id: 127
title: Single H1 per file rule
status: "🔲"
summary: >-
  New rule MDS051 that requires at most one top-level
  H1 heading per Markdown file. Closes the gap with
  markdownlint MD025/MD047 and complements MDS004
  (first-line-heading) by catching extra H1s further
  down the document.
model: sonnet
---
# Single H1 per file rule

## Goal

Let users require at most one H1 per file. The H1
typically corresponds to the document title; a
second H1 implies two documents in one file or a
heading-level mistake. markdownlint covers this as
[MD025][md025]; mdsmith has [MDS004][mds004] for the
*first-line* heading but no rule that catches a
*second* H1 later in the file.

## Background

### Relationship to existing rules

- [MDS003][mds003] (heading hierarchy) flags level
  *skips* (`# A` followed by `### B`). It does not
  forbid a repeated `# A` later.
- [MDS004][mds004] (first-line heading) requires the
  first non-front-matter content to be an H1. It
  does not bound the number of H1s.
- [MDS005][mds005] (no duplicate headings) flags two
  headings with the same *text*. Two H1s with
  different text pass MDS005.

MDS051 closes the remaining hole: file-level
uniqueness of the H1 *level*, irrespective of text.

### Front matter title

Some files set the title in YAML front matter
(`title:`) and skip the H1 entirely. MDS051 should
not require an H1 to exist (that's MDS004's job) —
it only caps the count when one or more are present.

## Design

### Configuration

```yaml
rules:
  single-h1:
    front-matter-title: title
```

Category: `heading`. Disabled by default (opt-in).

`front-matter-title` names the front-matter field
that, when present, *also* counts as an H1 for the
purposes of this rule. When the field is set and the
document also begins with an H1, the in-document H1
is treated as the second H1 and flagged. Set to the
empty string to disable this behavior.

### Detection

Walk `*ast.Heading`. Maintain a counter of nodes
where `node.Level == 1`. After the walk:

1. If the counter is zero, no diagnostic.
2. If the counter is one and front-matter title is
   absent (or `front-matter-title: ""`), no
   diagnostic.
3. If the counter is one and front-matter title is
   present, emit on the H1 node.
4. If the counter is greater than one, emit on the
   second and every subsequent H1 node.

### Auto-fix

Demote the offending H1 to H2 (`#` → `##`). This is
a one-character source rewrite that preserves the
heading text. It is safe even when the heading is
setext (`Title\n=====`): rewrite the underline from
`=` to `-`.

When the violation is the *first* H1 conflicting
with a front-matter title, the right fix is usually
to drop the in-document H1 entirely. Auto-fix is not
applied in that case — emit the diagnostic only.

### Error messages

```text
extra H1 heading; only one H1 is allowed per file
h1 heading conflicts with front-matter title
```

## Tasks

1. Scaffold `internal/rules/singleh1/` with
   `rule.go`, `rule_test.go`, and the `init()`
   `rule.Register` call.
2. Implement `Check()` walking `*ast.Heading` and
   counting `Level == 1` nodes.
3. Implement `rule.Configurable` for the
   `front-matter-title` field name.
4. Implement `Fix()` that demotes the second-and-
   later H1 nodes to H2 (ATX or setext).
5. Implement `rule.Defaultable` returning `false`.
6. Register as MDS051 in category `heading`.
7. Add fixture tests in
   `internal/rules/MDS051-single-h1/` covering:
   single H1 (clean), zero H1 with front-matter
   title (clean), zero H1 without title (clean), two
   H1s (second flagged + demoted), three H1s (second
   and third flagged), H1 in setext form (demoted to
   setext H2), H1 conflicting with front-matter
   `title:` (flagged, not auto-fixed), and
   `front-matter-title: ""` (front-matter ignored).
8. Add rule README following the MDS004 template.

## Acceptance Criteria

- [ ] A file with one H1 and several H2/H3 headings
      emits no diagnostic.
- [ ] A file with no headings emits no diagnostic.
- [ ] A file with two H1s emits one diagnostic on
      the second H1; auto-fix demotes it to H2.
- [ ] A file with three H1s emits two diagnostics;
      auto-fix demotes the second and third.
- [ ] A setext H1 (`Title\n====`) past the first H1
      is auto-fixed to setext H2 (`Title\n----`).
- [ ] A file with front-matter `title: Foo` and an
      in-document H1 emits one diagnostic with the
      "conflicts with front-matter title" message;
      no auto-fix is applied.
- [ ] Setting `front-matter-title: ""` suppresses
      the front-matter check; an in-document H1
      passes regardless of YAML.
- [ ] Rule is disabled by default.
- [ ] MDS004 (first-line-heading) still passes on
      the same fixtures.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes on the repo with the
      rule disabled (no regression for existing
      docs).
