---
id: MDS002
name: heading-style
status: ready
description: Heading style must be consistent.
category: heading
nature: style
maintainability: null
markdownlint:
  - id: MD003
    name: heading-style
    partial: false
    default: true
rumdl:
  - id: MD003
    name: heading-style
    partial: false
    default: true
mado:
  - id: MD003
    name: heading-style
    partial: false
    default: false
panache: []
obsidian-linter: []
gomarklint:
  - id: no-setext-headings
    name: no-setext-headings
    partial: false
    default: true
---
# MDS002: heading-style

Heading style must be consistent.

## Settings

| Setting | Type   | Default | Description                                                      |
| ------- | ------ | ------- | ---------------------------------------------------------------- |
| `style` | string | `"atx"` | `"atx"` (`# Heading`) or `"setext"` (underline with `===`/`---`) |

## Config

Enable (default):

```yaml
rules:
  heading-style:
    style: atx
```

Disable:

```yaml
rules:
  heading-style: false
```

Custom (setext style):

```yaml
rules:
  heading-style:
    style: setext
```

## Examples

### Bad (when style is `atx`)

Setext heading used when ATX is required:

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

Section
-------
```

<?/include?>

Setext heading with marker:

<?include
file: bad/with-marker.md
wrap: markdown
?>

```markdown
# Title

Section
-------

<?allow-empty-section?>
```

<?/include?>

### Bad (when style is `setext`)

ATX heading used when Setext is required:

<?include
file: bad/setext.md
wrap: markdown
?>

```markdown
# Title

## Section
```

<?/include?>

### Good (when style is `atx`)

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

## Section

Body text.
```

<?/include?>

### Good (when style is `setext`)

<?include
file: good/setext.md
wrap: markdown
?>

```markdown
Title
=====

Section
-------

Body text.
```

<?/include?>

## Diagnostics

| Message                          | Condition                                   |
| -------------------------------- | ------------------------------------------- |
| `heading style should be atx`    | `style: atx` and a Setext heading is found  |
| `heading style should be setext` | `style: setext` and an ATX heading is found |

## Meta-Information

- **ID**: MDS002
- **Name**: `heading-style`
- **Status**: ready
- **Default**: enabled, style: atx
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: heading
- **markdownlint**: [MD003][mdl-md003] (heading-style)
- **rumdl**: [MD003][rumdl-md003] (heading-style)
- **mado**: [MD003][mado-rules] (heading-style)
- **gomarklint**: [no-setext-headings][gomarklint-rules]

[mdl-md003]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md003.md
[rumdl-md003]: https://rumdl.dev/md003/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
[gomarklint-rules]: https://shinagawa-web.github.io/gomarklint/docs/rules/
