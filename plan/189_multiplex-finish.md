---
id: 189
title: Finish the multiplex migration for pure per-node rules
status: "🔳"
model: opus
depends-on: [187]
summary: >-
  Migrate every default rule whose Check is a pure per-node, stateless
  pass to rule.NodeChecker so the engine drives them from one shared
  ast.Walk. Reverses plan 187's task 3 deferral after a follow-up review
  judged the compound impact of multiple sub-10% improvements is the
  way to close the gap to faster Rust linters.
---
# Finish the multiplex migration for pure per-node rules

## Goal

Migrate every default rule whose Check is a pure per-node, stateless
pass to the rule.NodeChecker interface. The engine then runs one
shared ast.Walk over the AST instead of N per-rule walks. Three rules
were already migrated as a prototype in plan 187 task 3 (MDS002,
MDS010, MDS018); this finishes the sweep across the rest.

## Background

[Plan 187](187_neutral-corpus-engine-lever.md) prototyped the
multiplexed walk. It concluded the full migration was "not worth the
cross-rule churn". The per-document walk flat cost was about 6%, far
smaller than the 44% cumulative goldmark walkHelper showed.

A follow-up review judged differently. The compound impact of
multiple sub-10% improvements is how mdsmith closes the gap to
faster Rust linters. A 6% trim is worth taking when it stacks on
the next one. This plan reverses the deferral.

A rule is migratable when:

- Its Check does only `ast.Walk(f.AST, ...)` plus trivial setup
  (precomputed maps shared across nodes, e.g. `CollectCodeBlockLines`).
- Each visited node is inspected independently — no state is carried
  across nodes.
- It does not rely on `ast.WalkSkipChildren` or `ast.WalkStop` on the
  top-level walk for correctness (using them as optimisations on
  child walks inside helper functions is fine).

Stateful rules keep their per-rule walks. The full list lives under
"Not migrated" below.

## Tasks

1. [x] Write this plan.
2. [x] Migrate the heading batch: MDS013 blank-line-around-headings,
   MDS017 no-trailing-punctuation-in-heading.
3. [x] Migrate the list batch: MDS014 blank-line-around-lists, MDS016
   list-indent, MDS045 list-marker-style, MDS046 ordered-list-numbering,
   MDS061 list-marker-space.
4. [x] Migrate the code batch: MDS011 fenced-code-language, MDS015
   blank-line-around-fenced-code, MDS031 unclosed-code-block, MDS052
   no-space-in-code-spans.
5. [x] Migrate the prose / link batch: MDS012 no-bare-urls, MDS032
   no-empty-alt-text, MDS035 toc-directive, MDS041 no-inline-html, MDS042
   emphasis-style, MDS044 horizontal-rule-style, MDS049 no-space-in-link-text,
   MDS055 forbidden-paragraph-starts, MDS056 forbidden-text, MDS063
   descriptive-link-text.
6. [x] Extend `TestCheckRules_MultiplexedEqualsSequential` (or add a
   per-rule equivalence test) so it pins every migrated rule's
   diagnostic output as byte-identical to the pre-migration Check.

## Acceptance Criteria

- [x] All migratable rules implement `rule.NodeChecker`; their `Check`
      delegates to `rule.WalkNodes(r, f)`.
- [x] The list of "Not migrated — reason" is recorded at the bottom of
      this file.
- [x] `go test ./...` passes.
- [x] `mdsmith check .` passes (generated sections in sync).
- [x] `go tool golangci-lint run` reports no issues.
- [x] The existing byte-identity test in
      [internal/engine/multiplex_test.go](../internal/engine/multiplex_test.go)
      still pins multiplexed output to sequential.

## Not migrated — reason

- **MDS003 heading-increment**: tracks `prevLevel` across headings.
- **MDS005 no-duplicate-headings**: builds a `seen` map across headings.
- **MDS051 single-h1**: collects all H1s before emitting diagnostics.
- **MDS036 max-section-length**: collects all headings to compute
  per-section bounds.
- **MDS037 duplicated-content**: collects paragraphs to compare against
  one another.
- **MDS050 proper-names**: uses `WalkSkipChildren` for correctness on
  code spans / HTML blocks, then post-sorts and deduplicates matches.
- **MDS020 required-structure**: collects headings into an ordered list
  before validating against the schema.
- **MDS043 no-reference-style**: multi-pass — needs to know whether
  any reference-style link exists before reporting unused-definition
  diagnostics.
- **MDS029 conciseness-scoring**: opt-in; one-time scorer load
  failure is reported by `errReportedOnce` outside the walk; not
  worth complicating the engine path for an opt-in rule.
- **MDS024 paragraph-structure**, **paragraph-readability**: already
  share a memoized paragraph collector (plan 187 task 2) and never
  walked `f.AST` directly.
- **MDS034 markdown-flavor**: does not walk f.AST; uses its own
  `DetectFiltered` line-and-AST scan.
- **MDS027 cross-file-reference-integrity**, **MDS021 include**,
  **MDS019 catalog**, **MDS038 toc**, **MDS039 build**, **MDS048
  git-hook-sync**: cross-file or directive engines; their Check
  bodies do not walk the host AST node-by-node.
- **MDS001 line-length**, **MDS006 no-trailing-spaces**, **MDS007
  no-hard-tabs**, **MDS008 no-multiple-blanks**, **MDS009 single-
  trailing-newline**, **MDS022 max-file-length**, **MDS025 table-
  format**, **MDS026 table-readability**, **MDS028 token-budget**,
  **MDS033 directory-structure**, **MDS040 recipe-safety**, **MDS047
  ambiguous-emphasis**, **MDS053 no-unused-link-definitions**, **MDS054
  no-undefined-reference-labels**, **MDS057 required-text-patterns**,
  **MDS058 required-mentions**, **MDS059 blockquote-whitespace**,
  **MDS064 atx-heading-whitespace**: do not walk f.AST in Check at
  all (line-level scans, table extractors, file-level checks).

## ...

<?allow-empty-section?>
