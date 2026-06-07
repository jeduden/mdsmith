---
id: 213
title: Built-in `no-llm-tells` convention with append-mode forbidden lists
status: "✅"
model: opus
depends-on: []
summary: >-
  Ship a built-in convention that enables MDS055 and
  MDS056 with curated lists of LLM-writing tells
  (banned words, phrases, paragraph starts), plus
  tighter MDS023 / MDS024 thresholds for non-native
  readers. Switch MDS055 `starts:` and MDS056
  `contains:` to append-mode merge so projects can
  extend the bundle without losing it.
---
# Built-in `no-llm-tells` convention with append-mode forbidden lists

## Goal

One config knob — `convention: no-llm-tells`.

It enables a curated baseline of mechanical
anti-slop checks. Three categories: banned
vocabulary, banned phrases, banned sentence
openers. It also tightens MDS023 and MDS024
thresholds for non-native readers.

## Background

`docs-author` (`.claude/skills/docs-author/`)
ships a catalog of LLM-writing tells in
`slop-patterns.md`. The skill applies the catalog
at write or revise time.

Part of that catalog is mechanical. Banned words,
banned phrases, banned paragraph openers — the kind
of tells **MDS055 forbidden-paragraph-starts** and
**MDS056 forbidden-text** can already check. A
rule catches them in CI and in LSP without a model
in the loop.

The cleanest way to ship the mechanical layer is a
new built-in convention. The
[conventions reference](../docs/reference/conventions.md)
already documents the pattern: an opinionated rule
bundle behind one config key.

One blocker: MDS055's `starts:` and MDS056's
`contains:` are list settings, which replace by
default per
[CLAUDE.md's config merge semantics](../CLAUDE.md).
A project that sets `convention: no-llm-tells` and
then adds its own forbidden words would erase the
built-in list. The convention is unusable until
those keys merge by append.

## Non-Goals

- Detecting structural slop (rule-of-three
  garnish, hedging seesaw, meta-narration,
  uniform sentence length). Those stay in the
  `docs-author` skill — they need contextual
  judgement no `contains:` list can express.
- Detecting tone slop (promotional voice,
  obsequious phrasing, padded qualifiers).
  Same — needs context.
- Auto-generating the convention's lists from
  `slop-patterns.md`. v1 hardcodes the lists in
  Go; a follow-up plan can wire a generator if
  drift becomes a problem.
- Pinning a Markdown flavor. The convention
  declares `Flavor: FlavorNone` (or the
  least-restrictive option — see Open Questions)
  so projects using GFM or Obsidian can still
  opt in.

## Design

### Append-mode merge for forbidden lists

Two rule packages opt their list settings into
append-mode merge:

- `internal/rules/MDS055-forbidden-paragraph-starts`
  returns `MergeAppend` for `starts:`.
- `internal/rules/MDS056-forbidden-text` returns
  `MergeAppend` for `contains:`.

Both implement `rule.ListMerger.SettingMergeMode(key)`.
The placeholder
vocabulary (MDS023 `placeholders:`) is the
canonical reference implementation.

After this change:

- A user's `starts:` / `contains:` extends the
  convention's list rather than replacing it.
- Duplicate entries collapse to one (set-style
  union).
- Order is convention-first, then user (for
  deterministic diagnostics when more than one
  pattern matches at the same anchor).

### Convention definition

A new entry in
`internal/convention/convention.go`:

```go
"no-llm-tells": {
    Name: "no-llm-tells",
    // Flavor: see Open Questions.
    Rules: map[string]convention.RulePreset{
        "forbidden-text": {
            Enabled: true,
            Settings: map[string]any{
                "contains": llmVocabularyAndPhrases(),
            },
        },
        "forbidden-paragraph-starts": {
            Enabled: true,
            Settings: map[string]any{
                "starts": llmParagraphOpeners(),
            },
        },
        "paragraph-structure": {
            Enabled: true,
            Settings: map[string]any{
                "max-words-per-sentence": 25,
            },
        },
        "paragraph-readability": {
            Enabled: true,
            Settings: map[string]any{"max-index": 12.0},
        },
        "descriptive-link-text": {Enabled: true},
    },
},
```

The two list helpers live next to the convention
entry. v1 hardcodes their contents. The source is
`.claude/skills/docs-author/slop-patterns.md` —
its "Vocabulary tells", "Phrasal tells", and
"Sentence openers" sections.

### Source-of-truth note

For v1, `slop-patterns.md` and the convention
helpers carry parallel lists. A short comment
above each helper points readers to
`slop-patterns.md`; a short note in
`slop-patterns.md` points back. A drift-checker
test (see Tasks) asserts the rule lists are a
subset of the skill catalog so the two cannot
silently diverge.

## Open Questions

- **Flavor pinning.** Built-in conventions all pin
  a flavor today. Anti-slop is renderer-agnostic;
  pinning would force any user of this convention
  off GFM. Options: (a) leave the convention's
  `Flavor` unset and teach the loader to allow
  that; (b) pin `commonmark` like `plain` and
  document the trade-off; (c) ship one variant per
  flavor (`no-llm-tells`, `no-llm-tells-gfm`).
  Default: (a). User to confirm.
- **Convention name.** Working title is
  `no-llm-tells`. Alternatives: `plain-prose`,
  `clear-prose`, `low-slop`, `house-voice`. User
  to confirm before the plan moves to 🔳.
- **Default severity.** MDS055 / MDS056 emit
  Error today. Should the convention preset them
  to Warning, given the heuristic nature of slop
  detection? Default: leave at Error to match the
  other rules; projects can downgrade per rule.

## Tasks

1. [x] **MDS055 / MDS056 list-merger.** Implement
   `SettingMergeMode("starts") == MergeAppend` on
   MDS055 and `SettingMergeMode("contains") ==
   MergeAppend` on MDS056. Add a failing unit
   test asserting that a layered config (kind +
   override) unions the lists rather than
   replacing. Make it pass.
2. [x] **Convention entry.** Add `"no-llm-tells"` to
   `conventions` in
   `internal/convention/convention.go`. Add a
   convention-loader unit test that loads it,
   confirms the right rules are enabled, and
   confirms the lists deep-merge with a user's
   own forbidden entries.
3. [x] **Forbidden lists.** Add the curated word /
   phrase / opener lists as package-level
   functions next to the convention entry. Source
   from `slop-patterns.md`.
4. [x] **Drift-checker test.** Integration test that
   reads `.claude/skills/docs-author/slop-patterns.md`,
   extracts the items under "Vocabulary tells",
   "Phrasal tells", and "Sentence openers", and
   asserts the convention's lists are a subset of
   each. Fails CI when one source drifts from the
   other.
5. [x] **Docs.** Add a `no-llm-tells` section to
   [`docs/reference/conventions.md`](../docs/reference/conventions.md)
   matching the format of `portable` / `github` /
   `obsidian` / `plain`. Add a short pointer from
   `slop-patterns.md` back to the convention.
6. [x] **Skill update.** Add a one-paragraph note to
   `.claude/skills/docs-author/SKILL.md`
   explaining that the mechanical layer ships as
   the `no-llm-tells` convention, and that
   `mdsmith check` enforces it without the skill
   in the loop. The skill still owns the
   structural / tone / formatting passes.

## Acceptance Criteria

- [x] `convention: no-llm-tells` in a project's
      `.mdsmith.yml` enables MDS055, MDS056,
      MDS024, MDS023, and MDS063 with the
      documented settings.
- [x] A project that also sets `rules.forbidden-text.contains:`
      with extra strings unions its list with the
      convention's list; neither side is dropped.
- [x] Same for `rules.forbidden-paragraph-starts.starts:`.
- [x] The drift-checker test fails when an item
      is removed from either source.
- [x] `mdsmith check .` against a fixture with
      "delve" or "it's important to note that"
      produces an MDS056 diagnostic. (MDS056 is
      case-sensitive; the catalog form is
      lowercase.)
- [x] `mdsmith check .` against a fixture
      starting a paragraph with "Certainly," or
      "Moreover," produces an MDS055 diagnostic.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no
      issues.
- [x] `mdsmith check .` passes.
