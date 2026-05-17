---
id: 134
title: LSP completion for anchors, refs, kinds, and directive args
status: "✅"
model: sonnet
summary: >-
  Add `textDocument/completion` to `mdsmith lsp` so
  editors and agents can complete heading anchors in
  same-file and cross-file Markdown links, link
  reference labels, kind names in front matter, and
  `.md` paths in directive args. Reuses the workspace
  symbol index plan 131 already builds.
---
# LSP completion for anchors, refs, kinds, and directive args

## Goal

Let an LSP client complete the four token classes
mdsmith already indexes: heading anchors, link-ref
labels, kind names, and directive file paths.
Completion runs without grep. Agents stop
fabricating anchors and ref labels that do not
exist.

## Background

Plan 131 shipped the workspace symbol index. The
index backs definition, references, implementation,
and call hierarchy. Completion is the inverse
lookup. Given a partial token at the cursor, the
handler returns matching index entries. The data
is already there; this plan adds a handler.

Claude Code's [code intelligence][cc-ci] docs do
not list completion explicitly. Completion is a
standard LSP surface, and any LSP-aware agent or
editor will use it when present. The value is
high for Markdown authoring. Anchor and ref-label
fabrication is a common agent failure mode.
Completion eliminates it.

[cc-ci]: https://code.claude.com/docs/en/discover-plugins#code-intelligence

## Non-Goals

- Snippet templates beyond plain identifier
  insertion. No `[$1](#$2)` body completions.
- Completion of free-form heading text. This plan
  only completes targets that already exist in the
  index.
- Front-matter field-name completion. That requires
  schema integration (CUE / JSON Schema) and is
  better paired with a future schema-driven plan.
- Completion of directive names (`<?include`,
  `<?catalog`, `<?build`). Few directives exist and
  contributors learn them quickly; the win is
  small. Revisit if user feedback requests it.

## Design

### Capability

Advertise:

```jsonc
"completionProvider": {
  "triggerCharacters": ["#", "[", ":", "/", "\""],
  "resolveProvider": false
}
```

`resolveProvider` is false. All completion data
(label, detail, sortText) is computed in one pass
from the index. No second-stage lookup is needed.

### Trigger contexts

A new helper in
[`internal/index/locate.go`](../internal/index/locate.go)
returns a completion-context tag and the prefix
under the cursor. One handler per tag:

| Cursor on…                      | Items returned                  | `kind`       |
|---------------------------------|---------------------------------|--------------|
| `[text](#…` (no path)           | Headings in current file        | `Reference`  |
| `[text](./other.md#…`           | Headings in `other.md`          | `Reference`  |
| `[text][…`                      | Link-ref labels in current file | `Reference`  |
| Front-matter `kind:` value      | Kind names from `.mdsmith.yml`  | `EnumMember` |
| Front-matter `kinds:` list item | Kind names from `.mdsmith.yml`  | `EnumMember` |
| `<?include file: "…"?>` arg     | Workspace `.md` paths           | `File`       |
| `<?build source: "…"?>` arg     | Workspace `.md` paths           | `File`       |
| `<?catalog glob:…` entry        | Workspace `.md` paths           | `File`       |

`detail` carries the source location (file path
for headings, `.mdsmith.yml` for kinds). `sortText`
prioritises same-file matches above cross-file
matches for anchors.

### Anchor completion

For `[text](#…)` the handler walks the open
buffer's heading list (already cached by the
symbol index). For `[text](./other.md#…)` the
handler resolves `./other.md` against the document
URI and pulls headings from the index slice for
that file. Slugs come from
[`mdtext.CollectTOCItems`](../internal/mdtext/mdtext.go)
to match the same anchor format link resolution
already uses.

### Ref-label completion

For `[text][…]` the handler returns every
`LinkReferenceDefinition` label captured by the
index for the current file. Definitions in other
files are not returned (link refs are file-local
in CommonMark).

### Kind completion

The LSP server's existing `effectiveKindsFor` helper
in [`internal/lsp/symbols.go`](../internal/lsp/symbols.go)
already accepts both front-matter forms: the scalar
`kind: <name>` and the list `kinds: [a, b]`. The
scalar form is treated as a single-element kinds
list. The list form is parsed by
[`lint.ParseFrontMatterKinds`](../internal/lint/frontmatter.go);
the scalar form is parsed by `frontMatterScalarKind`
in the same `symbols.go` file.

Completion mirrors that. When the cursor sits in a
scalar `kind:` value, or inside a `kinds:` list
item (typically after `- ` on a new list line), the
handler returns the kind names declared in
[.mdsmith.yml](../.mdsmith.yml). Supporting both
keeps completion consistent with the existing
`definition`, `references`, and `implementation`
handlers, which already resolve via
`effectiveKindsFor`. The config is already loaded
by the server (plan 121) and re-read on
`.mdsmith.yml` change.

Note: scalar `kind:` is an LSP-layer convenience.
The lint pipeline parses only `kinds:` via
[`lint.ParseFrontMatterKinds`](../internal/lint/frontmatter.go),
and the user-facing
[file-kinds guide](../docs/guides/file-kinds.md)
documents only the list form.

So `mdsmith check` and `mdsmith fix` will not
honor a scalar `kind:` even though the LSP
treats both as equivalent. A future plan can
align the lint pipeline and the docs with the
LSP behavior. Until then, completion offers
`kind:` because `effectiveKindsFor` already
does, but contributors should prefer `kinds:`
in fixtures.

### Directive-arg completion

For `<?include file: "…"?>` and `<?build source:
"…"?>`, the handler walks the workspace `.md`
files (the same set the index covers) and returns
paths whose prefix matches. Paths are returned
relative to the open buffer's directory.

For `<?catalog glob: ["…"]?>` the same source set
applies. Glob characters in the prefix are
escaped; the handler does not try to expand globs
at completion time.

### Position and performance

Completion shares the symbol index plan 131 builds
(`internal/index`). Cold completion is bounded
by index lookup time; the bench in
[`internal/index/bench_test.go`](../internal/index/bench_test.go)
already establishes a cold-build budget of 1 s and
a per-`didChange` update budget under 20 ms on a
1 000-file synthetic corpus. Completion adds a
substring match over the index slice, which is
O(N) in the file's heading count and O(M) in the
workspace file count for cross-file paths. A
1 000-file workspace returns under 50 ms.

### Backwards compatibility

`completionProvider` is additive. Clients that
ignore the capability see the same post-plan-131
server.

## Tasks

1. Extend
   [`internal/index/locate.go`](../internal/index/locate.go)
   with a `completionContext(uri, pos)` helper
   returning `(tag, prefix, replaceRange)`. Cover
   each tag in the table with a unit test.
2. Add `textDocument/completion` to the server in
   [`internal/lsp/server.go`](../internal/lsp/server.go),
   dispatching per tag. Return an empty list (not
   `null`) when no items match, so clients do not
   trigger an error path.
3. Implement anchor completion (current file +
   cross-file) using the index. Cover both same-
   file and `./other.md#` cases with integration
   tests.
4. Implement ref-label completion using the
   current file's link-ref defs. Test that
   definitions from other files are excluded.
5. Implement kind completion against the loaded
   config for both front-matter forms: scalar
   `kind:` and `kinds:` list items. Mirror the
   existing parsing in `effectiveKindsFor`
   (`internal/lsp/symbols.go`) so completion
   stays consistent with `definition` /
   `implementation`. Test that `.mdsmith.yml`
   changes flush the cache (config-watcher in
   plan 121 already triggers a re-read; verify
   completion picks up the new kinds).
6. Implement directive-arg completion across
   `<?include?>`, `<?build?>`, and `<?catalog?>`.
   Return paths relative to the open buffer.
7. Advertise `completionProvider` with the trigger
   characters above. Add an end-to-end test in
   [`cmd/mdsmith`](../cmd/mdsmith) that drives
   `initialize` → `didOpen` → `completion` for at
   least one of each tag.
8. Add a "Completion" section to
   [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
   with the trigger-character list and the table
   of supported contexts.

## Acceptance Criteria

- [x] `completionProvider` appears in the
      `initialize` capabilities response with
      `triggerCharacters: ["#", "[", ":", "/", "\""]`.
- [x] Completion triggered after `[x](#` returns
      every heading in the current file by anchor
      slug.
- [x] Completion triggered after `[x](./other.md#`
      returns every heading in `other.md`.
- [x] Completion triggered after `[x][` returns
      every link-reference label defined in the
      current file and excludes labels from other
      files.
- [x] Completion inside a `kinds:` list item in
      front matter returns every kind name in
      `.mdsmith.yml`.
- [x] Completion after scalar `kind:` in front
      matter returns every kind name in
      `.mdsmith.yml`, matching the
      `effectiveKindsFor` server behavior.
- [x] Completion triggered inside an
      `<?include file: "…"?>` arg returns
      workspace `.md` paths whose prefix matches.
- [x] Completion outside any of the above contexts
      returns an empty list (not `null`, no
      error).
- [x] Completion latency stays under 50 ms on a
      synthetic 1 000-file workspace; bench reused
      from plan 131.
- [x] [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
      lists `completionProvider` in the capability
      table and documents the supported contexts.
- [x] All tests pass: `go test ./...`.
- [x] `go tool golangci-lint run` reports no
      issues.
- [x] `mdsmith check .` passes.

## Open Questions

- **Heading slug ambiguity.** When two headings in
  one file collapse to the same slug,
  [`mdtext.CollectTOCItems`](../internal/mdtext/mdtext.go)
  appends `-1`, `-2`, …. Completion should return
  every disambiguated form. Verify in the
  integration test.
- **Completion in code blocks.** The trigger
  characters fire inside fenced code blocks too.
  The handler should return an empty list when the
  position is inside a code span or fenced block;
  reuse the same AST walk diagnostics use.
- **Completion in front matter values that are not
  `kind:` or `kinds:`.** The handler should only
  fire on those two keys to avoid noise. Match by
  the YAML key preceding the cursor; treat scalar
  `kind:` and list `kinds:` as the two valid
  contexts and skip everything else.

## ...

<?allow-empty-section?>
