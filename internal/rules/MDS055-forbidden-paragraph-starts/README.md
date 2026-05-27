---
id: MDS055
name: forbidden-paragraph-starts
status: ready
description: Paragraphs must not begin with any configured prefix.
category: prose
nature: content
maintainability: null
markdownlint: []
rumdl: []
mado: []
panache: []
---
# MDS055: forbidden-paragraph-starts

Paragraphs must not begin with any configured prefix.

For each paragraph, the rule compares the leading characters of the
paragraph's plain text against every entry in `starts:`. The first
matching prefix produces one diagnostic at the paragraph's start line.
Empty prefix entries are ignored. Tables (paragraphs whose first line
starts with `|`) are skipped because their pipe-prefixed rows are not
prose.

## Settings

| Setting  | Type | Default | Description                                                                |
| -------- | ---- | ------- | -------------------------------------------------------------------------- |
| `starts` | list | `[]`    | Prefixes forbidden at the start of any paragraph. Match is case-sensitive. |

## Config

Document-wide:

```yaml
rules:
  forbidden-paragraph-starts:
    starts: ["We ", "The team "]
```

Per-section (via plan 146 schema scope):

```yaml
kinds:
  runbook:
    schema:
      sections:
        - heading: "Diagnosis"
          rules:
            forbidden-paragraph-starts:
              starts: ["TL;DR"]
```

Disable:

```yaml
rules:
  forbidden-paragraph-starts: false
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

Operators should triage the alert and escalate as needed.
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

We need to investigate the failure before acting.
```

<?/include?>

## Meta-Information

- **ID**: MDS055
- **Name**: `forbidden-paragraph-starts`
- **Status**: ready
- **Default**: disabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: prose
