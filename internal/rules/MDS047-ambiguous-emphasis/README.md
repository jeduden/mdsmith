---
id: MDS047
name: ambiguous-emphasis
status: ready
description: Forbid emphasis sequences whose meaning a human cannot predict at a glance.
---
# MDS047: ambiguous-emphasis

Forbid emphasis sequences whose meaning a human cannot predict at a glance.

CommonMark pairs `*` and `_` runs by counting flanking delimiters. The
output is well-defined; the source is not. This rule names three shapes
that read poorly and refuses them.

The shapes are:

- long runs of the same delimiter character
- backslash escapes glued to a run
- the same delimiter repeated three times across word boundaries

The check runs on raw source bytes. It skips ranges covered by inline
code spans, fenced code blocks, and indented code blocks.

## Settings

| Setting                      | Type | Default | Description                                                                  |
|------------------------------|------|---------|------------------------------------------------------------------------------|
| `max-run`                    | int  | 0       | Maximum allowed length of a contiguous `*` or `_` run; 0 disables the check  |
| `forbid-escaped-in-run`      | bool | false   | Flag `*\*` or `_\_` where a backslash-escaped delimiter butts against a run  |
| `forbid-adjacent-same-delim` | bool | false   | Flag three same-delimiter runs glued by non-whitespace (`*a*b*`, `__a__b__`) |

The defaults make the rule a no-op even when enabled, so it ships safe.
Profile activation in plan 112 (`portable`, `plain`) sets `max-run: 2`
and both bool flags to `true`. User overrides on top still win via
deep-merge.

## Config

Enable with the active profile values:

```yaml
rules:
  ambiguous-emphasis:
    max-run: 2
    forbid-escaped-in-run: true
    forbid-adjacent-same-delim: true
```

Disable:

```yaml
rules:
  ambiguous-emphasis: false
```

Long-run only (skip the other two detectors):

```yaml
rules:
  ambiguous-emphasis:
    max-run: 2
```

## Examples

### Bad -- long delimiter run

<?include
file: bad/long-run.md
wrap: markdown
?>

```markdown
# Title

***bold-italic*** at the start of a paragraph.
```

<?/include?>

### Bad -- escaped delimiter inside a run

<?include
file: bad/escaped-in-run.md
wrap: markdown
?>

```markdown
# Title

*****\*a* in the rant string.
```

<?/include?>

### Bad -- adjacent same-delimiter runs

<?include
file: bad/adjacent-same-delim.md
wrap: markdown
?>

```markdown
# Title

__a__b__ ambiguous between bold(a) literal-b and literal-a bold(b).
```

<?/include?>

### Bad -- the rant's Peter Piper example

<?include
file: bad/peter-piper.md
wrap: markdown
?>

```markdown
# Title

***Peter* Piper** is the rant's other Exhibit-A example.
```

<?/include?>

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

This sentence has **bold** and *italic* and even `*****\*literal*` in a
code span without flagging.

Multiple separate emphases like *one* and *two* and *three* read
naturally because spaces split the runs.
```

<?/include?>

### Good -- patterns inside code spans

<?include
file: good/in-code-span.md
wrap: markdown
?>

```markdown
# In code span

The string `*****\*a*` inside an inline code span must not flag, and
neither must `__a__b__` nor `***Peter* Piper**` when wrapped this way.
```

<?/include?>

### Good -- patterns inside fenced code blocks

<?include
file: good/in-fenced-block.md
wrap: markdown
?>

````markdown
# In fenced block

The patterns below sit inside a fenced code block and must not flag.

```text
*****\*a*
***bold-italic***
__a__b__
***Peter* Piper**
```
````

<?/include?>

## Diagnostics

- `emphasis run of {n} delimiters; max is {max-run}`
- `escaped delimiter inside emphasis run`
- `adjacent same-delimiter emphasis is ambiguous`

## Edge Cases

- **No auto-fix.** The right rewrite depends on author intent: add a
  space, swap to an HTML entity, or split the run. The rule reports
  and lets the author choose.
- **Symmetric openers and closers collapse.** `***x***` has two
  three-star runs but emits a single long-run diagnostic, anchored at
  the first occurrence on the line.
- **Whitespace clears the gap.** `*a* *b* *c*` does not flag adjacent
  same-delimiter because each gap contains a space; CommonMark resolves
  the runs unambiguously.
- **Interaction with MDS042.** MDS042 (emphasis-style) pins which
  delimiter is used. MDS047 catches the ambiguous combinations that
  survive a delimiter pin. Both can fire on the same line.

## Meta-Information

- **ID**: MDS047
- **Name**: `ambiguous-emphasis`
- **Status**: ready
- **Default**: disabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta
