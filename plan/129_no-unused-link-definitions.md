---
id: 129
title: Flag unused or duplicate link reference definitions
status: "✅"
summary: >-
  New rule MDS053 that flags `[label]: url` link
  reference definitions that are never used by any
  reference-style link or image, and definitions that
  duplicate an existing label. Closes the parity gap
  with markdownlint MD053 and catches stale
  definitions left behind during edits.
model: sonnet
---
# Flag unused or duplicate link reference definitions

## Goal

Treat unused link reference definitions as lint
errors. A `[label]: url` line that no `[text][label]`
or `[label][]` or shortcut `[label]` consumes is dead
weight: it survives renames, accumulates over time,
and — most importantly — masks broken links because
`mdsmith check` never visits the URL. The same applies
to definitions whose URL points at a now-missing file:
without a usage to anchor MDS027 to, the broken target
is invisible.

This rule complements [MDS054][mds054-plan]
(no-undefined-reference-labels). MDS054 catches
references with no definition. MDS053 catches
definitions with no reference. Together they keep the
two halves of the reference-link table in sync.

## Background

### The hole this closes

[MDS027][mds027] (cross-file-reference-integrity) only
verifies destinations reachable from `*ast.Link`
nodes. goldmark exposes link reference definitions via
`parser.Context.Reference(label)`. It never emits an
AST node for an *unused* definition. MDS027 cannot
see it. A typo in the consuming reference (caught by
MDS054) leaves the definition orphaned. This rule
flags the orphan so the user notices.

The same pattern surfaces in
`docs/background/markdown-linters.md`. Renaming
a plan file (e.g. `120_glob-unification.md`) can
leave a stale `[plan121]: ../../plan/121_old-name.md`
entry behind. Nothing references it. It points at a
file that no longer exists. MDS053 catches the
orphan. Follow-on MDS027 catches the broken target
once the orphan is either removed or re-referenced.

### Duplicate definitions

CommonMark says: when two definitions share the same
normalized label, the *first* wins and the second is
silently ignored. Editors rarely notice the second
definition is dead. markdownlint's [MD053][md053]
flags duplicates as a separate violation. MDS053
follows the same rule: emit on every definition past
the first for any normalized label.

### Configuration: ignored labels

Some projects keep "header" or "footer" reference
definitions used by templates or by external tools
(e.g. a `[//]: # (comment)` style hack). An
`ignored-labels` setting names labels that are never
flagged as unused. The labels are CommonMark-normalized
before comparison.

## Design

### Configuration

```yaml
rules:
  no-unused-link-definitions:
    ignored-labels: []
```

Category: `link`. Enabled by default. `ignored-labels`
is a `replace`-mode list (override the whole list per
override layer); document this next to the
`ApplySettings` handler.

### Detection

1. Re-parse with `lint.NewParser()` so PI blocks are
   skipped consistently with MDS054 and the TOC
   directive.
2. Collect every link reference definition by walking
   the document. goldmark stores definitions in the
   `parser.Context`; iterate via the existing
   `Reference` lookup *plus* an AST scan for
   `*ast.LinkReferenceDefinition` nodes (or the
   parser's `references` slice if that path proves
   simpler) to recover both the label and source
   position.
3. Walk the AST and record the normalized label of
   every `*ast.Link` and `*ast.Image` whose
   `ReferenceType` is reference-typed.
4. Report two kinds of finding. **Unused**: a
   definition whose label is in the definition set
   but not in the usage set, and is not in
   `ignored-labels`. Emit at the definition line.
   **Duplicate**: every definition past the first
   for a given normalized label. Emit at the
   duplicate's line.

The "definition set" must use CommonMark-normalized
keys to match goldmark's lookup behavior.

### Auto-fix

Auto-fix removes unused definition lines (and the
duplicate copies, keeping the first). The fix is
line-deletion-only and trims any blank line left
behind so the file's blank-line policy is preserved.
When a definition is fenced inside a comment-style
block (`<!-- ... -->`) the fix is skipped — emit only.

### Error messages

```text
unused link reference definition "plan121"
duplicate link reference definition "plan121";
first defined on line 42
```

## Tasks

1. [x] Scaffold `internal/rules/nounusedlinkdefinitions/`
   with `rule.go`, `rule_test.go`, and the `init()`
   `rule.Register` call.
2. [x] Implement `Check()` that walks the AST for usages
   and the parser context (or AST) for definitions,
   then diffs the two sets.
3. [x] Implement duplicate detection by tracking the first
   line each normalized label appears on and emitting
   on every subsequent occurrence.
4. [x] Implement `rule.Configurable` for
   `ignored-labels`.
5. [x] Implement `rule.ListMerger` returning
   `rule.MergeReplace` for `ignored-labels` and
   document the choice next to `ApplySettings`.
6. [x] Implement `Fix()` that removes the offending
   definition lines and trims orphan blank lines.
7. [x] Implement `rule.Defaultable` returning `true`.
8. [x] Register as MDS053 in category `link`.
9. [x] Add fixture tests in
   `internal/rules/MDS053-no-unused-link-definitions/`
   covering: defined and used (clean), defined and
   unused (flagged + auto-removed), duplicate
   definition (second flagged + removed, first
   preserved), case-fold/whitespace-normalized match
   (clean), ignored label (clean even if unused),
   unused definition with surrounding blank lines
   (auto-fix collapses correctly), definition inside a
   PI block (not collected, not flagged).
10. [x] Add rule README following the MDS027 template
    with "See also" to plan 107, plan 128, MDS027.
11. [x] Audit `docs/background/markdown-linters.md` and
    other docs for orphaned reference definitions
    once the rule is enabled; remove or reconnect
    them in the same PR.

## Acceptance Criteria

- [x] A definition referenced by at least one link or
      image emits no diagnostic.
- [x] A definition with no consumer emits one
      diagnostic at the definition line; auto-fix
      removes the line.
- [x] Two definitions for `[foo]` emit one diagnostic
      on the second; auto-fix removes the second and
      keeps the first.
- [x] CommonMark normalization is applied: `[Foo
      Bar]: url` is considered used by `[x][foo bar]`.
- [x] A label listed in `ignored-labels` is never
      flagged as unused.
- [x] Auto-fix preserves blank-line policy: removing
      a definition between two blank lines collapses
      to a single blank line.
- [x] Definitions inside `<?...?>` PI blocks are not
      collected and therefore not flagged.
- [x] Repository docs pass with the rule enabled
      (orphans removed in this plan's PR).
- [x] Rule is enabled by default in the standard
      convention.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
- [x] `mdsmith check .` passes on the repo.

## See also

- [MDS027 cross-file-reference-integrity][mds027]
- [Plan 107: no-reference-style][plan107]
- [Plan 128: no-undefined-reference-labels][plan128]
- [Config merge semantics][config-merge]

[md053]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md053.md
[mds027]: ../internal/rules/MDS027-cross-file-reference-integrity/README.md
[mds054-plan]: 128_no-undefined-reference-labels.md
[plan107]: 107_no-reference-style.md
[plan128]: 128_no-undefined-reference-labels.md
[config-merge]: ../docs/development/index.md
