---
id: MDS049
name: no-space-in-code-spans
status: ready
description: >-
  Inline code spans must not have leading or trailing whitespace
  inside the backticks.
---
# MDS049: no-space-in-code-spans

Inline code spans must not have leading or trailing whitespace
inside the backticks.

CommonMark strips one optional space from each side of a code span
when both sides carry exactly one space (`` ` x ` `` renders as
`x`). Any other leading or trailing whitespace renders verbatim and
is almost always a typo: `` ` x` ``, `` `x ` ``, `` `  x  ` ``.

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

Use ` x` for a variable name.
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

Use `x ` for a variable name.
```

<?/include?>

### Bad -- double space each side

<?include
file: bad/both-sides.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Both Sides

Use `  x  ` for a variable name.
```

<?/include?>

### Bad -- leading tab

<?include
file: bad/leading-tab.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Leading Tab

Use `	x` for a variable name.
```

<?/include?>

### Good

<?include
file: good/clean.md
wrap: markdown
?>

```markdown
# Clean Code Spans

Use `x` for a variable name.

Use `` `backtick` `` inside a double-backtick span.

The ` x ` span is valid because CommonMark strips
the balanced single space from each side.
```

<?/include?>

## Diagnostics

- `code span has leading whitespace`
- `code span has trailing whitespace`

## Edge Cases

- **CommonMark balanced-space exempt.** A span with exactly one
  space on each side and non-whitespace content between them
  (`` ` x ` ``) is the CommonMark single-space-trim case; the
  renderer strips both spaces and the output is clean. This rule
  skips it.
- **Empty-after-trim spans.** A span like `` `   ` `` (all spaces)
  emits diagnostics but is not auto-fixed because trimming would
  produce an empty body, which may not be the author's intent.
- **Multi-backtick delimiters.** The fix preserves the delimiter
  count (`` ``  x  `` `` fixes to `` ``x`` ``).

## Meta-Information

- **ID**: MDS049
- **Name**: `no-space-in-code-spans`
- **Status**: ready
- **Default**: disabled, opt-in
- **Fixable**: yes (except empty-after-trim spans)
- **Implementation**:
  [source](./)
- **Category**: whitespace
