---
id: MDS064
name: atx-heading-whitespace
status: ready
description: ATX heading whitespace and indentation.
category: heading
nature: style
maintainability: null
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
