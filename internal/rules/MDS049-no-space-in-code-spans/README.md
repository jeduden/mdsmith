---
id: MDS049
name: no-space-in-code-spans
status: ready
description: Inline code spans must not have leading or trailing whitespace inside the backticks.
---
# MDS049: no-space-in-code-spans

Inline code spans must not have leading or trailing whitespace inside
the backticks.

CommonMark strips one space from each side of a code span when both
sides carry exactly one space (`` ` x ` `` renders as `x`). Any other
leading or trailing whitespace renders verbatim and is almost always a
typo: `` ` x` ``, `` `x ` ``, `` `  x  ` ``, `` `\tx` ``.

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

### Good

<?include
file: good/clean-spans.md
wrap: markdown
?>

```markdown
# Title

Use `x` for a clean code span.

The balanced single space ` x ` is also allowed.

Use `` `backtick` `` inside a double-backtick span.
```

<?/include?>

### Bad -- leading space

<?include
file: bad/leading-space.md
wrap: markdown
?>

```markdown
# Title

Use ` x` with a leading space.
```

<?/include?>

### Bad -- trailing space

<?include
file: bad/trailing-space.md
wrap: markdown
?>

```markdown
# Title

Use `x ` with a trailing space.
```

<?/include?>

### Bad -- double spaces on each side

<?include
file: bad/double-space-both-sides.md
wrap: markdown
?>

```markdown
# Title

Use `  x  ` with double spaces on each side.
```

<?/include?>

### Bad -- leading tab

<?include
file: bad/leading-tab.md
wrap: markdown
?>

```markdown
# Title

Use `	x` with a leading tab.
```

<?/include?>

### Bad -- empty after trim

<?include
file: bad/empty-after-trim.md
wrap: markdown
?>

```markdown
# Title

Use `   ` with only spaces inside.
```

<?/include?>

## Diagnostics

- `code span has leading whitespace`
- `code span has trailing whitespace`

## Edge Cases

- **CommonMark balanced single-space exception.** `` ` x ` `` is legal
  because CommonMark trims exactly one space from each side when both
  sides carry one. The rule skips this case.
- **Empty after trim.** When trimming would produce an empty span (e.g.
  `` `   ` ``), the rule emits both diagnostics but does not auto-fix,
  since an empty `` `` `` is ambiguous in context.
- **Preserves delimiter count.** The auto-fix trims only the whitespace
  inside the delimiters; the backtick sequence length is unchanged.

## Meta-Information

- **ID**: MDS049
- **Name**: `no-space-in-code-spans`
- **Status**: ready
- **Default**: disabled
- **Fixable**: yes (except when trimming produces an empty span)
- **Implementation**:
  [source](./)
- **Category**: whitespace
