---
id: 73
title: Unify template and processing directives
status: "🔳"
summary: >-
  Research plan: blind trials on all 33 rules
  identified six areas of user confusion.
  Implementation split into plans 74, 75, 76.
---
# Unify template and processing directives

Research for
[#68](https://github.com/jeduden/mdsmith/issues/68)
and
[#70](https://github.com/jeduden/mdsmith/issues/70).

Implementation plans:

- [74](74_directive-guide.md) -- directive guide
- [75](75_single-brace-placeholders.md) --
  `{field}` syntax for required-structure
- [76](76_rename-misleading-params.md) --
  rename misleading parameter names
- [77](77_template-composition-and-cycles.md) --
  template composition, cycle detection,
  `template` -> `schema` rename

## Blind trial design

Ten agents played "new developer." Each got a
two-sentence intro (max 50 words) and syntax
snippets. They guessed semantics and rated
confidence 1-5. Round 1 covered directives and
config (15 snippets). Round 2 covered the
remaining 25 rules.

## Results: confidence scores

| Snippet | Topic                       | Avg | Range |
|---------|-----------------------------|-----|-------|
| 1       | `<?catalog?>` pair            | 4.8 | 4-5   |
| 2       | `<?include?>` pair            | 4.8 | 4-5   |
| 3       | `<?require?>` single marker   | 4.0 | 2-5   |
| 4       | `<?allow-empty-section?>`     | 4.8 | 4-5   |
| 5       | `{{.id}}: {{.title}}` heading | 3.8 | 3-4   |
| 6       | catalog `row` table template  | 4.4 | 4-5   |
| 7       | `line-length` config          | 4.0 | 4-5   |
| 8       | config overrides            | 4.8 | 4-5   |
| 9       | CUE front-matter schema     | 4.4 | 4-5   |
| 10      | `{{.field}}` heading vs row   | 4.0 | 3-5   |
| 11      | 4-space indented directive  | 2.6 | 1-4   |
| 12      | nested directives in row    | 2.0 | 1-3   |
| 13      | empty section at EOF        | 4.6 | 3-5   |
| 14      | `paragraph-readability`       | 4.0 | 3-5   |
| 15      | `token-budget` with ratio     | 3.8 | 2-5   |

Round 2 (MDS002-MDS033): all simple style rules
scored confidence 5.

## Six areas of confusion

1. `{{.field}}` means "insert" in catalog but
   "match" in required-structure. All five
   participants flagged this as the top source
   of confusion. -> [plan 75](75_single-brace-placeholders.md)
2. 4-space indent silently kills directives.
   No diagnostic. Confidence 2.6, 4/5 called
   it a footgun. -> [plan 74](74_directive-guide.md)
3. Nested directives undefined. Confidence 2.0.
   Nobody could predict what happens.
   -> [plan 74](74_directive-guide.md)
4. Misleading parameter names: `ratio` (warning
   threshold?), `max-words` (per paragraph?),
   `variance` (statistical?).
   -> [plan 76](76_rename-misleading-params.md)
5. Users cannot predict which rules auto-fix.
   -> [plan 74](74_directive-guide.md)
6. `directory-structure: true` without `allowed`
   is a silent no-op.
   -> [plan 76](76_rename-misleading-params.md)

## What works well

- Self-describing names: `allow-empty-section`
  (4.8), `overrides` (4.8).
- Marker pairs intuitive: catalog and include
  both scored 4.8.
- CUE schema readable: `|` union and `?`
  optional read naturally (4.4).
- PIs are hidden on GitHub renders (intended).
- `{field}` renders as visible literal text on
  GitHub, making template placeholders readable
  in unprocessed files.

## Goal

Give each directive a clear "if X then Y" rule.
Write one guide covering all rules with
examples. Fix misleading names. Switch
required-structure to `{field}` single braces.

## Verification trial (post-change)

Re-ran 10 snippets with the guide, `{field}`
syntax, renamed params, and fixability table.

| Snippet | Topic                  | Before | After |
|---------|------------------------|--------|-------|
| 1       | `{field}` heading        | 3.8    | 4.6   |
| 2       | `{{.f}}` vs `{f}`          | 4.0    | 4.8   |
| 3       | 4-space indent         | 2.6    | 5.0   |
| 4       | nesting                | 2.0    | 4.8   |
| 5       | words-per-token        | 3.8    | 4.6   |
| 6       | max-words-per-sentence | n/a    | 5.0   |
| 7       | max-column-width-ratio | n/a    | 4.8   |
| 8       | dir-structure no-op    | n/a    | 4.6   |
| 9       | line-length fixable?   | ~2.5   | 5.0   |
| 10      | code-lang fixable?     | ~2.5   | 5.0   |

Key wins:

- Indent footgun: 2.6 -> 5.0 (guide warning)
- Nesting: 2.0 -> 4.8 (guide statement)
- Fixability: ~2.5 -> 5.0 (fixability table)
- No "threshold" misreadings of renamed params
- `{field}` vs `{{.field}}` clearly distinct

Remaining gaps:

- `directory-structure: true` no-op still
  confused 2/5. Plan 76 config warning needed.
- `<?require?>` in a normal file is silently
  ignored (5/5 flagged as confusing in
  template-vs-normal trial).
- `<?allow-empty-section?>` in a template does
  not propagate to documents (5/5 noted the
  misleading co-occurrence with `## ...`).
- Templates enforce headings and front matter
  only, not directives (2/5 uncertain about
  `<?catalog?>` in a template).

All three template-vs-normal gaps addressed
by plan 74 (guide section on templates).

## Hugo-user trial (5 participants)

Tested all 33 rules with agents primed as Hugo
users per
[#73](https://github.com/jeduden/mdsmith/issues/73).

Hugo-specific traps (confidence drops):

- `{{.title}}` vs Hugo's `{{ .Title }}`:
  case-sensitive key lookup silently returns
  empty string. 5/5 flagged.
- "Template" means validation schema, not
  rendering. 5/5 confused by the word reuse.
- Generated content committed to git: inverts
  Hugo's "never commit build output" model.
  5/5 called this disorienting.
- No nesting: Hugo shortcodes compose freely.
  5/5 expected nesting to work.
- `<?...?>` YAML quoting rules are alien vs
  Hugo's `{{< key="val" >}}`. 4/5 flagged.
- No template functions (`humanize`, etc.).
  3/5 reached for them.

What worked: simple style rules (5.0), config
overrides (5.0), self-describing names (5.0).

Confirmed prior findings: `max-words` misread
as per-paragraph (5/5), `variance` misread as
statistical (4/5).

All Hugo-specific gaps addressed by plan 74
(guide must include a "coming from Hugo"
section).

## Tasks

1. Write plan 74 (directive guide)
2. Write plan 75 (single-brace placeholders)
3. Write plan 76 (rename misleading params)

## Acceptance Criteria

- [ ] Plans 74, 75, 76 exist and pass lint
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
