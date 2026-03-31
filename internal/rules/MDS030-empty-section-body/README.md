---
id: MDS030
name: empty-section-body
status: ready
description: Section headings must include meaningful body content.
---
# MDS030: empty-section-body

Section headings must include meaningful body content.

- **ID**: MDS030
- **Name**: `empty-section-body`
- **Status**: ready
- **Default**: enabled, `min-level: 2`, `max-level: 6`
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Guide**:
  [directive guide](../../../docs/guides/directives/enforcing-structure.md)
- **Category**: heading

This rule reports sections that only contain whitespace,
comments, or nested headings without any body content.

## Semantics

A section starts at a heading and ends at the next heading
of the same or higher level.

Within that range, meaningful content includes:

- paragraphs
- lists
- tables
- code blocks
- other non-comment HTML blocks

Ignored content includes:

- blank lines
- HTML comments
- mdsmith directive processing instructions (e.g. `<?catalog?>`,
  `<?allow-empty-section?>`)
- nested headings by themselves

Use an explicit allow marker for intentional empty sections:

```markdown
<?allow-empty-section?>
```

## Prior Art

- `remark-lint-no-empty-sections` reports heading sections without content.
- `markdownlint`, Vale, and textlint do not provide a core rule with this
  exact section-empty semantic, so this rule adds explicit heading-scope
  behavior to mdsmith.

## Settings

| Setting        | Type   | Default               | Description                           |
|----------------|--------|-----------------------|---------------------------------------|
| `min-level`    | int    | `2`                   | minimum heading level to check        |
| `max-level`    | int    | `6`                   | maximum heading level to check        |
| `allow-marker` | string | `allow-empty-section` | comment marker that exempts a section |

## Config

```yaml
rules:
  empty-section-body:
    min-level: 2
    max-level: 6
    allow-marker: allow-empty-section
```

Disable:

```yaml
rules:
  empty-section-body: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

````markdown
# Empty Section Body Good

## Overview

This section has enough text to avoid an empty body diagnostic.

## Steps

- Gather inputs.
- Run checks.

## Example

```text
mdsmith check docs/
```
````

<?/include?>

### Good -- intentional empty section

<?include
file: good/allow-marker.md
wrap: markdown
?>

```markdown
# Allow Marker

## Compatibility

<?allow-empty-section?>

## Notes

This section has real content.
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# Empty Section Body Bad

## Placeholder

<!-- TODO -->

## Next

This section has real content.
```

<?/include?>

## Diagnostics

- `section "## Heading" has no meaningful body content; add paragraph, list,
  table, or code content, or add "<?marker?>" for an intentional empty
  section`

## Edge Cases

- Parent headings are considered non-empty if any meaningful content
  (including content that appears in nested subsections) exists within the
  parent section's range before the next same-or-higher-level heading.
- Heading-only nesting with no body content is reported.
- End-of-file sections are checked the same way as middle sections.
