---
id: 2607171900
title: >-
  Slidev structure rule (MDS073) — validate
  layouts, slots, fields, and frontmatter keys
  per slide
status: "🔳"
model: opus
depends-on: []
summary: >-
  A per-slide structural rule for Slidev decks:
  validates layout names, required ::slot::
  separators, layout-required frontmatter fields,
  and unknown frontmatter keys — the silent
  failures Slidev renders without ever reporting.
---
# Slidev structure rule (MDS073)

## Goal

Move mdsmith's Slidev support past the `slidev`
convention. That convention only *disables* eight
noisy rules. This rule *adds* value. It catches the
Slidev silent failures — the ones that render wrong
or empty output with no error.

Slidev's parser is permissive — an unmatched slot
drops its content, a misspelled `layout:` renders
blank, an unknown frontmatter key passes through as
data. None of these error. A static per-slide
linter catches them.

## Scope

A new opt-in rule `MDS073 slide-structure`,
enabled by the `slidev` convention. It splits a
deck into slides on `---` separators. It parses
each slide's frontmatter. Then it checks:

1. **Unknown layout** — `layout:` not in the
   built-in set (or the user's `custom-layouts`),
   with a did-you-mean suggestion.
2. **Missing required slot** — `two-cols` needs a
   `::right::`; `two-cols-header` needs `::left::`
   and `::right::`. Absent → the column renders
   empty.
3. **Orphaned slot** — a `::name::` separator the
   effective layout does not expose → the content
   after it silently vanishes.
4. **Missing required field** — `image*` layouts
   need `image:`; `iframe*` layouts need `url:`.
5. **Unknown frontmatter key** — a key not in the
   Slidev per-slide key set, within edit distance
   of a real one (typo like `transiton:`).

The rule is `Configurable`: `custom-layouts` lets a
project declare its theme's layout names so they
are not flagged as unknown. This is the escape
hatch that keeps mdsmith out of `node_modules` —
it never resolves theme packages.

## Non-goals (explicit boundaries)

- No Vue/UnoCSS/Comark validation — mdsmith owns
  the Markdown/structural layer, not rendering.
- No `src:` import graph, asset-path, code-highlight
  range, or per-slide size checks *in this rule* —
  those are follow-up plans that reuse the same
  slide model.
- No engine change. The rule does its own per-slide
  parsing inside `Check`, so the `File`
  single-frontmatter model is untouched. (The
  segmented-document engine model that lets the
  *existing* rules run per-slide — and retires the
  suppression convention — is a separate, larger
  plan.)

## Tasks

1. [ ] `internal/rules/slidevstructure/` — rule
   package. `Check` does a zero-alloc pre-scan of
   `f.Lines`; returns `nil` when the file has no
   `---` fence and no `::slot::` line (keeps the
   ≤10 allocs/op budget on ordinary Markdown).
2. [ ] Slide parser: split `f.Lines` into slides,
   parse each per-slide frontmatter block, collect
   `::slot::` markers, with body-relative line
   numbers.
3. [ ] The five checks above, each a table-driven
   unit test (red then green).
4. [ ] Register the rule; add to
   `internal/rules/all/all.go` and the integration
   import list. `EnabledByDefault() == false`.
5. [ ] `Configurable`: `custom-layouts` list
   setting with `ApplySettings` validation and a
   `DefaultSettings`.
6. [ ] Wire `slide-structure: {Enabled: true}` into
   the `slidev` convention so the convention finally
   adds a check.
7. [ ] Fixtures under
   `internal/rules/MDS073-slide-structure/` —
   `bad/` with expected diagnostics, `good/` a
   clean deck that also passes all default rules.
8. [ ] Rule `README.md` with the meta-information
   front matter (MDS169) and peer-linter mapping
   (all empty — no peer covers this).
9. [ ] Document the rule in
   `docs/reference/conventions.md` (the `slidev`
   section) and the rule reference.

## Acceptance Criteria

- [ ] `go test ./...` green, including the
  per-rule fixture and alloc-budget gates.
- [ ] `MDS073` stays ≤ 10 allocs/op on the shared
  alloc-budget fixture.
- [ ] `go run ./cmd/mdsmith check .` — 0 failures.
- [ ] The `slidev` convention enables
  `slide-structure`; a deck with a missing
  `::right::`, an unknown layout, and a typo'd key
  produces three diagnostics with precise lines.
- [ ] `go vet ./...` clean.
