---
id: 2607051919
title: >-
  Add dedicated unit tests for helpers added by the
  word-list and reflow features
status: "✅"
model: sonnet
summary: >-
  Several unexported helpers added for word-lists,
  no-llm-tells, line-length reflow, and the
  concisenessscoring classifier are only exercised
  transitively, with no TestFoo by name. Flagged by
  the 2026-07-05 audit.
---
# Add dedicated tests for new helpers

## Goal

Give every unexported helper flagged by the 2026-07-05
audit its own `TestFoo` (or `TestReceiver_Foo`) test.
This follows [the test pyramid's "every function has a
dedicated unit test"
rule](../docs/development/architecture/tests.md).

## Background

The 2026-07-05 audit found these new helpers.
Each one is covered only indirectly, through an
end-to-end test of its caller:

- `internal/wordlist/wordlist.go`: `allNames`, `flatten`
- `internal/config/wordlist_files.go`: `toWordlistMap`,
  `stripLists`, `resolveListEntries`, `expandRuleLists`
  (these three have zero test-file references at all)
- `internal/convention/nollmtells.go`: `llmVocabulary`,
  `llmPhrases`, `llmVocabularyAndPhrases`,
  `llmParagraphOpeners`
- `internal/rules/linelength/fix.go` and `reflow.go`:
  `overlapsGeneratedRange`, `trimTrailingCR`,
  `cloneBytes`, `paragraphHasRawHTML`
- `internal/rules/concisenessscoring/classifier/model.go`:
  `buildPhraseMarkers`

`stringsToAny`/`toAnySlice` also appeared in the audit's
test-debt list. They are handled instead by
[plan/2607051918](2607051918_arch-fix-wordlist-config-dedup.md),
which replaces them outright, so no separate test is
needed here.

## Tasks

1. `internal/wordlist/wordlist_test.go`: add
   `TestAllNames` and `TestFlatten` covering the empty,
   single-entry, and `extends`-chain cases.
2. `internal/config/wordlist_files_test.go` (create if
   missing): add `TestToWordlistMap`, `TestStripLists`,
   `TestResolveListEntries`, `TestExpandRuleLists`.
3. `internal/convention/nollmtells_test.go`: add
   `TestLlmVocabulary`, `TestLlmPhrases`,
   `TestLlmVocabularyAndPhrases`,
   `TestLlmParagraphOpeners` — each asserting non-empty
   output and a couple of known entries, mirroring the
   existing `TestNoLLMTellsWordlists` style.
4. `internal/rules/linelength/fix_test.go` /
   `reflow_test.go`: add `TestOverlapsGeneratedRange`,
   `TestTrimTrailingCR`, `TestCloneBytes`,
   `TestParagraphHasRawHTML`.
5. `internal/rules/concisenessscoring/classifier/model_test.go`:
   add `TestBuildPhraseMarkers`.
6. `go test ./...` passes.
7. Confirm the allocation-budget test
   (`internal/integration/alloc_budget_test.go`) still
   passes — new tests must not touch rule `Check` hot
   paths.

## Acceptance Criteria

- [x] Every helper named above has a dedicated test by
      name in a sibling `_test.go` file.
- [x] `go test ./...` is green.
- [x] `mdsmith check .` is green.
