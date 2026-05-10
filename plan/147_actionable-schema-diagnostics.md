---
id: 147
title: Actionable schema diagnostics for MDS020
status: "🔲"
model: opus
summary: >-
  Replace MDS020's coarse "front matter does not
  satisfy schema CUE constraints" message with
  per-violation diagnostics that name the field,
  show the actual value, name the constraint, and
  suggest a concrete fix. Apply to inline schemas
  (plan 146), file schemas, and structure / require
  failures.
---
# Actionable schema diagnostics for MDS020

## Goal

Every MDS020 diagnostic should let a reader fix
the file without opening the schema. The reader
sees the field that failed, the value they wrote,
the constraint they violated, and — when the
constraint admits a finite vocabulary or a
regular pattern — a concrete suggestion.

The same standard applies to structure failures
("missing section X") and `<?require?>` failures
("filename does not match").

## Background

Today MDS020 emits messages like:

```text
front matter does not satisfy schema CUE
constraints: status: 1 errors in empty
disjunction: status: conflicting values "draft"
and "open" (mismatched types string and string)
```

The issue surface:

- The field name appears once, deep in the CUE
  error chain.
- The actual value the user wrote is buried.
- The expected vocabulary (`"open" | "in-progress"
  | "done"`) is not extracted; the user must open
  the schema to see it.
- "1 errors in empty disjunction" is a CUE
  internal phrase, not user vocabulary.

Compounding: when several FM fields fail at once,
the messages collapse into one line and the editor
gutter shows a single squiggle.

mdbase's diagnostics pre-format these — field,
type-name (`enum<a|b|c>`), value, with a hint when
the value is "close" to a valid one. The mdbase
research [§S-1 trigger](../docs/research/mdbase-vs-mdsmith/learn-from-mdbase.md)
notes that mdsmith's diagnostic clarity is a
prerequisite for inline schemas to feel
ergonomic. This plan delivers that prerequisite.

## Non-Goals

- New rule. This plan rewires MDS020's existing
  emit path; no new rule code, no new rule ID.
- Auto-fix for schema violations. Suggesting a
  fix string in the message stays text; an
  applicable code action is out of scope (the
  user has to type it).
- CUE engine changes. mdsmith reads CUE errors as
  produced; the work is post-processing them.

## Design

### One diagnostic per failure

Today CUE returns a flat error list per file;
MDS020 collapses them into one diagnostic. Switch
to one diagnostic per CUE error (or per missing
section, per filename mismatch). LSP-side this
shows one squiggle per problem.

### Message template

A schema diagnostic carries five fields:

| Field        | Source                                                                  | Example                                     |
|--------------|-------------------------------------------------------------------------|---------------------------------------------|
| `field`      | CUE error path; structure section name; `filename` for require failures | `status`                                    |
| `actual`     | Value as the user wrote it (`%q` for strings, raw for other JSON types) | `"draft"`                                   |
| `expected`   | Extracted from the schema expression (see below)                        | `one of: "open", "in-progress", "done"`     |
| `hint`       | Optional; nearest match for short enums or regexes                      | `did you mean "open"?`                      |
| `schema_ref` | File:line of the constraint                                             | `plan/proto.md:4` or `kind task / schema:7` |

The diagnostic message format:

```text
status: got "draft", expected one of:
  "open", "in-progress", "done"
  (did you mean "open"?)
schema: plan/proto.md:4
```

Two lines so the schema reference is greppable
without parsing the message.

### Expected-value extraction

The schema engine walks the CUE expression for
the failed field and renders a user-facing
"expected" string per shape:

| CUE shape            | Rendered as                      |
|----------------------|----------------------------------|
| `"a" \| "b" \| "c"`  | `one of: "a", "b", "c"`          |
| `=~"^FOO-[0-9]{4}$"` | `string matching ^FOO-[0-9]{4}$` |
| `int & >=1 & <=5`    | `int between 1 and 5`            |
| `string & != ""`     | `non-empty string`               |
| `bool`               | `true or false`                  |
| anything else        | the raw CUE expression           |

The fallback is the raw expression; the rendered
forms cover the common cases.

### Hint extraction

Hints are best-effort and only fire on a small set
of shapes:

- **Disjunction of string literals:** if `actual`
  is within Levenshtein distance 2 of one
  literal, suggest it. ("did you mean
  `"open"`?")
- **Regex:** if a known proper-name or filename
  pattern, no hint (regex hints are noisy).
- **Numeric range:** if `actual` is just outside
  the range, hint with the nearest bound. ("try
  5")

When no hint applies, the message ends after
`expected`. A noisy hint is worse than no hint.

### Structure and require diagnostics

The same template applies beyond CUE failures:

- **Missing section.** `field: structure` — wait,
  use the section heading itself: `field: ##
  Goal`, `expected: section to be present`,
  `hint: insert "## Goal" after "# Title"`.
- **Unexpected section.** `field: ## Random` —
  `expected: not present`, `hint: remove or move
  to <name>`.
- **Filename violation.** `field: filename` —
  `actual: "01_my-task.md"` (string, not an int
  pattern), `expected: string matching
  TASK-[0-9]+.md`.

### Backward compatibility

Existing tests asserting on message text break.
This is intentional — message text is a UX
contract, and the new shape *is* the contract.
The plan updates assertions in lockstep with the
emitter.

External consumers (LSP clients, CI scripts) read
the diagnostic's structured fields, not the
message text. The message-text change is invisible
to them; the new `Source` and `Code` fields are
additive.

## Tasks

1. Add a `SchemaDiagnostic` type in
   `internal/rules/requiredstructure/` carrying
   `Field`, `Actual`, `Expected`, `Hint`,
   `SchemaRef`. Render via a single
   `Format()` method.
2. Walk CUE errors in
   `requiredstructure.Rule.Check` and emit one
   `SchemaDiagnostic` per error path. Stop
   collapsing into a single diagnostic.
3. Implement the expected-value extractor for the
   five common CUE shapes in the table; fall back
   to the raw expression otherwise.
4. Implement the hint extractor for
   string-literal disjunctions (Levenshtein ≤2)
   and numeric ranges.
5. Replace structure-violation messages with
   `SchemaDiagnostic` instances. Same for
   `<?require filename:?>`.
6. Ensure every `SchemaDiagnostic` carries
   `RuleID: "MDS020"` and `RuleName:
   "required-structure"`. The LSP conversion
   in
   [`internal/lint/diagnostic.go`](../internal/lint/diagnostic.go)
   already maps these onto the wire-level
   `Code` and `Source` fields; this task makes
   sure they are populated at emit time so the
   LSP and JSON outputs stay consistent.
7. Update fixtures in
   `internal/rules/MDS020-required-structure/bad/`
   so the front-matter assertions match the new
   message shape. Add a fixture per shape in the
   table above.
8. Update assertions in the requiredstructure
   tests
   ([rule_test.go](../internal/rules/requiredstructure/rule_test.go))
   and the integration runner.
9. Document the new diagnostic shape and the
   hint behavior in the
   [MDS020 README](../internal/rules/MDS020-required-structure/README.md).

## Acceptance Criteria

- [ ] A file violating one FM constraint produces
      exactly one diagnostic with `field`,
      `actual`, `expected` populated.
- [ ] A file violating three FM constraints
      produces three diagnostics, each anchored
      to its own field's position.
- [ ] A string-disjunction violation with a typo
      ("draf" against `"draft" | "open"`)
      produces a `did you mean "draft"?` hint.
- [ ] A numeric-range violation outside the
      bounds renders `int between N and M`, not
      raw CUE syntax.
- [ ] A regex violation renders `string matching
      <pattern>`, not raw CUE syntax.
- [ ] Missing sections, unexpected sections, and
      filename violations all use the same
      `field / actual / expected` shape.
- [ ] Every emitted diagnostic carries
      `RuleID: "MDS020"` and
      `RuleName: "required-structure"`, and the
      LSP conversion preserves these as `Code`
      and `Source` on the wire.
- [ ] Schema reference (`schema: <file>:<line>`
      or `schema: kind <name> / schema:<line>` for
      inline) appears on every diagnostic.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues.
