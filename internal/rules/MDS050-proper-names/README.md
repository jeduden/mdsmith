---
id: MDS050
name: proper-names
status: ready
description: Configured proper names (e.g. JavaScript, GitHub) must appear with their canonical casing.
nature: content
maintainability: null
---
# MDS050: proper-names

Configured proper names (e.g. JavaScript, GitHub) must appear with
their canonical casing.

Teams that document software projects need consistent casing for
product names: `Javascript` vs `JavaScript`, `Github` vs `GitHub`,
`markdown` vs `Markdown`. MDS050 lets you pin a vocabulary of proper
names and reports any occurrence whose casing does not match.

## Settings

| Setting      | Type         | Default | Merge   | Description                                                   |
|--------------|--------------|---------|---------|---------------------------------------------------------------|
| `names`      | list(string) | `[]`    | append  | Canonical spellings of proper names to enforce.               |
| `check-code` | bool         | `false` | replace | Also check inside code spans and fenced/indented code blocks. |
| `check-html` | bool         | `false` | replace | Also check inside raw HTML and HTML blocks.                   |

`names` **appends** across config layers so a kind layer can extend
the project vocabulary without replacing it. `check-code` and
`check-html` replace.

## Config

Enable with a vocabulary:

```yaml
rules:
  proper-names:
    names:
      - JavaScript
      - TypeScript
      - GitHub
      - mdsmith
```

Also check inside code blocks:

```yaml
rules:
  proper-names:
    names:
      - JavaScript
    check-code: true
```

Disable:

```yaml
rules:
  proper-names: false
```

## Detection

For each configured name, the rule scans prose text for
ASCII-case-insensitive matches that start at a word boundary.
Only ASCII letters (`A`–`Z`) are folded; non-ASCII characters
in a configured name are matched byte-for-byte. A match must be
preceded by a non-word character (not in `[A-Za-z0-9_]`) or
occur at the start of the file. Only the **left** boundary is enforced — there is no
right-boundary check. For example, `Javascripts` (wrong case)
matches `JavaScript` at the left boundary; the rule reports and
fixes the `Javascripts` prefix to `JavaScript`, leaving the
trailing `s` unchanged. Correctly-cased prefixes in longer words
(e.g. `JavaScripts`) are not flagged.
When the matched bytes differ from the canonical spelling, a
diagnostic is emitted.

URLs inside link destinations and autolinks are never checked.

## Fix

Each wrong-cased occurrence is replaced in place with the canonical
spelling.

## Examples

### Bad -- wrong casing in prose

<?include
file: bad/prose-wrong-casing.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Wrong Casing

Javascript is a scripting language.
```

<?/include?>

### Bad -- wrong casing in heading

<?include
file: bad/heading-wrong-casing.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Heading

## Github

Some text under the heading.
```

<?/include?>

### Bad -- wrong casing in link text (URL not checked)

<?include
file: bad/link-text-wrong-casing.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Links

[Github](https://github.com) is a code host.
```

<?/include?>

### Good -- correct casing

<?include
file: good/correct-prose.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Correct Casing

Use JavaScript for front-end code and GitHub for version control.

The mdsmith tool lints Markdown files.
```

<?/include?>

### Good -- word-boundary edge case

<?include
file: good/word-boundary.md
wrap: markdown
strip-frontmatter: "true"
?>

```markdown
# Word Boundary

GitHubber contributors are welcome.

The word "GitHubber" contains "GitHub" with correct casing so no
diagnostic is emitted.
```

<?/include?>

## Diagnostics

| Message                         | Meaning                                              |
|---------------------------------|------------------------------------------------------|
| `proper name "X" should be "Y"` | The occurrence `X` does not match canonical name `Y` |

## Meta-Information

- **ID**: MDS050
- **Name**: `proper-names`
- **Status**: ready
- **Default**: disabled, opt-in
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: prose
