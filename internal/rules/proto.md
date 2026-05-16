---
id: '=~"^MDS[0-9]{3}$"'
name: 'string & != ""'
status: '"ready" | "not-ready"'
description: 'string & != ""'
nature: '"directive" | "generator" | "content" | "style" | "structure"'
maintainability: '{signal: string & != "", fix: string & != "", "for-diagnostic"?: bool | *false} | null'
category: '"accessibility" | "code" | "directive" | "heading" | "line" | "link" | "list" | "prose" | "structural" | "table" | "whitespace"'
---
# {id}: {name}

<!-- Rule README template. Copy this file, replace placeholders,
     delete sections and comments that don't apply.
     Front matter is required. The catalog directive reads
     id, name, status, description, nature to generate the rules
     table and filtered listings. The `category:` field is validated
     by mdsmith check against the literal CUE union in this file's
     `category:` front matter, which is hand-kept in sync with
     config.ValidCategories.
     Repeat the description verbatim. Use prescriptive voice,
     present tense: "Headings must ..." not "Checks that ...".
     The `nature` key labels the rule's kind. Exactly one of:
       - "directive"  -- implements gensection.Directive
         (MDS019 catalog, MDS021 include, MDS038 toc, MDS039 build).
       - "generator"  -- fixed by introducing or updating a
         generated section authored elsewhere (MDS035 toc-directive).
       - "content"    -- readability, structure, or length checks
         on prose, lists, tables.
       - "style"      -- whitespace, capitalisation, fence/list
         marker choices, blank-line placement.
       - "structure"  -- schema, heading, kind, and cross-file
         structural checks (required structure, single H1, link
         integrity, directory layout). -->

{description}

<!-- Optional: ## Settings
     Include only when rule implements Configurable.
     Type: int, string, list. Description: fragment,
     no period. Delete if not applicable. -->

## ...

<?allow-empty-section?>

## Config

<!-- Show enable, disable, and (if configurable) custom
     settings as separate labeled yaml blocks. -->

```yaml
rules:
  rule-name: true
```

Disable:

```yaml
rules:
  rule-name: false
```

## ...

<?allow-empty-section?>

## Examples

<!-- Use <?include?> directives referencing fixture files
     in the rule's good/ and bad/ directories (or good.md
     and bad.md files). Use wrap: markdown so the fixture
     renders inside a fenced code block.
     Add ### Good and ### Bad subsections.
     Complex rules: multiple subsections labeled
     "### Good -- description" or "### Bad -- description".
     Always use include directives for examples. When the
     included output cannot show the difference (e.g., EOF
     newline in MDS009), add explanatory prose after the
     include. -->

<!-- Optional: ## Diagnostics
     Include when the rule emits more than one distinct
     message. Delete for single-message rules. -->

<!-- Optional: ## Edge Cases
     Include for complex rules. Delete otherwise. -->

## ...

<?allow-empty-section?>

## Meta-Information

- **ID**: {id}
- **Name**: `{name}`
- **Status**: {status}
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: CATEGORY
- **Concept**:
  [NAME](../../../docs/background/concepts/NAME.md)

<!-- Bullets in this order: ID, Name, Status, Default, Fixable,
     Implementation, Category, Concept (if applicable).
     Default may include key settings: "enabled, max: 80".
     Category must match the `category:` front-matter field and one
     of the values in ValidCategories. Pick the narrowest that fits;
     drop any category not in this list.
     Delete Concept bullet if not used. -->

## ...

<?allow-empty-section?>
