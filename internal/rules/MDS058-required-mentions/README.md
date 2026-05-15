---
id: MDS058
name: required-mentions
status: ready
description: Heading-bounded sections must contain every configured substring.
nature: content
maintainability: null
---
# MDS058: required-mentions

Heading-bounded sections must contain every configured substring.

The rule walks every heading in the document. For each heading the
rule joins the plain text of paragraphs in the section's line range.
The range runs from the heading down to the next heading at the same
or shallower level, including nested sub-sections.

Each entry in `mentions:` is checked against the joined text. Missing
mentions emit one diagnostic at the section's heading line. Under a
schema scope override (plan 146), the per-scope filter keeps only
diagnostics inside the scope's range.

## Settings

| Setting    | Type | Default | Description                                                                        |
|------------|------|---------|------------------------------------------------------------------------------------|
| `mentions` | list | `[]`    | Substrings that must each appear somewhere in the section's prose. Case-sensitive. |

## Config

Document-wide:

```yaml
rules:
  required-mentions:
    mentions: ["scope: production"]
```

Per-section (via plan 146 schema scope):

```yaml
kinds:
  runbook:
    schema:
      sections:
        - heading: "Rollback"
          rules:
            required-mentions:
              mentions: ["forward reference"]
```

Disable:

```yaml
rules:
  required-mentions: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

Deploy to production after the smoke test passes.
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

Deploy to staging.
```

<?/include?>

## Meta-Information

- **ID**: MDS058
- **Name**: `required-mentions`
- **Status**: ready
- **Default**: disabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: prose
