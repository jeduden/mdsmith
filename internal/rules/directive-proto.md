---
id: '=~"^MDS[0-9]{3}$"'
name: 'string & != ""'
status: '"ready" | "not-ready"'
description: 'string & != ""'
nature: '"directive"'
maintainability: '{signal: string & != "", fix: string & != "", "for-diagnostic"?: bool | *false} | null'
markdownlint: '[...{id: =~"^MD[0-9]{3}$", name: string & != "", partial?: bool | *false}] | null'
category: '"directive"'
---
# {id}: {name}

<!-- Directive-rule README template. Composes with the
     standard rule README schema and adds the Pattern
     section that demonstrates the user-authored
     anti-pattern this directive fixes.
     The catalog directive reads id, name, status,
     description, nature to generate the rules table and
     filtered listings. Every file using this template
     must declare `nature: directive`. -->

{description}

## ...

<?allow-empty-section?>

## Config

```yaml
rules:
  rule-name: true
```

## ...

<?allow-empty-section?>

## Examples

<!-- Lint-test fixtures: <?include?>s from good/ and bad/
     show inputs the rule itself flags. Distinct from the
     Pattern section below. -->

## ...

<?allow-empty-section?>

## Pattern

<!-- Demonstrate the user-authored anti-pattern this
     directive replaces. The Bad subsection includes
     a sample from pattern/bad/; the Good subsection
     includes the same content rewritten with the
     directive from pattern/good/. The markdown-audit
     skill reads these folders as the canonical
     before/after pair. -->

### Without the directive

<!-- <?include?> a file from pattern/bad/ with
     wrap: markdown so it renders inside a fenced
     block. -->

### With the directive

<!-- <?include?> the matching file from pattern/good/
     with wrap: markdown. -->

## ...

<?allow-empty-section?>

## Meta-Information

- **ID**: {id}
- **Name**: `{name}`
- **Status**: {status}

## ...

<?allow-empty-section?>
