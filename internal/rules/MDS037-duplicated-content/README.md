---
id: MDS037
name: duplicated-content
status: ready
description: Paragraphs should not repeat verbatim across Markdown files.
---
# MDS037: duplicated-content

Paragraphs should not repeat verbatim across Markdown files.

- **ID**: MDS037
- **Name**: `duplicated-content`
- **Status**: ready
- **Default**: disabled (opt-in via `.mdsmith.yml`);
  include: [], exclude: [], min-chars: 200
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta

## Settings

| Setting     | Type | Default | Description                                    |
|-------------|------|---------|------------------------------------------------|
| `include`   | list | `[]`    | glob patterns limiting which siblings to scan  |
| `exclude`   | list | `[]`    | glob patterns of siblings to skip              |
| `min-chars` | int  | `200`   | minimum normalized paragraph length to compare |

Before comparing, the rule normalizes each paragraph. Whitespace
collapses to single spaces. Letters become lowercase. Leading and
trailing space is trimmed. A paragraph shorter than `min-chars` runes
is skipped; short stubs would otherwise produce noise.

The rule walks `RootFS` when the project root is known. Otherwise it
falls back to the file's own directory. An `include` list narrows the
scan to matching paths. An `exclude` entry takes precedence.

## Performance

Each checked file reads every other Markdown file in scope
(`.md` and `.markdown`). A project
with *N* Markdown files performs *O(N²)* reads. Small and medium
corpora stay fast. For large corpora add an `exclude` entry for
generated or vendored directories.

## Config

```yaml
rules:
  duplicated-content:
    include:
      - "docs/**"
    exclude:
      - "docs/generated/**"
    min-chars: 200
```

Disable:

```yaml
rules:
  duplicated-content: false
```

## Examples

### Good

<?include
file: good/simple.md
wrap: markdown
?>

```markdown
# Simple Fixture

One short fixture sits alone in its folder and exists to exercise
the duplicate detector. Every other rule stays quiet because the
text is simple and brief. The paragraph holds enough characters to
pass two hundred runes after normalization. Each sentence is plain
and ends early. No other file here repeats this wording.
```

<?/include?>

### Bad -- duplicated paragraph

<?include
file: bad/duplicate.md
wrap: markdown
?>

```markdown
# Duplicate Fixture

A distinctive paragraph appears in this file and in a sibling
fixture, so MDS037 must flag the match and point at the other
location. The wording stays above the default two-hundred character
threshold after normalization. It stays unique relative to the
other rule fixtures so nothing matches by accident across the test
suite.
```

<?/include?>

### Bad -- duplicated source

<?include
file: bad/ref/source.md
wrap: markdown
?>

```markdown
# Source Fixture

A distinctive paragraph appears in this file and in a sibling
fixture, so MDS037 must flag the match and point at the other
location. The wording stays above the default two-hundred character
threshold after normalization. It stays unique relative to the
other rule fixtures so nothing matches by accident across the test
suite.
```

<?/include?>

## Diagnostics

| Condition         | Message                                  |
|-------------------|------------------------------------------|
| paragraph repeats | paragraph duplicated in {other}:{line}   |
| invalid glob      | duplicated-content: invalid glob pattern |
