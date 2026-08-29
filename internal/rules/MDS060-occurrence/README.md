---
id: MDS060
name: occurrence
status: ready
description: >-
  A scope must contain each configured token or pattern between `min` and
  `max` times (inclusive). Counts prose only; fenced and indented code
  blocks are excluded.
category: prose
nature: content
maintainability: null
markdownlint: []
rumdl: []
mado: []
panache: []
obsidian-linter: []
gomarklint: []
---
# MDS060: occurrence

A scope must contain each configured token or pattern between `min` and
`max` times (inclusive). Counts prose only; fenced and indented code
blocks are excluded.

The rule walks every scope unit (file, heading-bounded section, or
paragraph) and counts non-overlapping occurrences of each token or regex
pattern match. Fenced and indented code blocks are never counted.
A diagnostic is emitted at the scope unit's first line when the count
falls below `min` or exceeds `max`.

`tokens` and `pattern` are mutually exclusive. Use `lists:` to pull
token sets from wordlist files without repeating them inline.

## Settings

| Setting          | Type            | Default     | Description                                                         |
| ---------------- | --------------- | ----------- | ------------------------------------------------------------------- |
| `scope`          | string          | `paragraph` | `file`, `section`, or `paragraph`                                   |
| `tokens`         | list of strings | `[]`        | Literal substrings to count. Populated by `lists:`. Mutually        |
|                  |                 |             | exclusive with `pattern`.                                           |
| `pattern`        | string          | `""`        | Go RE2 regex to match. Mutually exclusive with `tokens`.            |
| `min`            | integer         | `0`         | Minimum occurrences per scope unit (must be >= 0)                   |
| `max`            | integer         | `-1`        | Maximum occurrences per scope unit; `-1` is unbounded               |
| `count`          | string          | `each`      | `each` checks each token independently; `combined` sums all matches |
| `case-sensitive` | bool            | `false`     | When false, token matching ignores case                             |

## Config

Enable with em-dash density preset (at most two per paragraph):

```yaml
rules:
  occurrence:
    pattern: "—"
    scope: paragraph
    count: combined
    max: 2
```

Enable with term-density preset (buzzword list, at most twice per section):

```yaml
rules:
  occurrence:
    scope: section
    count: each
    max: 2
    lists:
      - buzzwords
```

Disable:

```yaml
rules:
  occurrence: false
```

## Examples

### Token exceeds max

<?include
file: bad/section-scope.md
wrap: markdown
?>

```markdown
# Title

jargon jargon jargon here.
```

<?/include?>

### Pattern under max

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

First — second — end.
```

<?/include?>

### Fenced code blocks excluded

<?include
file: good/fenced-code-excluded.md
wrap: markdown
?>

````markdown
# Title

keyword keyword.

```text
keyword keyword keyword
```
````

<?/include?>

## Meta-Information

- **ID**: MDS060
- **Name**: `occurrence`
- **Status**: ready
- **Default**: disabled
- **Fixable**: no
- **Implementation**: [source](./)
- **Category**: prose
