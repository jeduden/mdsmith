---
id: MDS052
name: no-space-in-code-spans
status: ready
description: Inline code spans with leading or trailing whitespace inside the backticks are almost always typos; flag them.
---
# MDS052: no-space-in-code-spans

Inline code spans with leading or trailing whitespace inside the
backticks are almost always typos; flag them.

CommonMark strips one space from each side of a code span when the
content starts *and* ends with a space and is not entirely whitespace
(`` ` x ` `` → `x`, `` `  x ` `` → `` ` x` ``). Whitespace that
remains after this normalisation — a double space on one side, a tab,
a space on only one side, or a newline — renders verbatim. This rule
flags those cases.

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

### Good -- no spaces

<?include
file: good/clean.md
wrap: markdown
?>

```markdown
# Clean Code Spans

Use `x` for the value.

Use ` x ` when balanced single space is intentional.

Use `foo bar` for multi-word.
```

<?/include?>

## Diagnostics

| Message                             | Meaning                                           |
|-------------------------------------|---------------------------------------------------|
| `code span has leading whitespace`  | The first byte inside the backticks is whitespace |
| `code span has trailing whitespace` | The last byte inside the backticks is whitespace  |

## Meta-Information

- **ID**: MDS052
- **Name**: `no-space-in-code-spans`
- **Status**: ready
- **Default**: disabled, opt-in
- **Fixable**: yes (trims whitespace; empty-after-trim spans are not auto-fixed)
- **Implementation**:
  [source](./)
- **Category**: whitespace
