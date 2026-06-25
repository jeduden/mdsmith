---
id: 2606251522
title: user-extensible named word-lists
status: "🔲"
summary: >-
  Replace the hardcoded word lists in nollmtells.go with a
  named word-list resource under `.mdsmith/wordlists/`. Every
  list-consuming rule gains a `lists:` setting that unions
  named lists into its inline list; the no-llm-tells
  convention names the built-in `ai-speak` and `ai-openers`
  lists, and a project adds or extends lists with no rebuild.
model: opus
depends-on: []
---
# user-extensible named word-lists

## Goal

Turn the curated anti-slop vocabulary into a named word-list
resource. Any rule can reference it. Any project can extend
it, with no rebuild of the mdsmith binary.

## Context

`internal/convention/nollmtells.go` stores the LLM tell
words, phrases, and sentence openers as Go slices. The
`no-llm-tells` convention is the only way to apply them. A
rebuild is the only way to change them. A project can append
raw words at `forbidden-text.contains`, but it cannot name,
reuse, or extend the curated set.

Sixteen built-in rules already read a user-supplied list of
strings:

- `forbidden-text.contains`
- `forbidden-paragraph-starts.starts`
- `proper-names.names`
- `no-inline-html.allow`
- `descriptive-link-text.banned`
- `callout-type.allow`
- `no-unused-link-definitions.ignored-labels`
- `required-mentions.mentions`
- the `placeholders` vocabulary, shared by eight rules

Each keeps its list alone. None can point at a shared, named
list.

Scope for this plan: wire all fifteen rules, name the
resource `wordlists`, and fold the work into the open PR on
this branch.

## Design

A word-list is a named, ordered set of literal strings with
an optional `extends:` parent. It is the fourth
user-extensible `.mdsmith/` resource, after kinds, schemas,
and conventions. It reuses their loader pattern.

- Built-in lists ship embedded: `ai-speak` (tell words plus
  phrases) and `ai-openers` (sentence openers). The data
  moves out of Go into a new `internal/wordlist` package as
  embedded files.
- User lists live at `.mdsmith/wordlists/<name>.md`, where
  the basename is the list name. A list may `extends:` a
  built-in or another user list. Built-in names are reserved
  from redefinition but stay open to `extends:`, as
  convention names are.
- Rules name lists through a new generic `lists:` setting.
  The resolved entries union into the rule's own inline
  list. `lists:` appends across config layers, so a
  convention's `lists:` and a project's `lists:` combine.
- The `no-llm-tells` convention drops its embedded Go
  literals for `forbidden-text: {lists: [ai-speak]}` and
  `forbidden-paragraph-starts: {lists: [ai-openers]}`.
- A rule decides how it reads its list: `forbidden-text`
  bans each entry, while `required-mentions` requires each
  entry in every section. `required-text-patterns` (MDS057)
  stays out — its `patterns:` are regular expressions, not
  plain words.

### File format

A `.mdsmith/wordlists/<name>.md` file carries optional YAML
front matter for `extends:` only. The body is one entry per
non-blank line, taken as written after trimming. A line that
starts with `#` after trimming is a comment. One entry per
line — not a YAML or bullet list — so phrases and trailing
commas survive with no quoting or marker stripping. The file
reads as a plain catalog that mdsmith can lint. The built-in
lists use this same format as embedded files, so one parser
serves both.

```markdown
---
extends: ai-speak
---
synergy
circle back
```

### Resolution

The `lists:` key is resolved in the config layer, never per
check, so rule allocation budgets stay flat:

1. The merge layer treats `lists` as append for every rule,
   in one place.
2. A final pass over the merged rules expands each `lists:`
   against the registry. It unions the entries into the
   rule's target list, then drops the `lists:` key so the
   rule never sees it.

The target list per rule comes from a small interface,
`rule.WordlistConsumer`, whose one method returns the
setting key (`contains`, `starts`, `placeholders`, and so
on). Config reads it through `rule.ByName`, the same path
the merge layer already uses for `ListMerger` and
`SettingsTranslator`. A rule's match logic does not change.

## Tasks

1. Add package `internal/wordlist`: the `Wordlist` type, a
   body parser, `Lookup` (user first, then built-in), and
   `Resolve` for the `extends:` chain with cycle and
   missing-parent detection. Embed `data/ai-speak.md` and
   `data/ai-openers.md`, ported from the slices in
   `internal/convention/nollmtells.go`.
2. Add the `WordlistConsumer` interface to
   `internal/rule/rule.go`.
3. Rework the `no-llm-tells` entry in
   `internal/convention/convention.go` to name the two
   built-in lists. Delete `internal/convention/nollmtells.go`.
4. Add the user-file loader
   `internal/config/wordlist_files.go`, modeled on
   `internal/config/convention_files.go`. Add
   `Config.Wordlists` and its deep-copy in
   `internal/config/merge.go`.
5. Wire resolution. Make `lists` append in
   `internal/config/deepmerge.go`. Add the expand-and-strip
   pass at the end of `effectiveRules` in
   `internal/config/merge.go`.
6. Add `validateWordlists` and call it from
   `internal/config/load.go`. Reject an unknown list name, a
   `lists:` on a rule that is not a consumer, a redefined
   built-in name, and an `extends:` cycle.
7. Give each of the sixteen rules its one-line
   `WordlistTarget()` method and interface assertion.
   `required-mentions` targets `mentions`; its entries are
   required, not forbidden, but the list mechanism is the
   same.
8. Repoint `internal/integration/nollmtells_drift_test.go` to
   compare the embedded built-in data against the catalog in
   `.claude/skills/docs-author/slop-patterns.md`.
9. Add `docs/reference/wordlist-files.md`. Update
   `docs/reference/conventions.md`,
   `docs/reference/convention-files.md`, and
   `docs/background/concepts/placeholder-grammar.md`. Add
   fixture cases driven by a `lists:` setting.

## Acceptance Criteria

- [ ] A `.mdsmith/wordlists/team.md` with `extends: ai-speak`
      resolves, and a doc using a team word fails
      `mdsmith check`.
- [ ] A doc using only built-in words still fails under
      `convention: no-llm-tells`.
- [ ] An unknown list name, a `lists:` on a non-consumer
      rule, a redefined built-in name, and an `extends:` cycle
      each fail at config load with a clear message.
- [ ] `internal/convention/nollmtells.go` is gone and the
      drift test passes against the embedded data.
- [ ] `mdsmith check .` stays green (the repo pins
      `convention: no-llm-tells`).
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
