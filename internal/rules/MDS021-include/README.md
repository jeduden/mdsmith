---
id: MDS021
name: include
status: ready
description: Include section content must match the referenced file.
---
# MDS021: include

Include section content must match the referenced file.

## Marker Syntax

```text
<?include
file: path/to/file.md
strip-frontmatter: "true"
wrap: markdown
heading-level: "absolute"
?>
...included content...
<?/include?>
```

Do not use YAML folded scalars (`>`, `>-`) in the YAML
body. Markdown parsers interpret `>` at the start of a
line as a blockquote marker, which breaks the processing
instruction content. Use literal block scalars (`|`,
`|-`, `|+`) or quoted strings instead. See the
[generated-section docs](../../../docs/background/concepts/generated-section.md)
for details.

## Parameters

| Parameter           | Required | Default  | Description                                       |
|---------------------|----------|----------|---------------------------------------------------|
| `file`              | yes      | --       | Relative path to include                          |
| `strip-frontmatter` | no       | `"true"` | Remove YAML frontmatter                           |
| `wrap`              | no       | --       | Wrap in code fence (value = language)             |
| `heading-level`     | no       | --       | `"absolute"`: shift headings to nest under parent |

## Link Adjustment

Relative link and image targets in included content are
automatically rewritten so they resolve from the
including file's directory. Absolute URLs, anchor-only
links (`#foo`), and protocol links (`http://`,
`https://`, `mailto:`) are not modified.

For example, `DEVELOPMENT.md` contains
`[rules](internal/rules/)`. When included from
`docs/guide.md`, the link becomes
`[rules](../internal/rules/)`.

## Heading-Level Adjustment

When `heading-level: "absolute"` is set, included
headings shift so the top-level heading becomes a
child of the enclosing section.

Example: the marker sits under `## Project` (level 2).
The included file has `## Build` (level 2) and
`### Sub` (level 3). The shift is
`2 - 2 + 1 = 1`. Result: `### Build` (3) and
`#### Sub` (4). Levels are capped at 6.

When the marker sits at document root (no preceding
heading), no shift is applied.

## Cycle Detection

Include chains are tracked during check and fix.
A diagnostic is emitted when:

- A file includes itself (direct cycle).
- A file includes B which includes A (indirect
  cycle, detected by scanning nested includes).
- The include chain exceeds 10 levels deep.

The cycle message shows the full chain:

```text
cyclic include: a.md -> b.md -> a.md
```

## Config

```yaml
rules:
  include: true
```

Disable:

```yaml
rules:
  include: false
```

## Examples

### Basic Include

```markdown
<?include
file: data.md
?>
Hello world
<?/include?>
```

### With Code Fence Wrapping

````markdown
<?include
file: config.yml
wrap: yaml
?>
```yaml
key: value
```
<?/include?>
````

### With Frontmatter Kept

```markdown
<?include
file: data.md
strip-frontmatter: "false"
?>
---
title: My Doc
---
Content here.
<?/include?>
```

### With Heading-Level Shift

Given `DEVELOPMENT.md` contains `## Build` and
`### Sub`, including under `## Project` shifts
headings one level down:

```markdown
## Project

<?include
file: DEVELOPMENT.md
heading-level: "absolute"
?>
### Build

Steps here.

#### Sub

Details.
<?/include?>
```

### With Link Rewriting

Given `DEVELOPMENT.md` in the repo root contains
`[rules](internal/rules/)`, including it from
`docs/guide.md` rewrites the link:

```markdown
<?include
file: DEVELOPMENT.md
?>
See [rules](../internal/rules/) for details.
<?/include?>
```

### Bad — Outdated Content

```markdown
<?include
file: data.md
?>
Outdated content
<?/include?>
```

## Diagnostics

| Condition             | Message                                                            |
|-----------------------|--------------------------------------------------------------------|
| content mismatch      | generated section is out of date                                   |
| missing file          | include file "x.md" not found                                      |
| no file param         | include directive missing required "file" parameter                |
| absolute path         | include directive has absolute file path                           |
| escapes root          | include file path escapes project root                             |
| no root for dotdot    | include file path contains ".." but project root is not configured |
| invalid heading-level | include directive "heading-level" must be "absolute"               |
| cyclic include        | cyclic include: a.md -> b.md -> a.md                               |
| depth exceeded        | include depth exceeds maximum (10)                                 |

## Meta-Information

- **ID**: MDS021
- **Name**: `include`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: meta
- **Concept**:
  [generated-section](../../../docs/background/concepts/generated-section.md)
- **Guide**:
  [directive guide](../../../docs/guides/directives/generating-content.md)
