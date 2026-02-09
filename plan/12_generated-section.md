# TM019: generated-section

## Goal

Add a lint rule that verifies auto-generated sections in markdown files
stay in sync with their directive output, and can regenerate them with
`--fix`.

## Tasks

1. Add `FS fs.FS` field to `lint.File`
   - Add `FS fs.FS` field to `File` struct in `internal/lint/file.go`
   - Set `f.FS = os.DirFS(filepath.Dir(path))` in
     `internal/engine/runner.go` after `NewFile`
   - Set `f.FS = os.DirFS(filepath.Dir(path))` in
     `internal/fix/fix.go` after each `NewFile` call
   - Leave `FS` as `nil` for stdin (rules check for nil)

2. Create `internal/rules/generatedsection/parse.go`
   - `findMarkerPairs()` -- scan source for start/end marker pairs,
     skip lines inside code blocks and HTML blocks (use goldmark AST,
     same pattern as `linelength`). The `-->` terminator is recognized
     when a trimmed line equals `-->`.
   - `parseDirective()` -- extract directive name (must be lowercase)
     from the first line. Produce diagnostic if directive name is
     missing. Parse the remaining lines as YAML using
     `gopkg.in/yaml.v3`, split result into parameters (`glob`, `sort`)
     and template sections (`header`, `row`, `footer`, `empty`).
     Validate all YAML values are strings (non-string values produce
     diagnostic per key). Validate `row` is non-empty when present.
     Emit error when `header` or `footer` is present but `row` is
     missing (`empty` alone is valid; `empty` + `header`/`footer`
     without `row` still fires the missing-row diagnostic).
   - Types: `markerPair`, `directive`

3. Create `internal/rules/generatedsection/generate.go`
   - `type fileEntry struct { Fields map[string]string }` (includes
     `filename` key)
   - `renderTemplate()` -- render header + row-per-file + footer.
     Each rendered value (`header`, `row`, `footer`, `empty`) is
     terminated by `\n`; if the value already ends with `\n`, no
     additional newline is added. This applies uniformly.
   - `renderDefault()` -- render minimal bullet list (no template
     case). Link text is basename, link target is relative path.
     Each entry is followed by a trailing `\n`.

4. Create `internal/rules/generatedsection/rule.go`
   - `Rule` struct, `init()` registration, `ID()` -> `"TM019"`,
     `Name()` -> `"generated-section"`
   - `Check(f *lint.File)` -- skip if `f.FS == nil` (stdin). Find
     markers, parse YAML body, generate expected content, compare,
     emit diagnostics.
   - `Fix(f *lint.File)` -- skip if `f.FS == nil`. Find markers,
     parse YAML body, generate expected content, replace stale
     sections. Leave malformed markers and template-execution-error
     sections unchanged.
   - `resolveCatalog()` -- glob expansion via
     `doublestar.Glob(f.FS, pattern)` (supports `**` recursive
     patterns, operates on `fs.FS`). Reject absolute glob paths
     and patterns containing `..`. Read front matter from matched
     files via `fs.ReadFile(f.FS, path)`. Skip unreadable files
     and directories silently.
   - `parseSort()` -- parse sort value: optional `-` prefix for
     descending, then key name. Built-in keys: `path`, `filename`.
     All other keys look up front matter. Case-insensitive comparison
     with path as tiebreaker.
   - `readFrontMatter()` -- read file via `fs.ReadFile`, parse YAML
     via `lint.StripFrontMatter()` + `gopkg.in/yaml.v3`
   - Code block and HTML block line collection (reuse pattern from
     `linelength`)

5. Register the rule and add dependency
   - `go get github.com/bmatcuk/doublestar/v4` to add the dependency
   - Add `"generated-section"` to `allRuleNames` in
     `internal/config/load.go`
   - Add blank import `_ "github.com/.../generatedsection"` in
     `cmd/tidymark/main.go`
   - Add blank import in `internal/integration/rules_test.go`

6. Create `internal/rules/generatedsection/rule_test.go`
   - Use `fstest.MapFS` for in-memory test filesystems (no
     `t.TempDir()` needed for unit tests)
   - Unit tests for: marker parsing (including `-->` whitespace
     trimming, single-line start marker, directive name whitespace
     trimming, case-sensitive directive matching), YAML body
     parsing, template rendering (minimal/list/table/multi-line
     row with `|`, `|+`, `|-`), implicit trailing `\n` on all
     sections (header/row/footer/empty), empty fallback (with and
     without `row`), `empty` + `header` without `row`, `empty`
     with `{{...}}` renders literally, non-string YAML value
     diagnostic, empty `row`/`sort` value diagnostics, missing
     directive name, nested start markers, Check (fresh/stale/
     malformed markers/invalid YAML), Fix (idempotent, multiple
     pairs per file), sort behavior (`path`, `filename`, front
     matter key, descending with `-` prefix, `sort: "-"` invalid,
     case-insensitive, path tiebreaker), sort in minimal mode
     with front matter key, recursive `**` glob, absolute glob
     path rejected, glob with `..` rejected, brace expansion not
     supported, dotfiles not matched, code-block-ignored markers,
     HTML-block-ignored markers, stdin skip (`f.FS == nil`),
     unreadable files, invalid front matter in matched files,
     binary/non-Markdown matched files, glob matching linted file,
     template execution errors, unknown YAML keys ignored,
     duplicate YAML keys (last wins), validation short-circuits

7. Create `rules/TM019-generated-section/README.md`
   - Full rule specification (already drafted in the rules directory)

## Key Files

| File | Action |
|------|--------|
| `internal/lint/file.go` | Edit (add `FS fs.FS` field) |
| `internal/engine/runner.go` | Edit (set `f.FS`) |
| `internal/fix/fix.go` | Edit (set `f.FS`) |
| `internal/rules/generatedsection/parse.go` | Create |
| `internal/rules/generatedsection/generate.go` | Create |
| `internal/rules/generatedsection/rule.go` | Create |
| `internal/rules/generatedsection/rule_test.go` | Create |
| `internal/config/load.go` | Edit |
| `cmd/tidymark/main.go` | Edit |
| `internal/integration/rules_test.go` | Edit |
| `go.mod` / `go.sum` | Edit (add doublestar dependency) |
| `rules/TM019-generated-section/README.md` | Create |

## Implementation Notes

### Filesystem access via `fs.FS`

Add an `FS fs.FS` field to `lint.File`. The engine sets it to
`os.DirFS(filepath.Dir(path))` for disk files; it stays `nil` for
stdin. The rule checks `f.FS == nil` and skips when there is no
filesystem context.

This uses Go's standard `io/fs.FS` interface (Go 1.16+). The
`doublestar` library natively supports `fs.FS` via
`doublestar.Glob(fsys, pattern)`. File reading uses
`fs.ReadFile(fsys, path)`.

Benefits:
- No changes to the `Rule`/`FixableRule` interfaces
- No changes to existing rule implementations
- Tests use `fstest.MapFS` (stdlib) for fast, deterministic,
  in-memory test doubles
- Future rules that need filesystem access get it for free

Constraint: `io/fs.FS` does not support `..` path traversal. Glob
patterns must be relative-downward (e.g., `docs/*.md`, `**/*.md`).
This matches the spec requirement.

### Marker detection

Markers inside fenced code blocks or HTML blocks must be ignored. Use
the goldmark AST to collect code block and HTML block line ranges (same
pattern as `linelength`'s `collectCodeBlockLines`).

The `-->` terminator is recognized when a line, after trimming leading
and trailing whitespace, equals `-->`.

### YAML value validation

The marker's YAML body values must all be strings. Non-string values
(numbers, booleans, arrays, maps) produce a diagnostic per key. Use
`gopkg.in/yaml.v3` to unmarshal into `map[string]any` and type-assert
each value. Additionally, `row: ""` and `sort: ""` (empty strings)
produce diagnostics.

### Sort implementation

Parse the sort value: strip optional leading `-` for descending order,
then use the remaining string as the key name. Built-in keys `path`
and `filename` are resolved from the file entry; all other keys are
looked up in front matter (missing -> empty string). Comparison is
case-insensitive (via `strings.ToLower` before comparing). When values are equal, use
relative file path (ascending, case-insensitive) as tiebreaker.

In minimal mode, front matter is read only when the sort key is a
front matter field (not `path` or `filename`).

### Front matter handling

External files' front matter is extracted via `lint.StripFrontMatter()`
and parsed with `gopkg.in/yaml.v3` into `map[string]any`. Non-string
values are converted via `fmt.Sprintf("%v", val)`. (Note: this is
different from the marker YAML body where non-string values are
rejected -- front matter values are coerced because the rule does not
own those files.)

### Trailing newline handling

Each rendered value (`header`, `row`, `footer`, `empty`) is terminated
by `\n`. If the value already ends with `\n` (from a YAML block
scalar), no additional newline is added. This applies uniformly to all
sections. Minimal mode entries each end with `\n`.

### Content boundary

The generated content region spans from the line immediately after the
`-->` line of the start marker to the line immediately before the
`<!-- tidymark:gen:end -->` line. Comparison is byte-exact over this
range. Generated output uses `\n` line endings.

### Interpolated error messages

Diagnostic messages that include `...` (e.g., "invalid glob pattern: ...")
interpolate the underlying Go error string as-is.

## Acceptance Criteria

- [ ] `lint.File` has `FS fs.FS` field, set by engine/fixer, nil for stdin
- [ ] Minimal mode (glob only) produces plain link list with basenames
- [ ] List template renders per-file with front matter fields
- [ ] Table template renders static header + per-file rows
- [ ] Multi-line `row` value with YAML `|` produces multi-line output per file
- [ ] Multi-line `row` value with YAML `|+` preserves trailing blank lines
- [ ] YAML `|-` strips trailing newlines; implicit `\n` rule adds one back
- [ ] Each row is followed by implicit trailing `\n`
- [ ] `footer` renders static content after rows
- [ ] `empty` renders fallback text when glob matches zero files
- [ ] `empty` alone without `row` is valid (no diagnostic)
- [ ] `empty` + `header` without `row` produces missing-row diagnostic
- [ ] `empty` value gets trailing `\n`
- [ ] No `empty` + no matches produces empty content between markers
- [ ] Up-to-date section produces zero TM019 diagnostics
- [ ] Stale section produces one diagnostic per section
- [ ] Unclosed start marker produces diagnostic
- [ ] Orphaned end marker produces diagnostic
- [ ] Nested start markers produce diagnostic
- [ ] Missing directive name produces diagnostic
- [ ] Unknown directive name produces diagnostic
- [ ] Missing `glob` parameter produces diagnostic
- [ ] Empty `glob: ""` produces diagnostic
- [ ] Absolute glob path produces diagnostic
- [ ] Glob with `..` produces diagnostic
- [ ] Brace expansion in glob not supported
- [ ] Invalid glob pattern produces diagnostic
- [ ] Invalid YAML body produces diagnostic
- [ ] Non-string YAML values produce diagnostic per key
- [ ] Empty `row: ""` produces diagnostic
- [ ] Empty `sort: ""` produces diagnostic
- [ ] `header`/`footer` without `row` produce diagnostic
- [ ] Invalid template syntax produces diagnostic
- [ ] Template execution error produces diagnostic
- [ ] Invalid `sort` value (e.g., `"-"`) produces diagnostic
- [ ] Unknown YAML keys are silently ignored
- [ ] Duplicate YAML keys use last value
- [ ] YAML anchors, aliases, and merge keys are supported
- [ ] Files without front matter resolve fields to empty string
- [ ] `{{.filename}}` resolves to path relative to linted file's directory
- [ ] `{{.filename}}` never has leading `./` prefix
- [ ] `header`/`footer` containing `{{...}}` render literally
- [ ] `empty` containing `{{...}}` renders literally
- [ ] `header` and `footer` get implicit trailing `\n` (same rule as rows)
- [ ] When `empty` renders, `header`/`footer` are not included in output
- [ ] When glob matches files and `empty` is defined, `empty` is ignored
- [ ] Matched file with invalid front matter treated as no front matter
- [ ] Matched binary/non-Markdown file included (no front matter extracted)
- [ ] Multiple marker pairs in one file processed independently
- [ ] Symlinks in glob results are followed
- [ ] Glob matching the linted file includes it
- [ ] Windows `\r\n` files flagged as stale (generated content uses `\n`)
- [ ] Directive name is case-sensitive (`Catalog` -> unknown directive)
- [ ] Directive name whitespace is trimmed; extra words after name ignored
- [ ] End marker matched after trimming whitespace (`<!-- tidymark:gen:end -->`)
- [ ] Sort value with whitespace (e.g., `"foo bar"`) produces diagnostic
- [ ] All diagnostics report column 1
- [ ] Validation short-circuits on structural errors
- [ ] Fix regenerates stale sections correctly
- [ ] Fix is idempotent on fresh content
- [ ] Fix with multiple marker pairs uses on-disk state, not in-memory
- [ ] Fix leaves malformed markers unchanged
- [ ] Fix leaves template-execution-error sections unchanged
- [ ] `sort: path` orders case-insensitively by relative file path
- [ ] `sort: filename` orders by basename
- [ ] `sort: title` orders by front matter `title` field
- [ ] `sort: -title` orders descending
- [ ] Sort uses path as tiebreaker when values are equal
- [ ] Sort comparison is case-insensitive
- [ ] Sort with front matter key in minimal mode reads front matter
- [ ] Recursive `**` glob patterns are supported
- [ ] Dotfiles not matched by `*`/`**` unless pattern has leading dot
- [ ] Markers inside fenced code blocks are ignored
- [ ] Markers inside HTML blocks are ignored
- [ ] `-->` terminator allows leading/trailing whitespace
- [ ] Single-line start marker has empty YAML body
- [ ] Unreadable matched files are silently skipped
- [ ] Glob matching a directory silently skips it
- [ ] Stdin input skips the rule (`f.FS == nil`)
- [ ] Tests use `fstest.MapFS` (no `t.TempDir()` for unit tests)
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
