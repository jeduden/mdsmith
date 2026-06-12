---
id: MDS009
name: single-trailing-newline
status: ready
description: File must end with exactly one newline character.
category: whitespace
nature: style
maintainability: null
markdownlint:
  - id: MD047
    name: single-trailing-newline
    partial: false
    default: true
rumdl:
  - id: MD047
    name: file-end-newline
    partial: false
    default: true
mado:
  - id: MD047
    name: single-trailing-newline
    partial: false
    default: true
panache: []
obsidian-linter:
  - id: line-break-at-document-end
    name: line-break-at-document-end
    partial: false
    default: false
gomarklint:
  - id: final-blank-line
    name: final-blank-line
    partial: false
    default: true
---
# MDS009: single-trailing-newline

File must end with exactly one newline character.

## Config

Enable:

```yaml
rules:
  single-trailing-newline: true
```

Disable:

```yaml
rules:
  single-trailing-newline: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

Content here.
```

<?/include?>

The file ends with exactly one `\n` after the last line.

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

Content here.
```

<?/include?>

The file has no trailing newline after the last line.
The `<?include?>` output looks identical because the
wrap always adds a newline, but the actual fixture file
(`bad/default.md`) ends without one — that missing byte
is what the rule detects.

## Meta-Information

- **ID**: MDS009
- **Name**: `single-trailing-newline`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: whitespace
- **markdownlint**: [MD047][mdl-md047] (single-trailing-newline)
- **rumdl**: [MD047][rumdl-md047] (file-end-newline)
- **mado**: [MD047][mado-rules] (single-trailing-newline)
- **obsidian-linter**: [line-break-at-document-end]
- **gomarklint**: [final-blank-line][gomarklint-rules]

[mdl-md047]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md047.md
[rumdl-md047]: https://rumdl.dev/md047/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
[line-break-at-document-end]:
  https://platers.github.io/obsidian-linter/settings/spacing-rules/#line-break-at-document-end
[gomarklint-rules]: https://shinagawa-web.github.io/gomarklint/docs/rules/
