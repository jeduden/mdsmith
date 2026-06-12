---
id: MDS052
name: no-space-in-code-spans
status: ready
description: Inline code spans with leading or trailing whitespace inside the backticks are almost always typos; flag them.
category: whitespace
nature: style
maintainability: null
markdownlint:
  - id: MD038
    name: no-space-in-code
    partial: false
    default: true
rumdl:
  - id: MD038
    name: no-space-in-code
    partial: false
    default: true
mado:
  - id: MD038
    name: no-space-in-code
    partial: false
    default: true
panache: []
obsidian-linter: []
gomarklint: []
---
# MDS052: no-space-in-code-spans

Inline code spans with leading or trailing whitespace inside the
backticks are almost always typos; flag them.

CommonMark strips one space from each side of a code span when the
content starts *and* ends with a space and is not entirely whitespace
(`` ` x ` `` → `x`, `` `  x ` `` → `` ` x` ``). Whitespace that
remains after this normalisation — a double space on one side, a tab,
or a space on only one side — renders verbatim. Newlines inside code
spans are normalised to spaces by CommonMark, so a leading or trailing
newline renders as a leading or trailing space. This rule flags all of
those cases.

## Settings

This rule has no tunable settings. Enable or disable it as a unit.

## Config

Enable:

```yaml
rules:
  no-space-in-code-spans: true
```

Disable:

```yaml
rules:
  no-space-in-code-spans: false
```

## Examples

### Bad -- leading space

<?include
file: bad/leading-space.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Leading Space

Use ` x` here.
```

<?/include?>

### Bad -- trailing space

<?include
file: bad/trailing-space.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Trailing Space

Use `x ` here.
```

<?/include?>

### Bad -- double space on both sides

<?include
file: bad/double-space-both-sides.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Double Space Both Sides

Use `  x  ` here.
```

<?/include?>

### Good -- no visible boundary whitespace

<?include
file: good/clean.md
wrap: markdown
?>

```markdown
# Clean Code Spans

Use `x` for the value.

Use ` x ` — CommonMark's single-space trim strips both outer spaces.

Use `foo bar` for multi-word.
```

<?/include?>

## Diagnostics

| Message                             | Meaning                                                                                      |
| ----------------------------------- | -------------------------------------------------------------------------------------------- |
| `code span has leading whitespace`  | Whitespace is visible at the start of the span after CommonMark's single-space normalisation |
| `code span has trailing whitespace` | Whitespace is visible at the end of the span after CommonMark's single-space normalisation   |

## Meta-Information

- **ID**: MDS052
- **Name**: `no-space-in-code-spans`
- **Status**: ready
- **Default**: disabled, opt-in
- **Fixable**: yes (trims whitespace; spans that are empty after trimming or
  whose trimmed content starts/ends with a backtick adjacent to a delimiter
  are left unchanged or have a single protective space preserved)
- **Implementation**:
  [source](./)
- **Category**: whitespace
- **markdownlint**: [MD038][mdl-md038] (no-space-in-code)
- **rumdl**: [MD038][rumdl-md038] (no-space-in-code)
- **mado**: [MD038][mado-rules] (no-space-in-code)

[mdl-md038]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md038.md
[rumdl-md038]: https://rumdl.dev/md038/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
