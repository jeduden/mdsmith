---
id: '=~"^MDS[0-9]{3}$"'
name: 'string & != ""'
description: 'string & != ""'
---
# {{.id}}: {{.name}}

<!-- Rule README template. Copy this file, replace placeholders,
     delete sections and comments that don't apply.
     Front matter is required. The catalog directive reads
     id, name, description to generate the rules table.
     Repeat the description verbatim. Use prescriptive voice,
     present tense: "Headings must ..." not "Checks that ...". -->

{{.description}}

- **ID**: {{.id}}
- **Name**: `{{.name}}`
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: CATEGORY
- **Archetype**:
  [NAME](../../../archetypes/NAME/)

<!-- Bullets in this order: ID, Name, Default, Fixable,
     Implementation, Category, Archetype (if applicable).
     Default may include key settings: "enabled, max: 80".
     Categories: code, heading, line, link, list, meta,
     whitespace. Delete Archetype bullet if not used. -->

<!-- Optional: ## Settings
     Include only when rule implements Configurable.
     Type: int, string, list. Description: fragment,
     no period. Delete if not applicable. -->

## ...

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

## Examples

<!-- Use ```markdown fenced blocks. Minimal context.
     Invisible chars: trailing spaces as ···, tabs as →.
     Add ### Good and ### Bad subsections.
     Complex rules: multiple subsections labeled
     "### Good -- description" or "### Bad -- description". -->

<!-- Optional: ## Diagnostics
     Include when the rule emits more than one distinct
     message. Delete for single-message rules. -->

<!-- Optional: ## Edge Cases
     Include for complex rules. Delete otherwise. -->

## ...
