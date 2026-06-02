---
id: 230
title: Navigable schema diagnostics in the editor
status: "✅"
model: opus
depends-on: [147, 133]
summary: >-
  Make MDS020 schema violations navigable and
  readable in editors. Add structured
  `RelatedLocations` to `lint.Diagnostic` so each
  diagnostic points at the `proto.md` or kind-file
  line that declares the violated constraint. Redesign
  the LSP hover so the file's issue is the primary
  content and the rule docs sit below a separator as
  a one-line summary plus a link. Map
  `RelatedLocations` to LSP `relatedInformation`, and
  derive a `codeDescription` doc link from the rule
  ID, and fix the same contract for the Obsidian
  tooltip.
---
# Navigable schema diagnostics in the editor

## Goal

A reader who hits a schema violation should see three
things, in this order of prominence. What is wrong in
their file. Where in the file it is. A one-click jump
to the `proto.md` or kind-schema line that declares
the rule. The rule's own docs stay available, but
secondary.

## Background

[Plan 147](147_actionable-schema-diagnostics.md) made
MDS020 emit one diagnostic per violation. Each names
the field, the value, the expected constraint, a
hint, and a `schema:` reference. That solved the
message *text*. Three gaps remain on the editor
surface.

- **The schema location is not navigable.** It lives
  only as plain text in the message. The LSP wire
  diagnostic
  ([diagnostics.go](../internal/lsp/diagnostics.go))
  carries `Code`, `Source`, and `Message` — no
  `relatedInformation`, no `codeDescription`. So the
  editor shows the rule ID as a structured element
  while the schema location stays unlinked text. For
  inline kind schemas it is worse: `schemaRef`
  ([validate.go](../internal/schema/validate.go))
  returns the bare label `inline kind schema`, with
  no file and no line.
- **Diagnostics anchor at line 1, not the issue.**
  Frontmatter-field errors anchor at the field line.
  But missing sections, filename violations, and
  schema-compile errors fall back to
  `nonBodyDiagLine(f)` — file line 1. In a plan or
  kind file, that is where the frontmatter and the
  leading `<?require?>` directive sit. So the squiggle
  lands "at the start of the directive", not where
  the section belongs.
- **Rule docs dominate the issue in hover.**
  `ruleHoverContent`
  ([hover.go](../internal/lsp/hover.go)) renders the
  message as one line, then appends the *entire* rule
  README. The issue reads as a footnote above a wall
  of documentation.

`lint.Diagnostic`
([diagnostic.go](../internal/lint/diagnostic.go)) has
no field for a secondary location at all. The VS Code
extension
([extension.ts](../editors/vscode/src/extension.ts))
is a thin client; it renders what the server sends,
so it inherits every gap. Obsidian is unbuilt;
[plan 217](217_obsidian-plugin.md) designs the same
"rule code plus message" tooltip, so it would inherit
the gaps too.

## Non-Goals

- Auto-fixing schema violations. Anchoring a missing
  section is in scope; inserting it is not.
- Changing CUE validation or the plan-147 extractors.
- Building the Obsidian plugin. The rendering stays in
  [plan 217](217_obsidian-plugin.md); this plan fixes
  the shared contract and 217's design text.
- New rule IDs. This rewires MDS020's emit path.

## Design

### Related locations on `lint.Diagnostic`

The cross-cutting enabler. Add one type and two fields
to [diagnostic.go](../internal/lint/diagnostic.go):

```go
// RelatedLocation is a secondary source location that
// explains a diagnostic — e.g. the schema constraint
// a value violated. File may differ from the
// diagnostic's own File (the schema lives elsewhere).
type RelatedLocation struct {
    File    string // may be a proto.md / kind file
    Line    int    // 1-based
    Column  int    // 1-based; 0 when only line is known
    Message string // e.g. "schema requires one of: …"
}

// On Diagnostic:
//   RelatedLocations []RelatedLocation
```

`RelatedLocations` is rule-agnostic. Any rule may
attach them. `lint.Diagnostic` stays the single shared
seam. The schema package is the first producer; the
LSP and CLI consume. The rule-doc link (LSP
`codeDescription`) is not a diagnostic field — it is
derived from the rule ID via `rules.DocURL` at the LSP
surface.

The schema reference stops being baked into the
message. `SchemaDiagnostic.Format()`
([diagnostic.go](../internal/schema/diagnostic.go))
drops its trailing `schema:` line. The MDS020 emit
path attaches a `RelatedLocation` built from the same
`SchemaRef` data instead. One source of truth, two
consumers:

- **CLI** prints related locations as trailer lines,
  so the schema reference still appears in `check`
  output — from the struct, not the message.
- **LSP** maps them to `relatedInformation`.

### Anchor at the issue, not line 1

| Failure                  | Today           | After                                           |
| ------------------------ | --------------- | ----------------------------------------------- |
| FM field value           | field line      | unchanged; + related location                   |
| Missing required section | line 1          | insertion point: end of prior sibling, or H + 1 |
| Missing content node     | section heading | end of the matched section body                 |
| Filename / path-pattern  | line 1          | line 1 + related location to `path-pattern:`    |
| Schema compile error     | line 1          | line 1 + related location to the schema file    |

Every schema diagnostic gets the best anchor in the
linted file, plus a related location to the schema
rule. A missing section anchors at the heading it
should follow, via `MissingSectionAnchor`, not at the
top of the file. The guard stays intact. When the
insertion point sits inside a generated section, or
there is no preceding heading, the anchor falls back
to the non-positive `nonBodyDiagLine` value. That way
[`filterGeneratedDiags`](../internal/engine/check.go)
can never drop a missing-section diagnostic.

### Issue-first hover

Restructure `ruleHoverContent` so the issue leads and
the rule identity follows a separator:

```markdown
**status** — got `"draft"`, expected one of
`"open"`, `"in-progress"`, `"done"`
*did you mean `"open"`?*

Schema: [plan/proto.md:4](…#L4)

---

`MDS020` · required-structure — declared sections
must be present and in order. [Open docs ↗](…)
```

Design rules:

1. The issue block leads. The rule code is not
   prefixed to it; it moves to the secondary block.
2. A `---` separates the issue from rule identity.
3. The rule block is condensed to the README's
   one-line `summary:`, not the full body, plus a
   doc link.
4. The schema location renders as a link in the
   tooltip footer (Obsidian) and as native
   `relatedInformation` in VS Code.

`cachedRuleInfo` already loads `RuleInfo`, which has a
one-line `Description` from the README front matter
(MDS020: "Document structure and front matter must
match its schema"). The hover uses `Description`, not
`info.Content`. The doc link comes from
`rules.DocURL(RuleID)` (rule-ID → website page), mapped
to `codeDescription.href`, which must be an `http(s)`
URL. One call to settle in code: whether to keep the
plan-147 remediation line.

### LSP wire and Obsidian

Add `relatedInformation` and `codeDescription` to the
LSP `Diagnostic`
([protocol.go](../internal/lsp/protocol.go)), per LSP
§3.18.6. Map them in `toLSP`
([diagnostics.go](../internal/lsp/diagnostics.go)).
A related location can point at a different file, so
`toLSP` resolves each `RelatedLocation.File` to a
`file://` URI against the workspace root. That root is
the one new input to its signature.

[Plan 217](217_obsidian-plugin.md) consumes the same
contract. Its tooltip adopts the issue-first layout
and renders the schema location as a link, since
CodeMirror has no native `relatedInformation`. The
WASM payload carries `related_locations`
(`pkg/mdsmith.Diagnostic`); the rule-doc link is
derived from the rule ID, not stored. This plan
updates 217's text; the rendering stays there.

### Inline-schema location

So inline kinds get a real file:line, the loader
records each schema key's source position. Kind files
([plan 208](208_kind-files.md)) and `proto.md` are
standalone Markdown; the `yaml.Node` parse already
has line numbers, so wire them into
`Schema.FrontmatterLines`. Inline-in-`.mdsmith.yml`
keys need the config loader to retain per-key lines.
If that proves costly, fall back to the kind
declaration's line — still navigable.

## Tasks

1. Add `RelatedLocation` and `RelatedLocations` to
   [diagnostic.go](../internal/lint/diagnostic.go),
   with a test that zero values stay compatible.
2. Drop the `schema:` trailer from
   `SchemaDiagnostic.Format()`; add a method that
   returns the schema reference as a
   `RelatedLocation`. Update tests in lockstep.
3. Attach a schema `RelatedLocation` to every MDS020
   diagnostic in
   [validate.go](../internal/schema/validate.go) and
   [the rule](../internal/rules/requiredstructure/rule.go).
   Callers mutate the `Diagnostic` that `MakeDiag`
   returns, so its `(file, line, msg)` signature stays
   unchanged across its ~30 call sites.
4. Re-anchor a missing section at the preceding
   heading (`MissingSectionAnchor`), guarded so an
   insertion point inside a generated section falls
   back to the non-body anchor.
5. Add `rules.DocURL` (a reusable rule-ID → doc-page
   URL lookup) and derive `codeDescription` from it in
   `toLSP`. No `DocURL` field on the diagnostic.
6. CLI: print `RelatedLocations` as trailer lines in
   `check` / `fix`. Update surface tests.
7. LSP: add `relatedInformation` and
   `codeDescription` to the wire `Diagnostic`; map
   them in `toLSP`, resolving cross-file URIs.
8. Redesign `ruleHoverContent`
   ([hover.go](../internal/lsp/hover.go)) to the
   issue-first layout, using `RuleInfo.Description`
   for the one-line rule summary.
9. Thread the kind's `SourcePath` onto the inline
   schema-sources entry in the merge layer so an inline
   kind schema names its defining file, not the bare
   `inline kind schema` label. (Per-key `yaml.Node`
   line numbers within that file remain a follow-up.)
10. Update [plan 217](217_obsidian-plugin.md) to
    require the issue-first tooltip and the
    schema-location link.
11. Document the new shape in the
    [MDS020 README](../internal/rules/MDS020-required-structure/README.md)
    and
    [live-diagnostics](../docs/features/live-diagnostics.md).
12. VS Code: rewrite the hover's `rules.DocURL`
    website link to a `command:` link that opens the
    rule's embedded README offline in a `mdsmith-rule:`
    virtual document, backed by `mdsmith help rule`.
    The server keeps emitting the https link so other
    editors are unaffected.
13. Run `mdsmith fix .` and confirm `check .` passes.

## Acceptance Criteria

- [x] `lint.Diagnostic` carries `RelatedLocations`;
      old diagnostics serialize unchanged. The rule-doc
      link is derived from the rule ID at the LSP
      surface, not stored on the diagnostic.
- [x] An MDS020 FM violation has a `RelatedLocation`
      naming the schema file and line.
- [x] An inline-kind violation names the kind's
      defining file, not the bare label. (Per-key line
      numbers within that file stay a follow-up.)
- [x] A missing section anchors at the preceding
      heading, not file line 1, while keeping the
      `filterGeneratedDiags` guarantee (see Design).
- [x] A filename violation stays on line 1 but carries
      a related location to `path-pattern:`.
- [x] `mdsmith check` prints the schema reference from
      `RelatedLocations`, not the message body.
- [x] The LSP diagnostic includes
      `relatedInformation` (cross-file URI) and
      `codeDescription.href` for MDS020.
- [x] A VS Code hover shows the issue first, a
      separator, then a one-line summary and a doc
      link — not the full README.
- [x] The VS Code hover's doc link opens the rule's
      embedded README offline (a `mdsmith-rule:`
      virtual doc via `mdsmith help rule`), not the
      website — preserving the offline guarantee.
- [x] [Plan 217](217_obsidian-plugin.md) requires the
      issue-first tooltip and a navigable link.
- [x] The
      [MDS020 README](../internal/rules/MDS020-required-structure/README.md)
      documents the new shape.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues.
- [x] `mdsmith check .` passes.
