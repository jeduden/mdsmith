---
id: 2606251522
title: user-extensible named word-lists
status: "🔳"
summary: >-
  Add a named word-list resource under `.mdsmith/wordlists/`.
  Every list-consuming rule gains a `lists:` setting that
  unions named lists into its inline list. No lists ship
  compiled in: the no-llm-tells convention keeps its curated
  words inline, and a project declares and extends its own
  lists with no rebuild.
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

- No lists ship compiled in. The `no-llm-tells` convention
  keeps its curated `ai-speak`/`ai-openers` words inline in
  `internal/convention/nollmtells.go`, as the rules'
  `contains:`/`starts:` presets.
- A new `internal/wordlist` package parses, looks up, and
  resolves user lists (the `extends:` chain, with cycle and
  missing-parent detection). It ships no embedded data.
- User lists live at `.mdsmith/wordlists/<name>.yaml`, where
  the basename is the list name. A list may `extends:`
  another user list.
- Rules name lists through a new generic `lists:` setting.
  The resolved entries union into the rule's own inline
  list. `lists:` appends across config layers, so a
  convention's `lists:` and a project's `lists:` combine.
- A rule decides how it reads its list: `forbidden-text`
  bans each entry, while `required-mentions` requires each
  entry in every section. `required-text-patterns` (MDS057)
  stays out — its `patterns:` are regular expressions, not
  plain words.
- `mdsmith init --wordlists` scaffolds the curated
  `ai-speak`/`ai-openers` set into `.mdsmith/wordlists/` as
  editable files, rendered from the convention's built-in
  data via `wordlist.RenderFile`. It is the only way mdsmith
  writes a curated list to disk; the resolver still has no
  built-ins, and an existing file is left untouched.

### File format

A `.mdsmith/wordlists/<name>.yaml` file carries an optional
`extends:` parent. It also carries a required `entries:`
list of literal strings. Strict YAML decoding rejects any
other key. Anchors and aliases are refused, as the other
`.mdsmith/` loaders do.

YAML keeps these files out of the `**/*.md` lint walk. So a
project's own denylist file is never flagged by the rule it
feeds.

(This replaces the Markdown body sketched earlier. YAML
matches the other loaders and avoids self-linting.)

```yaml
extends: house-base
entries:
  - synergy
  - circle back
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
   body parser, `Lookup` (user lists only), and `Resolve`
   for the `extends:` chain with cycle and missing-parent
   detection. No embedded data.
2. Add the `WordlistConsumer` interface to
   `internal/rule/rule.go`.
3. Keep the `no-llm-tells` entry in
   `internal/convention/convention.go` pointed at its inline
   `contains:`/`starts:` presets, with the curated words in
   `internal/convention/nollmtells.go`.
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
   `lists:` on a rule that is not a consumer, and an
   `extends:` cycle.
7. Give each of the sixteen rules its one-line
   `WordlistTarget()` method and interface assertion.
   `required-mentions` targets `mentions`; its entries are
   required, not forbidden, but the list mechanism is the
   same.
8. Keep `internal/integration/nollmtells_drift_test.go`
   comparing the convention's inline `contains:`/`starts:`
   lists against the catalog in
   `.claude/skills/docs-author/slop-patterns.md`.
9. Add `docs/reference/wordlist-files.md`. Update
   `docs/reference/conventions.md`,
   `docs/reference/convention-files.md`, and
   `docs/background/concepts/placeholder-grammar.md`. Add
   fixture cases driven by a `lists:` setting.

## Acceptance Criteria

- [ ] A `.mdsmith/wordlists/team.yaml` that `extends:`
      another file resolves, and a doc using a team word
      fails `mdsmith check`.
- [ ] A doc using the convention's curated words fails under
      `convention: no-llm-tells`.
- [ ] An unknown list name, a `lists:` on a non-consumer
      rule, and an `extends:` cycle each fail at config load
      with a clear message.
- [ ] `internal/convention/nollmtells.go` carries the curated
      words and the drift test passes against the
      convention's inline lists.
- [ ] `mdsmith init --wordlists` writes editable
      `.mdsmith/wordlists/ai-speak.yaml` and `ai-openers.yaml`,
      and skips a file that already exists.
- [ ] `mdsmith check .` stays green (the repo pins
      `convention: no-llm-tells`).
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
