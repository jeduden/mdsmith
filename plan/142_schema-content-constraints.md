---
id: 142
title: Content rules for prose constraints
status: "✅"
model: sonnet
depends-on: [146]
summary: >-
  Ship four new content rules
  (`forbidden-paragraph-starts`, `forbidden-text`,
  `required-text-patterns`, `required-mentions`)
  and extend MDS036 with word and paragraph
  caps. All default-disabled, configurable
  document-wide today and per-section once plan
  146's per-scope override lands. No new schema
  language — the schema just reuses the standard
  `rules:` block.
---
# Content rules for prose constraints

## Goal

Cover the per-section prose constraints in
S-7's runbook sketch (max words, forbidden
starts, forbidden contains, required patterns,
required mentions) as ordinary mdsmith rules.
Each rule is configurable like every other
rule. The schema language gets no new shape;
it just reuses plan 146's per-scope `rules:`
block.

## Background

Earlier drafts of this plan defined `max-words:`,
`forbidden-starts:`, etc. as a second
configuration surface inside the schema. That
turns out to duplicate the rule pipeline — a
new diagnostic ID format, a new place for LSP
hover text, a new path for `mdsmith help` to
discover. The cleaner answer is normal rules:
they register in the rule set, they expose
settings via `Configurable`, they appear in
`mdsmith kinds resolve`, and they apply
per-scope as soon as plan 146 ships its
override mechanism.

The
[mdbase research](../docs/research/mdbase-vs-mdsmith/learn-from-mdbase.md)
describes these constraints as part of S-7;
this plan ships the rule-set half, leaving the
schema-side wiring to plan 146.

## Non-Goals

- Auto-fix for any of the new rules. Detecting
  "first paragraph starts with 'We'" is fine;
  rewriting it is the user's job.
- A schema-side `max-words:` / `forbidden-starts:`
  shape. The schema is purely a scope tree
  (plan 146); rule config goes through the
  standard `rules:` surface.

## Design

### Five additions

Four new rules plus settings on MDS036:

| Rule                                  | Setting                                        | Effect                                                            |
| ------------------------------------- | ---------------------------------------------- | ----------------------------------------------------------------- |
| **MDS036** max-section-length         | `max-words:`, `min-words:`, `max-paragraphs:`  | Word and paragraph caps in addition to today's line cap.          |
| **MDS055** forbidden-paragraph-starts | `starts: [str, ...]`                           | Flag paragraphs that begin with any listed string.                |
| **MDS056** forbidden-text             | `contains: [str, ...]`                         | Flag the scope when its text contains any listed string.          |
| **MDS057** required-text-patterns     | `patterns: [{pattern, message, skip-indices}]` | Flag the scope when its text does not match the regex.            |
| **MDS058** required-mentions          | `mentions: [str, ...]`                         | Flag the scope when its text does not mention each listed string. |

All four new rules are default-disabled. A
project that wants document-wide enforcement
sets the rule globally in `.mdsmith.yml`; a
project that wants section-scoped enforcement
relies on plan 146's per-scope override. Both
surfaces feed the same rule code.

### Document-wide and per-section

Document-wide:

```yaml
rules:
  required-mentions:
    mentions: ["scope: production"]
  forbidden-text:
    contains: ["should", "may", "might"]
```

Per-section (via plan 146):

```yaml
schema:
  sections:
    - heading: "Diagnosis"
      rules:
        forbidden-text:
          contains: ["should", "may"]
        required-mentions:
          mentions: ["forward reference"]
```

Same rule, two scopes. The merge rules from
plan 97 already cover this composition — list
settings replace by default, with `append`
opt-in per setting.

### Diagnostics

Each rule emits through the existing
`lint.Diagnostic` shape. The diagnostic message
names the offending value (the start string,
the unmatched pattern, the missing mention)
and points at the source line. Rules that
operate on a scope (MDS057, MDS058) anchor at
the scope's heading line.

`skip-indices:` on MDS057 patterns exempts
specific child indices ("the last step is
exempt"; negative indices count from the end).
This setting is meaningful only when the rule
runs from a scope override on a section with
`children:` (plan 146); document-wide use
ignores it.

### Tokenization

All four new rules reuse the tokenizer from
[`internal/lint`](../internal/lint) for word
counts and paragraph segmentation. MDS036's
new word and paragraph counters share the same
helper. No new tokenization code.

## Tasks

1. ✅ Extend MDS036 max-section-length
   ([README](../internal/rules/MDS036-max-section-length/README.md))
   with `max-words:`, `min-words:`, and
   `max-paragraphs:` settings. The existing
   `max:` (lines) keeps its current
   behavior.
2. ✅ Add `MDS055 forbidden-paragraph-starts`.
   One paragraph walk per scope. Default
   disabled.
3. ✅ Add `MDS056 forbidden-text`. One text walk
   per scope. Default disabled.
4. ✅ Add `MDS057 required-text-patterns`.
   Compile regexes at config-load time.
   `skip-indices:` parses today but is a
   no-op; it activates once the
   `children:` schema feature ships.
5. ✅ Add `MDS058 required-mentions`. Substring
   match on the scope's text.
6. ✅ Verify each new rule composes with plan
   146's per-scope override mechanism.
   `TestCheck_InlineSchema_PerScopeForbiddenText`
   and `TestCheck_InlineSchema_PerScopeRequiredMentions`
   in `internal/rules/requiredstructure/`
   cover document-vs-section behavior.
7. ✅ Documented the rule pack in
   `docs/guides/schemas.md` under "Content
   constraints". Each rule also has its
   standard
   `internal/rules/<id>-<name>/README.md`.
8. ✅ Added good and bad fixtures per rule
   under `internal/rules/MDS055-…/`,
   `internal/rules/MDS056-…/`,
   `internal/rules/MDS057-…/`,
   `internal/rules/MDS058-…/`. MDS036
   gained `good/max-words.md`,
   `good/min-words.md`,
   `good/max-paragraphs.md`, and matching
   `bad/` fixtures.

## Acceptance Criteria

- [x] MDS036 with `max-words: 50` flags a
      section above 50 words and passes one
      below.
- [x] MDS036 with `min-words: 10` flags a
      section below 10 words.
- [x] MDS036 with `max-paragraphs: 3` flags
      a section with four paragraphs.
- [x] MDS055 flags a paragraph starting with
      a configured string and passes other
      paragraphs.
- [x] MDS056 flags a scope whose body
      contains a configured string.
- [x] MDS057 flags a scope whose body does
      not match the configured regex.
      `skip-indices:` parses today but is a
      no-op; enforcement waits on the
      `children:` schema feature.
- [x] MDS058 flags a scope that does not
      mention every configured string.
- [x] Each new rule applies document-wide
      when configured under top-level
      `rules:`.
- [x] Each new rule applies per-section when
      configured under a schema scope's
      `rules:` block (plan 146).
- [x] All new rules are default-disabled.
- [x] Each new rule has a README under
      `internal/rules/<id>-<name>/` and a
      good/bad fixture set.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no
      issues.
