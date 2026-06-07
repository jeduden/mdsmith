---
id: MDS064
name: atx-heading-whitespace
status: ready
description: ATX heading whitespace and indentation.
category: heading
nature: style
maintainability: null
markdownlint:
  - id: MD018
    name: no-missing-space-atx
    partial: false
    default: true
  - id: MD019
    name: no-multiple-space-atx
    partial: false
    default: true
  - id: MD020
    name: no-missing-space-closed-atx
    partial: true
    default: true
  - id: MD021
    name: no-multiple-space-closed-atx
    partial: false
    default: true
  - id: MD023
    name: heading-start-left
    partial: false
    default: true
rumdl:
  - id: MD018
    name: no-space-atx
    partial: false
    default: true
  - id: MD019
    name: multiple-space-atx
    partial: false
    default: true
  - id: MD020
    name: no-space-closed-atx
    partial: true
    default: true
  - id: MD021
    name: multiple-space-closed-atx
    partial: false
    default: true
  - id: MD023
    name: heading-start-left
    partial: false
    default: true
mado:
  - id: MD018
    name: no-missing-space-atx
    partial: false
    default: true
  - id: MD019
    name: no-multiple-space-atx
    partial: false
    default: true
  - id: MD020
    name: no-missing-space-closed-atx
    partial: true
    default: false
  - id: MD021
    name: no-multiple-space-closed-atx
    partial: false
    default: true
  - id: MD023
    name: heading-start-left
    partial: false
    default: true
panache: []
obsidian-linter:
  - id: headings-start-line
    name: headings-start-line
    partial: true
    default: false
---
# MDS064: atx-heading-whitespace

ATX heading whitespace and indentation.

Flags malformed ATX headings. When the heading has content, checks that the
opening hashes are followed by exactly one space (not a tab, not two spaces);
empty headings (`##` with nothing after the hashes) are valid. Also checks
that the heading starts at column 1, and that no closing hash sequence appears
after the content.
A trailing `#` run is only treated as a closing marker when preceded by
whitespace; a `#` with no preceding space is kept as content.

## Config

Enable:

```yaml
rules:
  atx-heading-whitespace: true
```

Disable:

```yaml
rules:
  atx-heading-whitespace: false
```

## Examples

### Bad

Missing space after `#`:

```markdown
#Heading
```

Multiple spaces after `#`:

```markdown
#  Heading
```

Indented heading:

```markdown
   # Heading
```

Closing `#` marker (any whitespace before `#`):

```markdown
# Heading #
```

Multiple spaces before closing `#`:

```markdown
# Heading  #
```

Tab after opening `#`:

```markdown
#	Heading
```

### Good

```markdown
# Heading

## Section

### Subsection
```

## Meta-Information

- **ID**: MDS064
- **Name**: `atx-heading-whitespace`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**: [source](./)
- **Category**: heading
- **markdownlint**:
  - [MD018][mdl-md018] (no-missing-space-atx)
  - [MD019][mdl-md019] (no-multiple-space-atx)
  - [MD020][mdl-md020] (no-missing-space-closed-atx) (partial)
  - [MD021][mdl-md021] (no-multiple-space-closed-atx)
  - [MD023][mdl-md023] (heading-start-left)
- **rumdl**:
  - [MD018][rumdl-md018] (no-space-atx)
  - [MD019][rumdl-md019] (multiple-space-atx)
  - [MD020][rumdl-md020] (no-space-closed-atx) (partial)
  - [MD021][rumdl-md021] (multiple-space-closed-atx)
  - [MD023][rumdl-md023] (heading-start-left)
- **mado**:
  - [MD018][mado-rules] (no-missing-space-atx)
  - [MD019][mado-rules] (no-multiple-space-atx)
  - [MD020][mado-rules] (no-missing-space-closed-atx) (partial)
  - [MD021][mado-rules] (no-multiple-space-closed-atx)
  - [MD023][mado-rules] (heading-start-left)
- **obsidian-linter**: [headings-start-line] (partial)

[mdl-md018]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md018.md
[mdl-md019]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md019.md
[mdl-md020]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md020.md
[mdl-md021]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md021.md
[mdl-md023]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md023.md
[rumdl-md018]: https://rumdl.dev/md018/
[rumdl-md019]: https://rumdl.dev/md019/
[rumdl-md020]: https://rumdl.dev/md020/
[rumdl-md021]: https://rumdl.dev/md021/
[rumdl-md023]: https://rumdl.dev/md023/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
[headings-start-line]:
  https://platers.github.io/obsidian-linter/settings/heading-rules/#headings-start-line
