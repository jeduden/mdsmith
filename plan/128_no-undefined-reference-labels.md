---
id: 128
title: Reject undefined reference-link labels
status: "🔲"
summary: >-
  New rule MDS053 that flags reference-style links and
  images whose label has no matching link reference
  definition in the file. Closes the parity gap with
  markdownlint MD052 and catches a class of broken link
  that MDS027 silently misses today.
model: sonnet
---
# Reject undefined reference-link labels

## Goal

Flag `[text][label]`, `[label][]`, and `![alt][label]`
forms whose `label` has no matching `[label]: url`
definition in the same file. These are broken links.
They render as literal text in most viewers (GitHub
keeps the brackets visible). They currently pass
mdsmith's lint without any diagnostic.

## Background

### Why MDS027 misses this

[MDS027][mds027] (cross-file-reference-integrity) walks
`*ast.Link` nodes and verifies the destination resolves
on disk. goldmark only constructs an `*ast.Link` for a
reference-style usage *when a matching link reference
definition exists*. When the definition is missing,
goldmark leaves the bracketed text as plain
`*ast.Text`. MDS027's walk never sees the broken link.
No diagnostic is emitted.

The `docs/background/markdown-linters.md` comparison
page surfaced this failure. It contained
`[plan 12N][planNNN]` references with no matching
`[planNNN]: ...` definition. The lint passed.
markdownlint's [MD052][md052] catches this case.

### Why a separate rule

[Plan 107][plan107] proposes forbidding reference-style
links entirely (`no-reference-style`). That is a
stylistic policy. Some projects opt out (including
the comparison page in this repo) because reference
style keeps wide tables readable.

MDS053 must work *with* reference-style links by
verifying their labels resolve. The two rules compose.
Plan 107 lets you ban reference style; MDS053 lets
projects that allow reference style catch typos.

### CommonMark normalization

CommonMark normalizes reference labels. It folds
case, collapses inner whitespace, and trims the
ends. goldmark's `parser.Context.Reference(label)`
takes a normalized label. The rule must normalize the
same way. Else `[Foo][BAR]` matching `[bar]: ...`
flags as a false positive.

### Shortcut references

`[label]` (with no following `[ref]` or `(url)`) is a
*shortcut* reference. The bracketed text is also the
label. The rule should treat shortcut references the
same as `[label][]` collapsed references. Flag
when the label has no definition.

False positives are a real concern. `[just brackets]`
in prose is technically a shortcut reference under
CommonMark. Forbidding all of them would be too
aggressive. The mitigation: only flag shortcut
references when the surrounding text suggests link
intent. For example, when the bracketed text contains
characters typical of an anchor label (digits,
hyphens, underscores), or is followed by `[]`. The
default behavior is:

1. Always flag `[text][label]` and `[label][]` with no
   matching definition (these are unambiguously link
   syntax).
2. Flag bare `[label]` only when the label looks like
   a reference target, *or* when configured to do so.

A `shortcut` setting (`always` | `collapsed-only` |
`heuristic`, default `heuristic`) gives users control.

## Design

### Configuration

```yaml
rules:
  no-undefined-reference-labels:
    shortcut: heuristic
    placeholders: []
```

Category: `link`. Enabled by default.

`placeholders` follows the [placeholder
grammar][placeholder-grammar] convention used by
MDS027 and other link rules. A label whose normalized
text matches a configured placeholder token is treated
as opaque and never flagged.

### Detection

1. Re-use the lint parser context obtained from
   `lint.NewParser()` so processing-instruction
   blocks are excluded the same way other PI-aware
   rules handle them. This mirrors
   `tocdirective.hasTOCLinkReference`.
2. Walk the AST and collect every `*ast.Link` and
   `*ast.Image` whose `ReferenceType` is not
   `LinkRegular` (i.e. full, collapsed, or shortcut
   reference).
3. For each, look up the normalized label in
   `parser.Context.Reference(label)`. If absent, emit
   a diagnostic at the bracket position.

Because goldmark drops *undefined* references back to
text, step 2 alone misses them. To catch the dropped
cases, add a source-level scan. A regex over the raw
bytes that finds `[...](...)`-free bracket pairs of
the forms `[X][Y]` and `[X][]`, then re-checks the
label against the reference store. The scan must
honor code spans, code fences, link destinations, and
HTML blocks.

### Auto-fix

No auto-fix. The right repair is project-specific: add
the missing definition, remove the broken link, or
correct the label spelling.

### Error messages

```text
reference label "plan128" has no matching link
reference definition
```

## Tasks

1. Scaffold `internal/rules/noundefinedreferencelabels/`
   with `rule.go`, `rule_test.go`, and the `init()`
   `rule.Register` call.
2. Implement `Check()` that:
   a. Re-parses with `lint.NewParser` to get the
      `parser.Context` (so PI blocks are skipped) and
      collect link reference definitions.
   b. Walks `*ast.Link` and `*ast.Image` nodes for
      reference-typed usages and verifies each label.
   c. Performs a guarded source-level scan for the
      bracket forms goldmark drops, emitting a
      diagnostic per unresolved label.
3. Implement `rule.Configurable` for `shortcut` and
   `placeholders`.
4. Implement `rule.Defaultable` returning `true`.
5. Register as MDS053 in category `link`.
6. Add fixture tests in
   `internal/rules/MDS053-no-undefined-reference-labels/`
   covering: matching full reference (clean), matching
   collapsed reference (clean), matching shortcut
   (clean), undefined full reference (flagged),
   undefined collapsed reference (flagged), undefined
   shortcut with link-shaped label (flagged under
   `heuristic`), bracketed prose (clean under
   `heuristic`), case-fold/whitespace-normalized match
   (clean), placeholder label (clean), label inside
   code span (clean), label inside fenced code
   (clean), label inside PI block (clean).
7. Add rule README following the MDS027 template,
   including a "See also" link to plan 107
   (no-reference-style) and to MDS027.
8. Reproduce the original failure: re-introduce a
   `[plan128][planXYZ]` reference with no `[planXYZ]:`
   definition in `docs/background/markdown-linters.md`,
   confirm the rule flags it, then remove the test
   reference.

## Acceptance Criteria

- [ ] `[a][b]` with `[b]: url` defined emits no
      diagnostic.
- [ ] `[b][]` with `[b]: url` defined emits no
      diagnostic.
- [ ] `[b]` with `[b]: url` defined emits no
      diagnostic.
- [ ] `[a][b]` with no `[b]: ...` definition emits one
      diagnostic on the link position.
- [ ] `[b][]` with no `[b]: ...` definition emits one
      diagnostic.
- [ ] `[plan128]` with no definition emits a
      diagnostic under `shortcut: heuristic` because
      the label looks like a reference target.
- [ ] `[just brackets]` in prose emits no diagnostic
      under `shortcut: heuristic`.
- [ ] `[Foo Bar]` resolves to `[foo bar]: ...`
      (CommonMark-normalized).
- [ ] A label listed in `placeholders` is never
      flagged regardless of definition state.
- [ ] Reference-style links inside code spans, fenced
      code, indented code, and `<?...?>` PI blocks are
      not flagged.
- [ ] Re-running the rule on
      `docs/background/markdown-linters.md` (with a
      deliberately undefined `[plan999]` reference)
      flags exactly one diagnostic; removing the
      reference makes the file clean again.
- [ ] Rule is enabled by default in the standard
      convention.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes on the repo.

## See also

- [MDS027 cross-file-reference-integrity][mds027]
- [Plan 107: no-reference-style][plan107]
- [Plan 129: no-unused-link-definitions][plan129]
- [Placeholder grammar][placeholder-grammar]

[md052]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md052.md
[mds027]: ../internal/rules/MDS027-cross-file-reference-integrity/README.md
[placeholder-grammar]: ../docs/background/concepts/placeholder-grammar.md
[plan107]: 107_no-reference-style.md
[plan129]: 129_no-unused-link-definitions.md
