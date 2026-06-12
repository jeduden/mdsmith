---
id: MDS057
name: required-text-patterns
status: ready
description: Heading-bounded sections must match every configured regex.
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
# MDS057: required-text-patterns

Heading-bounded sections must match every configured regex.

The rule walks every heading in the document. For each heading the
rule joins the plain text of paragraphs in the section's line range.
The range runs from the heading down to the next heading at the same
or shallower level, including nested sub-sections.

Each configured regex is tested against the joined text. Patterns
that do not match emit one diagnostic at the section's heading line.
Under a [schema scope override](../../../docs/reference/section-schema.md),
the per-scope filter keeps only diagnostics inside the scope's range.

`skip-indices` exempts named child indices when the rule runs from a
scope override on a section with `children:`. The setting parses but
has no effect today; it activates once the `children:` schema feature
lands.

## Settings

| Setting    | Type | Default | Description                                                                                                           |
| ---------- | ---- | ------- | --------------------------------------------------------------------------------------------------------------------- |
| `patterns` | list | `[]`    | List of `{pattern, message, skip-indices}` entries. `pattern` must be a valid Go regular expression; others optional. |

## Config

Document-wide:

```yaml
rules:
  required-text-patterns:
    patterns:
      - pattern: "scope: production"
        message: "every section must declare scope"
```

Per-section (via schema scope):

```yaml
kinds:
  runbook:
    schema:
      sections:
        - heading: "Diagnosis"
          rules:
            required-text-patterns:
              patterns:
                - pattern: "forward reference"
                  skip-indices: [-1]
```

Disable:

```yaml
rules:
  required-text-patterns: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# Title

This section meets the expectation: forward reference.
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Title

This section is missing the required token.
```

<?/include?>

## Meta-Information

- **ID**: MDS057
- **Name**: `required-text-patterns`
- **Status**: ready
- **Default**: disabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: prose
