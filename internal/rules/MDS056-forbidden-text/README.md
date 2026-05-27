---
id: MDS056
name: forbidden-text
status: ready
description: Paragraphs must not contain any configured substring.
category: prose
nature: content
maintainability: null
markdownlint: []
rumdl: []
mado: []
panache: []
---
# MDS056: forbidden-text

Paragraphs must not contain any configured substring.

For each paragraph in the document, the rule scans the plain text for
every entry in `contains:` and emits one diagnostic per match,
anchored at the paragraph's start line. Tables (paragraphs whose first
line starts with `|`) are skipped. Empty substrings are ignored.

Under a schema scope override (plan 146), the per-scope filter keeps
only diagnostics inside the scope's range. The same code applies to a
single section.

## Settings

| Setting    | Type | Default | Description                                                     |
| ---------- | ---- | ------- | --------------------------------------------------------------- |
| `contains` | list | `[]`    | Substrings forbidden in any paragraph. Match is case-sensitive. |

## Config

Document-wide:

```yaml
rules:
  forbidden-text:
    contains: ["should", "may", "might"]
```

Per-section (via plan 146 schema scope):

```yaml
kinds:
  runbook:
    schema:
      sections:
        - heading: "Diagnosis"
          rules:
            forbidden-text:
              contains: ["should", "may"]
```

Disable:

```yaml
rules:
  forbidden-text: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

The platform must investigate the failure before acting.

Operators escalate the alert when the timer expires.
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

We should investigate the failure before acting.
```

<?/include?>

## Meta-Information

- **ID**: MDS056
- **Name**: `forbidden-text`
- **Status**: ready
- **Default**: disabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: prose
