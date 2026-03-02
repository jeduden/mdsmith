---
id: 66
title: Switch Directives to HTML Processing Instructions
status: 🔲
---
# Switch Directives to HTML Processing Instructions

## Goal

Replace HTML comment syntax (`<!-- name ... -->`) with HTML processing
instruction syntax (`<?name ... ?>`) for all mdsmith directives, eliminating
accidental collisions with regular HTML comments and consolidating
marker parsing through the existing gensection package.

## Context

HTML processing instructions (`<?...?>`) are an alternative to HTML comments
(`<!-- ... -->`). Both are parsed by goldmark as `ast.HTMLBlock` nodes.

The current comment-based markers accidentally match any HTML comment that
starts with the directive name. For example, `<!-- include / preprocessor
tool links -->` triggers MDS021 because its prefix matches `<!-- include`.
Switching to `<?...?>` avoids this class of false positives entirely:
regular HTML comments are no longer ambiguous with directives.

The CommonMark spec (§4.6) defines processing instructions as type-3 HTML
blocks: start condition `<?`, end condition first line containing `?>`.
Multi-line processing instructions — with a YAML body between `<?name` and
a `?>` line — are fully conformant CommonMark.

## Directive Syntax After Change

Multi-line (with YAML params):

```text
<?include
file: path/to/file.md
?>
...included content...
<?/include?>
```

Single-line (no params):

```text
<?allow-empty-section?>
```

## Tasks

### 1. Update `gensection` engine

File: `internal/archetype/gensection/engine.go`

- Add `terminator string` field to `Engine` struct.
- In `NewEngine`, derive:
  - `startPrefix = "<?" + d.Name()`
  - `endMarker   = "<?" + "/" + d.Name() + "?>"`
  - `terminator  = "?>"`

### 2. Update `gensection` parser

File: `internal/archetype/gensection/parse.go`

- Add `terminator string` parameter to `FindMarkerPairs` and thread it
  through to `processMarkerLine`.
- In `processMarkerLine` (YAML body mode): replace hardcoded
  `trimmed == "-->"` with `trimmed == terminator`.
- In `processLineOutsidePair` (single-line start detection): replace
  `strings.HasSuffix(rest, "-->")` with `strings.HasSuffix(rest, terminator)`.
- Update `TrimSuffix` call accordingly.
- `CollectIgnoredLines` / `addHTMLBlockLines`: no logic change needed — the
  existing check (`strings.HasPrefix(firstLineText, startPrefix)`) works for
  `<?` prefixes as well as `<!--` prefixes.

### 3. Add single-line directive helper to `gensection`

File: `internal/archetype/gensection/parse.go` (or a new
`directive.go` in the same package)

Add an exported helper:

```go
// IsSingleLineDirective reports whether line is a single-line
// processing instruction with the given name: <?name?>.
func IsSingleLineDirective(line, name string) bool
```

This is the single canonical place that encodes the `<?name?>` format
for parameterless directives.

### 4. Update `emptysectionbody` to use the central helper

File: `internal/rules/emptysectionbody/rule.go`

- Remove `buildAllowMarkerPattern` and its regex.
- Replace `hasAllowMarker` (which iterates `ast.HTMLBlock` nodes and
  applies the regex) with a call to `gensection.IsSingleLineDirective`
  on the raw line text of each `ast.HTMLBlock` node.
- The config key and marker name (`allow-empty-section`) stay the same;
  only the detection mechanism changes.

### 5. Migrate real marker files

Update all non-example, non-testdata `.md` files that contain live markers:

- `README.md` — two `<!-- catalog ... --> ... <!-- /catalog -->` blocks
- `PLAN.md` — one `<!-- catalog ... --> ... <!-- /catalog -->` block
- `background/markdown-linters.md` — one include marker block
- `plan/proto.md` — `<!-- allow-empty-section -->` template lines

Only two plan files currently use `<!-- allow-empty-section -->`;
update those as well.

### 6. Migrate example/documentation files

Update marker syntax shown in:

- `internal/rules/MDS019-catalog/README.md`
- `internal/rules/MDS021-include/README.md`
- `archetypes/generated-section/README.md`
- `internal/rules/MDS030-empty-section-body/README.md`

### 7. Update tests

- `internal/archetype/gensection/engine_test.go` — update all
  marker literals in test cases.
- `internal/rules/include/rule_test.go` and fixtures.
- `internal/rules/catalog/rule_test.go` and fixtures.
- `internal/rules/emptysectionbody/rule_test.go` — update pattern
  literals and test fixtures.
- Any `testdata/` fixtures that embed marker syntax.

## Acceptance Criteria

- [ ] All `.md` files containing live markers use `<?...?>` syntax.
- [ ] `mdsmith check .` exits 0 with no diagnostics.
- [ ] `<!-- include / preprocessor tool links -->` in
  `background/markdown-linters.md` is no longer flagged by MDS021.
- [ ] Regular HTML comments (`<!-- ... -->`) containing directive-like words
  are not flagged by any rule.
- [ ] `go test ./...` passes.
- [ ] `golangci-lint run` reports no issues.
