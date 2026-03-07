---
id: MDS021
name: include
status: ready
description: Include section content must match the referenced file.
---
# MDS021: include

Include section content must match the referenced file.

- **ID**: MDS021
- **Name**: `include`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: meta
- **Archetype**:
  [generated-section](../../../docs/design/archetypes/generated-section/)

## Marker Syntax

```text
<?include
file: path/to/file.md
strip-frontmatter: "true"
wrap: markdown
?>
...included content...
<?/include?>
```

Do not use YAML folded scalars (`>`, `>-`) in the YAML
body. Markdown parsers interpret `>` at the start of a
line as a blockquote marker, which breaks the processing
instruction content. Use literal block scalars (`|`,
`|-`, `|+`) or quoted strings instead. See the
[archetype docs](../../../docs/design/archetypes/generated-section/)
for details.

## Parameters

| Parameter         | Required | Default | Description                           |
|-------------------|----------|---------|---------------------------------------|
| `file`              | yes      | --      | Relative path to include              |
| `strip-frontmatter` | no       | `"true"`  | Remove YAML frontmatter               |
| `wrap`              | no       | --      | Wrap in code fence (value = language) |

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

### Good

```markdown
<?include
file: data.md
?>
Hello world
<?/include?>
```

### Bad

```markdown
<?include
file: data.md
?>
Outdated content
<?/include?>
```

## Diagnostics

| Condition        | Message                                             |
|------------------|-----------------------------------------------------|
| content mismatch | generated section is out of date                    |
| missing file     | include file "x.md" not found                       |
| no file param    | include directive missing required "file" parameter |
| absolute path    | include directive has absolute file path            |
| path traversal   | include directive has file path with ".." traversal |
