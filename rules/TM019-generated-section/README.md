# TM019: generated-section

Generated sections must match the content their directive would produce.

- **ID**: TM019
- **Name**: `generated-section`
- **Default**: enabled
- **Fixable**: yes
- **Implementation**: [`internal/rules/generatedsection/`](../../internal/rules/generatedsection/)

## Marker Syntax

Markers are HTML comments that delimit a generated section:

```
<!-- tidymark:gen:start DIRECTIVE
key: value
-->
...generated content...
<!-- tidymark:gen:end -->
```

The opening comment has two parts:

1. **First line** -- `<!-- tidymark:gen:start DIRECTIVE` where DIRECTIVE is the
   directive name in lowercase (e.g., `catalog`). The directive name is
   case-sensitive; `Catalog` or `CATALOG` trigger the "unknown directive"
   diagnostic. The directive name is required; omitting it produces a
   diagnostic. Leading and trailing whitespace around the directive name is
   trimmed. Only the first whitespace-delimited word after `start` is used as
   the directive name; additional text on the first line is ignored.
2. **YAML body** -- all subsequent lines until `-->` are parsed as YAML. This
   body contains both parameters (e.g., `glob`, `sort`) and template sections
   (e.g., `header`, `row`, `footer`, `empty`).

The closing `-->` is recognized when a line, after trimming leading and
trailing whitespace, equals `-->`. The YAML body may contain any valid YAML
except that such a `-->` line terminates the comment. If `-->` appears on its
own line within a YAML value, the HTML comment terminates prematurely, likely
causing an invalid YAML diagnostic. Avoid `-->` inside YAML values.

If `-->` appears on the same line as the directive name (e.g.,
`<!-- tidymark:gen:start catalog -->`), the YAML body is empty. This is valid
syntax but will typically trigger a missing-parameter diagnostic.

The directive operates in *template mode* when the YAML body contains a `row`
key, and in *minimal mode* otherwise. See Minimal mode below.

### YAML keys

Parameters and template sections share the same YAML namespace. All values
must be strings. Non-string values (null, numbers, booleans, arrays, maps) produce
a diagnostic per key.

Duplicate YAML keys use the last value (standard YAML behavior). YAML anchors,
aliases, and merge keys are supported (standard YAML features).

**Parameters** (directive-specific):

| Key | Type | Description |
|-----|------|-------------|
| `glob` | string | File glob pattern |
| `sort` | string | Sort key with optional direction prefix |

**Template sections:**

| Key | Required | Description |
|-----|----------|-------------|
| `header` | no | Static text rendered once at the top. No template expansion; `{{...}}` appears literally. |
| `row` | conditional | Per-file block, rendered once per matched file. Uses Go `text/template` syntax. Required when `header` or `footer` is present. Must be non-empty (empty string or whitespace-only produces diagnostic). |
| `footer` | no | Static text rendered once at the bottom. No template expansion; `{{...}}` appears literally. |
| `empty` | no | Fallback text rendered instead of header+rows+footer when glob matches zero files. No template expansion; `{{...}}` appears literally. Can appear alone without `row`. However, if `header` or `footer` is also present, `row` is required regardless of whether `empty` is defined. |

Single-line values use YAML string syntax. Multi-line values use YAML literal
block scalars (`|`, `|+`, or `|-`):

```yaml
row: "- [{{.title}}]({{.filename}}) — {{.description}}"
```

```yaml
header: |
  | Title | Description |
  |-------|-------------|
```

Note: YAML requires quoting values that contain special characters like `{`,
`}`, `|`, `:`, or `#`. Use double quotes for single-line templates containing
`{{...}}`.

The YAML parser strips the leading indentation from literal block content, so
markdown indentation is preserved correctly (e.g., a 4-space code block inside
a 2-space-indented `|` block comes out with the correct 4 spaces).

Use `|+` (keep chomp) to preserve trailing blank lines in multi-line values.
Use `|` (clip chomp, default) when no trailing blank line is needed.
Use `|-` (strip chomp) to remove all trailing newlines; the implicit trailing
newline rule (see Rendering logic) will add one back.

### Template fields

The `row` section uses Go `text/template` syntax: `{{.fieldname}}`.

- `{{.filename}}` -- the matched file's path, relative to the directory of
  the file containing the marker. Never includes a leading `./` prefix. Note:
  despite the name, this is a relative path (e.g., `docs/api-reference.md`),
  not a basename.
- All other names (e.g., `{{.title}}`, `{{.description}}`, `{{.author}}`) --
  looked up in the matched file's YAML front matter (delimited by `---` lines
  at the start of the file)
- Missing front matter field -> empty string
- Non-string front matter value -> converted to string via Go's default
  formatting. Complex types (arrays, maps) produce unhelpful strings; prefer
  flat string fields in front matter for template use.
- No custom template functions are registered. Go's built-in template
  functions (e.g., `print`, `len`, `index`) are available.
- Template output is not HTML-escaped. Values containing `<`, `>`, `&`, or
  other markdown-significant characters appear literally in the output.

### Rendering logic

1. If glob matches files: output = `header` + (`row` rendered per file) +
   `footer`. The `empty` value is ignored when files are matched.
2. If glob matches no files and `empty` is defined: output = `empty` text
3. If glob matches no files and no `empty` key: output is empty (zero
   lines between markers)

Each rendered value (`header`, `row`, `footer`, `empty`) is terminated by a
`\n` character. If the value (as parsed from YAML) already ends with `\n`, no
additional `\n` is appended. This applies uniformly to all sections.

In minimal mode, each entry is terminated by a `\n`.

Use `|+` in the row template to include a trailing blank line between entries.

Validation is sequential. Structural errors (missing directive name, invalid
YAML, non-string values) short-circuit further validation. Parameter validation
(missing glob, invalid sort) is performed only after YAML parsing succeeds.

### General rules

- Markers inside fenced code blocks or HTML blocks are ignored.
- Multiple independent marker pairs per file are supported.
- Content between markers starts on the line immediately after the start
  marker's closing `-->` line and ends on the line immediately before the
  `<!-- tidymark:gen:end -->` line. Comparison is performed on the exact text
  between the markers (preserving original line endings) versus the freshly
  generated text (which uses `\n` line endings). Any difference, including
  line-ending mismatches, constitutes a mismatch.
- The end marker is recognized when a line, after trimming leading and
  trailing whitespace, equals `<!-- tidymark:gen:end -->`. It must appear on
  its own line (no other content on that line).
- All diagnostics are reported at column 1.
- Diagnostics are reported on the start marker line, except for orphaned
  end markers which are reported on the end marker line.

## Directive: `catalog`

Lists files matching a glob pattern with configurable output.

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `glob` | yes | -- | File glob pattern, resolved relative to the directory of the file containing the marker. Supports `*`, `?`, `[...]`, and `**` (recursive). Absolute paths are rejected. Parent traversal (`..`) is not supported. Brace expansion (`{a,b}`) is not supported. Dotfiles are not matched by `*` or `**` unless the pattern explicitly includes a leading dot. |
| `sort` | no | `path` | Sort key with optional `-` prefix for descending order. See Sort behavior. |

### Sort behavior

The `sort` value has the format `[-]KEY` where:

- `-` prefix (optional): descending order. Without prefix, ascending order.
  Only the first `-` is consumed as the direction prefix; `sort: --priority`
  means descending by key `-priority`.
- `KEY`: a non-empty string without whitespace. No whitespace trimming is
  performed on the key after stripping the prefix. One of the built-in keys
  or any front matter field name. A sort value containing whitespace (e.g.,
  `sort: "foo bar"`) triggers the "invalid sort value" diagnostic.

If the sort value is empty (`sort: ""`), the "empty sort value" diagnostic
is emitted. If the key name is empty after stripping the `-` prefix (i.e.,
`sort: "-"`), the "invalid sort value" diagnostic is emitted.

**Built-in keys:**

| Key | Description |
|-----|-------------|
| `path` | Relative file path (default) |
| `filename` | File basename |

Any other key name is looked up in the matched file's YAML front matter.
Missing front matter values sort as empty string.

Comparison is case-insensitive. When values are equal, files are secondarily
sorted by relative file path (ascending, case-insensitive) for deterministic
output.

Examples:

- `sort: path` -- ascending by relative file path (default)
- `sort: -path` -- descending by relative file path
- `sort: title` -- ascending by front matter `title`
- `sort: -date` -- descending by front matter `date`
- `sort: filename` -- ascending by file basename

### Minimal mode

When the YAML body contains no `row` key (and no `header` or `footer`), the
directive produces a plain bullet list with one entry per matched file:

```
- [<basename>](<relative-path>)
```

Link text is the file's basename (e.g., `api-reference.md`). Link target is
the file's path relative to the directory of the file containing the marker
(the same value as `{{.filename}}` in template mode). The link target never
includes a leading `./` prefix.

Front matter is read only when the sort key references a front matter field.
Otherwise, no front matter extraction is performed.

## Settings

This rule has no configurable settings. All configuration lives in the
directive markers themselves.

## Config

```yaml
rules:
  generated-section: true
```

Disable:

```yaml
rules:
  generated-section: false
```

## Examples

### Good -- minimal (glob only, no template)

```markdown
<!-- tidymark:gen:start catalog
glob: docs/*.md
-->
- [api-reference.md](docs/api-reference.md)
- [getting-started.md](docs/getting-started.md)
<!-- tidymark:gen:end -->
```

### Good -- list with front matter

```markdown
<!-- tidymark:gen:start catalog
glob: docs/*.md
row: "- [{{.title}}]({{.filename}}) — {{.description}}"
-->
- [API Reference](docs/api-reference.md) — Complete API documentation
- [Getting Started](docs/getting-started.md) — How to get started
<!-- tidymark:gen:end -->
```

### Good -- table with header

```markdown
<!-- tidymark:gen:start catalog
glob: docs/*.md
header: |
  | Title | Description |
  |-------|-------------|
row: "| [{{.title}}]({{.filename}}) | {{.description}} |"
-->
| Title | Description |
|-------|-------------|
| [API Reference](docs/api-reference.md) | Complete API documentation |
| [Getting Started](docs/getting-started.md) | How to get started |
<!-- tidymark:gen:end -->
```

### Good -- table with header and footer

The blank line before `---` comes from the leading empty line in the
`footer: |` block scalar.

```markdown
<!-- tidymark:gen:start catalog
glob: docs/*.md
header: |
  | Title | Description |
  |-------|-------------|
row: "| [{{.title}}]({{.filename}}) | {{.description}} |"
footer: |

  ---
-->
| Title | Description |
|-------|-------------|
| [API Reference](docs/api-reference.md) | Complete API documentation |
| [Getting Started](docs/getting-started.md) | How to get started |

---
<!-- tidymark:gen:end -->
```

### Good -- multi-line row (rich layout)

Use `|+` to preserve the trailing blank line between entries:

```markdown
<!-- tidymark:gen:start catalog
glob: docs/*.md
row: |+
  ### [{{.title}}]({{.filename}})

  {{.description}}

-->
### [API Reference](docs/api-reference.md)

Complete API documentation

### [Getting Started](docs/getting-started.md)

How to get started

<!-- tidymark:gen:end -->
```

### Good -- empty fallback only

`empty` can appear without `row` to provide fallback text for an empty glob:

```markdown
<!-- tidymark:gen:start catalog
glob: docs/*.md
empty: No documents found.
-->
No documents found.
<!-- tidymark:gen:end -->
```

### Good -- empty fallback with template

When the glob matches zero files, the `empty` text is rendered regardless of
whether `header`, `row`, and `footer` are also defined:

```markdown
<!-- tidymark:gen:start catalog
glob: docs/*.md
header: |
  | Title | Description |
  |-------|-------------|
row: "| [{{.title}}]({{.filename}}) | {{.description}} |"
empty: No documents found.
-->
No documents found.
<!-- tidymark:gen:end -->
```

### Good -- descending sort by front matter field

```markdown
<!-- tidymark:gen:start catalog
glob: docs/*.md
sort: -title
row: "- [{{.title}}]({{.filename}})"
-->
- [Getting Started](docs/getting-started.md)
- [API Reference](docs/api-reference.md)
<!-- tidymark:gen:end -->
```

### Bad -- stale section (content does not match)

A new file `docs/tutorial.md` was added but the section was not regenerated:

```markdown
<!-- tidymark:gen:start catalog
glob: docs/*.md
row: "- [{{.title}}]({{.filename}}) — {{.description}}"
-->
- [API Reference](docs/api-reference.md) — Complete API documentation
<!-- tidymark:gen:end -->
```

Diagnostic: `generated section is out of date`

### Bad -- unclosed marker

```markdown
<!-- tidymark:gen:start catalog
glob: docs/*.md
-->
- [api-reference.md](docs/api-reference.md)
```

Diagnostic: `generated section has no closing marker`

### Bad -- missing directive name

```markdown
<!-- tidymark:gen:start
glob: docs/*.md
-->
<!-- tidymark:gen:end -->
```

Diagnostic: `generated section marker missing directive name`

### Bad -- unknown directive

```markdown
<!-- tidymark:gen:start foobar
glob: docs/*.md
-->
<!-- tidymark:gen:end -->
```

Diagnostic: `unknown generated section directive "foobar"`

### Bad -- nested markers

```markdown
<!-- tidymark:gen:start catalog
glob: docs/*.md
-->
<!-- tidymark:gen:start catalog
glob: other/*.md
-->
<!-- tidymark:gen:end -->
```

Diagnostic: `nested generated section markers are not allowed`

### Bad -- template sections without `row`

```markdown
<!-- tidymark:gen:start catalog
glob: docs/*.md
header: |
  | Title | Description |
  |-------|-------------|
-->
<!-- tidymark:gen:end -->
```

Diagnostic: `generated section template missing required "row" key`

### Bad -- invalid YAML

```markdown
<!-- tidymark:gen:start catalog
glob: docs/*.md
row: [invalid
-->
<!-- tidymark:gen:end -->
```

Diagnostic: `generated section has invalid YAML: ...`

### Bad -- non-string YAML value

```markdown
<!-- tidymark:gen:start catalog
glob: 42
-->
<!-- tidymark:gen:end -->
```

Diagnostic: `generated section has non-string value for key "glob"`

### Bad -- orphaned end marker

```markdown
Some text
<!-- tidymark:gen:end -->
```

Diagnostic: `unexpected generated section end marker`

### Bad -- absolute glob path

```markdown
<!-- tidymark:gen:start catalog
glob: /etc/*.md
-->
<!-- tidymark:gen:end -->
```

Diagnostic: `generated section directive has absolute glob path`

## Diagnostics

| Condition | Message | Reported on |
|-----------|---------|-------------|
| Content mismatch | `generated section is out of date` | start marker line |
| No closing marker | `generated section has no closing marker` | start marker line |
| Orphaned end marker | `unexpected generated section end marker` | end marker line |
| Nested start markers | `nested generated section markers are not allowed` | nested start line |
| Missing directive name | `generated section marker missing directive name` | start marker line |
| Unknown directive | `unknown generated section directive "NAME"` | start marker line |
| Invalid YAML body | `generated section has invalid YAML: ...` | start marker line |
| Non-string YAML value | `generated section has non-string value for key "KEY"` | start marker line |
| Missing `glob` | `generated section directive missing required "glob" parameter` | start marker line |
| Empty `glob` value | `generated section directive has empty "glob" parameter` | start marker line |
| Absolute glob path | `generated section directive has absolute glob path` | start marker line |
| Glob with `..` | `generated section directive has glob pattern with ".." path traversal` | start marker line |
| Invalid glob | `generated section directive has invalid glob pattern: ...` | start marker line |
| Empty sort value | `generated section directive has empty "sort" value` | start marker line |
| Invalid sort value | `generated section directive has invalid sort value "VALUE"` | start marker line |
| Empty `row` value | `generated section directive has empty "row" value` | start marker line |
| `header`/`footer` without `row` | `generated section template missing required "row" key` | start marker line |
| Invalid template syntax | `generated section has invalid template: ...` | start marker line |
| Template execution error | `generated section template execution failed: ...` | start marker line |

## Fix Behavior

- Replace content between valid marker pairs with freshly generated content
- Leave malformed marker regions unchanged
- Leave sections unchanged when template execution fails
- Idempotent: fixing an up-to-date file produces no changes
- When fixing multiple marker pairs in one file, each pair's generation uses
  the on-disk filesystem state, not the partially-fixed in-memory content

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| Glob matches no files | Empty text or `empty` fallback text |
| Glob matches files + `empty` defined | `empty` is ignored; header+rows+footer rendered |
| File has no front matter | `{{.filename}}` works; other fields -> empty string |
| Matched file has invalid front matter | Treated as no front matter (all fields -> empty string) |
| Front matter field missing | Empty string in template output |
| No filesystem context (stdin or in-memory) | Rule skipped entirely (cannot resolve relative globs) |
| Unreadable matched file | Silently skipped (not included in output) |
| Glob matches a directory | Silently skipped |
| Glob matches the linted file | Included (uses current on-disk content for front matter) |
| Matched file is binary or non-Markdown | Included; no front matter extracted; `{{.filename}}` resolves |
| Markers inside fenced code blocks | Ignored |
| Markers inside HTML blocks | Ignored |
| Multiple marker pairs in one file | Each processed independently |
| Symlinks in glob results | Followed (doublestar handles cycles) |
| Dotfiles | Not matched by `*` or `**` unless pattern has leading dot |
| Absolute glob path | Diagnostic |
| Glob with `..` | Diagnostic (parent traversal not supported by `io/fs.FS`) |
| Brace expansion in glob | Not supported; treated as literal characters |
| Empty glob value | Diagnostic |
| Empty `row` value | Diagnostic (empty string or whitespace-only) |
| Empty `sort` value | Diagnostic |
| `sort: "-"` (dash only) | Diagnostic (invalid sort value) |
| Sort value with whitespace | Diagnostic (invalid sort value) |
| Unknown YAML keys | Ignored (forward-compatible with future parameters) |
| Non-string YAML values | Diagnostic per key |
| `empty` without `row` | Valid; only `header`/`footer` require `row` |
| `empty` + `header` without `row` | Diagnostic (missing `row` still fires) |
| Duplicate YAML keys | Last value wins (standard YAML) |
| Single-line start marker | Valid; empty YAML body triggers missing-parameter diagnostic |
| Windows-style line endings (`\r\n`) | Generated content uses `\n`; will flag `\r\n` files as stale |
| Template execution error | Diagnostic emitted; fix leaves section unchanged |
