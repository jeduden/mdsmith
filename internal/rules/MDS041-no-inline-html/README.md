---
id: MDS041
name: no-inline-html
status: ready
description: >-
  Raw HTML tags in Markdown are not allowed; use a
  Markdown construct or an mdsmith directive instead.
---
# MDS041: no-inline-html

Raw HTML tags in Markdown are not allowed; use a
Markdown construct or an mdsmith directive instead.

## Config

Enable with default settings (empty allowlist,
comments permitted):

```yaml
rules:
  no-inline-html: true
```

Enable with specific tags allowed:

```yaml
rules:
  no-inline-html:
    allow: [kbd, sub, sup]
    allow-comments: true
```

Disable:

```yaml
rules:
  no-inline-html: false
```

### Settings

| Setting          | Type            | Default | Description                                            |
|------------------|-----------------|---------|--------------------------------------------------------|
| `allow`          | list of strings | `[]`    | Tag names that are permitted (replaces, not appended). |
| `allow-comments` | bool            | `true`  | Whether HTML comments (`<!-- ... -->`) are allowed.    |

## Examples

### Bad

<?include
file: bad/inline-span.md
wrap: markdown
?>

```markdown
# Title

text <span>x</span> text
```

<?/include?>

### Good

<?include
file: good/allowed-tag.md
wrap: markdown
?>

```markdown
# Title

Press <kbd>Enter</kbd> to continue.
```

<?/include?>

## What is not flagged

- Fenced and indented code blocks containing HTML
- Inline code spans
- Autolinks (`<https://example.com>`)
- mdsmith directives (`<?name ... ?>`) — block forms
  are parsed as `ProcessingInstruction` nodes; inline
  forms are skipped because they start with `<?`
- HTML entities in text (`&amp;`, `&#x2014;`)
- Closing tags (`</div>`) — the matching opening tag
  already produced a diagnostic

## Meta-Information

- **ID**: MDS041
- **Name**: `no-inline-html`
- **Status**: ready
- **Default**: disabled (opt-in)
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: meta
