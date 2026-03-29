---
id: 73
title: Unify template and processing directives
status: "🔲"
summary: >-
  Evaluate unifying catalog templates,
  required-structure patterns, and processing
  directives under a single user model with a
  simple mental model and a comprehensive guide.
---
# Unify template and processing directives

Addresses [#68](https://github.com/jeduden/mdsmith/issues/68)
(unify template syntax) and
[#70](https://github.com/jeduden/mdsmith/issues/70)
(central directive documentation).

## Problem

Three mini-languages overlap: Go templates,
pattern placeholders, and processing
instructions. No single reference covers them.
`{{.field}}` means "insert" in catalog but
"match" in required-structure.

## Goal

Give each directive a clear "if X then Y" rule.
Write one guide covering all rules with examples.
Fix misleading parameter names found by trials.

## Blind trial results (5 participants)

Five developers got a two-sentence intro and
15 syntax snippets. They guessed what each
snippet does and rated their confidence.

### Confidence scores (1=guessing, 5=certain)

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

### Key misconceptions found

1. **`{{.field}}` dual meaning (snippet 10):**
   All five participants correctly identified
   that template headings and catalog rows use
   the same syntax for different purposes. All
   flagged this as the top source of confusion.
   Three called it "genuinely confusing" despite
   getting the right answer. The syntax collision
   means even correct users feel uncertain.

2. **Indented directives silently break
   (snippet 11):** Average confidence 2.6.
   Most guessed it would be treated as a code
   block, but nobody was sure. Four of five
   called this a "footgun" because there is no
   diagnostic -- the directive just disappears.

3. **Nested directives undefined (snippet 12):**
   Confidence 2.0, the lowest of all snippets.
   Guesses ranged from "literal text" to "nested
   not allowed" to "recursive include." Nobody
   was sure.

4. **`<?require?>` role unclear (snippet 3):**
   One participant rated confidence 2. The word
   "require" is ambiguous: require a file to
   exist? A filename pattern? Front-matter
   fields? The snippet alone does not tell you.

5. **`ratio: 0.75` meaning (snippet 15):**
   Two participants misread `ratio` as a warning
   threshold (warn at 75% of budget). It is
   actually a words-to-tokens multiplier. The
   parameter name does not signal its unit.

6. **fix behavior unclear for simple rules
   (snippet 7):** Three participants were unsure
   whether `line-length` is fixable. One rated
   fix confidence at 2. Users cannot predict
   which rules are fixable without reading each
   rule's docs.

### What worked well

- Self-describing names scored highest:
  `allow-empty-section` (4.8), `overrides` (4.8).
- Marker pairs are intuitive: catalog and
  include both scored 4.8.
- CUE schema readable: `|` union and `?`
  optional read naturally (4.4).

### Round 2: all remaining 25 rules

Simple rules (MDS002-MDS016) scored confidence
5 across the board. Names are self-describing.
New misconceptions found only in meta/complex
rules:

1. `directory-structure: true` is a silent
   no-op without an `allowed` list.
2. `max-words` in paragraph-structure is
   per-sentence, not per-paragraph. Name
   misleads.
3. `max-column-width-variance` is a ratio
   (max/min), not statistical variance.
4. `?` not flagged by no-trailing-punctuation
   -- intentional but undiscoverable.
5. Fixability asymmetry: `fenced-code-style`
   is fixable but `fenced-code-language` is
   not.

### Implications for the user model

Simple style rules need no rethinking -- names
do the work. Confusion concentrates in:

- Same syntax, two meanings (`{{.field}}`).
- Silent failures (indented directives).
- Undefined composition (nesting).
- Misleading parameter names (`ratio`,
  `max-words`, `variance`).
- Unpredictable fixability.
- Silent no-ops (`directory-structure: true`).

The guide must address all six.

## Proposed user model

### Principle: two kinds of markers

Users already grasp this (blind trial 4.8 avg
for pairs). Make it the documented rule:

- Closing tag present: `fix` regenerates body.
- No closing tag: `check` validates a condition.

### Principle: one template language

Unify `{{.field}}` to always mean the same
thing: "insert the value of `field`."

- **Catalog** already uses Go `text/template` --
  keep as-is.
- **Required-structure** currently reuses the
  `{{.field}}` syntax for pattern matching, not
  insertion. This is the source of confusion.

**Proposal**: change required-structure to use a
different sigil for wildcard matching. Two
options:

| Option          | Syntax                | Meaning                                                    |
|-----------------|-----------------------|------------------------------------------------------------|
| A (recommended) | `{field}` single braces | "heading must contain the value of front-matter key `field`" |
| B               | `*`                     | "any text" (glob-style)                                    |

Option A keeps the link to front matter explicit
while removing the visual collision with Go
templates. Option B is simpler but loses the
field-name hint.

**Decision required from maintainer:** pick A
or B before implementation.

### What does NOT change

The `<?...?>` syntax, YAML parameters,
`gensection` engine, CLI commands, and
`.mdsmith.yml` all stay. This plan improves
the user model and docs, not the engine.

## Tasks

### Phase 1: central directive guide (#70)

1. Write `docs/guides/directives.md` covering:

  - Quick-reference table (name, purpose, has
    closing tag, fixable, parameters)
  - Placement rules: max 3-space indent, not
    inside fenced code or HTML blocks. Call out
    the 4-space footgun with an explicit warning
    and example (blind trial snippet 11)
  - Each directive: purpose, parameters, one
    good example, one bad example, what `check`
    reports, what `fix` does
  - A "which rules auto-fix?" summary (blind
    trial snippet 7 confusion)
  - Nesting: explicitly state that directives
    inside generated content are not processed
    (blind trial snippet 12 confusion)

2. Add cross-links from each rule README to the
   guide.
3. Update `CLAUDE.md` catalog to include the new
   page.

### Phase 2: resolve `{{.field}}` ambiguity

4. Decide on Option A or B for
   required-structure patterns (needs maintainer
   input).
5. Implement the chosen syntax in
   `requiredstructure/rule.go`:

  - Update pattern extraction to use new sigil
  - Keep `{{.field}}` as a deprecated alias for
     one release cycle

6. Update `internal/rules/MDS020-required-structure/README.md`.
7. Update the directive guide with the new
   syntax.
8. Migrate existing template files
   (`plan/proto.md`,
   `internal/rules/proto.md`,
   `.claude/skills/proto.md`) to the new syntax.

### Phase 2b: fix misleading names and no-ops

9. Rename `token-budget` `ratio` to
   `words-per-token`. Keep `ratio` as alias.
10. Rename `paragraph-structure` `max-words` to
   `max-words-per-sentence`. Keep alias.
11. Rename `table-readability`
   `max-column-width-variance` to
   `max-column-width-ratio`. Keep alias.
12. Make `directory-structure: true` without
   `allowed` emit a warning instead of silently
   doing nothing.

### Phase 3: evaluate template engine (stretch)

13. Prototype replacing Go `text/template` with
   gonja or pongo2 in catalog rendering.

  - Measure: does Jinja syntax reduce user
     confusion vs Go templates?
  - Measure: does it enable filters/conditionals
     that users actually need?

14. If the prototype shows clear benefit, plan a
   migration. If not, keep Go `text/template` and
   document its quirks in the guide.

## Acceptance Criteria

- [ ] `docs/guides/directives.md` exists and
      covers every directive with examples
- [ ] The guide passes `mdsmith check
      docs/guides/`
- [ ] The guide documents the 4-space indent
      footgun and nesting behavior
- [ ] The guide includes a fixability summary
- [ ] `{{.field}}` in required-structure
      templates uses a distinct syntax from
      catalog templates (Phase 2)
- [ ] Existing template files are migrated
      (Phase 2)
- [ ] `ratio` renamed to `words-per-token` with
      backward-compatible alias (Phase 2b)
- [ ] `max-words` renamed to
      `max-words-per-sentence` (Phase 2b)
- [ ] `max-column-width-variance` renamed to
      `max-column-width-ratio` (Phase 2b)
- [ ] `directory-structure: true` without
      `allowed` emits a config warning
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
