---
id: '=~"^MDS[0-9]{3}$"'
name: 'string & != ""'
status: '"ready" | "not-ready"'
description: 'string & != ""'
nature: '"directive" | "generator" | "content" | "style" | "structure"'
maintainability: '{signal: string & != "", fix: string & != "", "for-diagnostic"?: bool | *false} | null'
markdownlint: '[...{id: =~"^MD[0-9]{3}$", name: string & != "", partial: bool, default: bool}]'
rumdl: '[...{id: =~"^MD[0-9]{3}$", name: string & != "", partial: bool, default: bool}]'
mado: '[...{id: =~"^MD[0-9]{3}$", name: string & != "", partial: bool, default: bool}]'
panache: '[...{id: =~"^[a-z][a-z0-9-]*$", name: string & != "", partial: bool, default: bool}]'
obsidian-linter: '[...{id: =~"^[a-z][a-z0-9-]*$", name: string & != "", partial: bool, default: bool}]'
gomarklint: '[...{id: =~"^[a-z][a-z0-9-]*$", name: string & != "", partial: bool, default: bool}]'
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
     The `markdownlint:`, `rumdl:`, `mado:`, `panache:`,
     `obsidian-linter:`, and `gomarklint:` keys each list the
     peer linter's rules
     that this mdsmith rule covers. Each entry has `id:`,
     `name:`, a required `partial: bool` (true when the mdsmith
     rule only partly covers the peer check), and a required
     `default:` (whether the peer linter ships the rule enabled
     by default upstream). These per-rule front-matter blocks are
     the source of truth for the coverage matrix at
     docs/research/markdownlint-coverage/README.md.
     Set the key to `[]` (empty list) for tools that have no
     analog rule; the schema no longer accepts `null` here.
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
- **Category**: {category}
- **markdownlint**: [MDxxx][mdl-mdxxx] (name)
- **rumdl**: [MDxxx][rumdl-mdxxx] (name)
- **mado**: [MDxxx][mado-rules] (name)

[mdl-mdxxx]: https://github.com/DavidAnson/markdownlint/blob/main/doc/mdxxx.md
[rumdl-mdxxx]: https://rumdl.dev/mdxxx/
[mado-rules]: https://github.com/akiomik/mado#supported-rules

<!-- Bullets in this order: ID, Name, Status, Default, Fixable,
     Implementation, Category, then one bullet per peer linter the rule
     covers (markdownlint, rumdl, mado, panache, obsidian-linter,
     gomarklint), and optionally Concept or Guide.
     Default may include key settings: "enabled, max: 80".
     Category must match the `category:` front-matter field and one
     of the values in ValidCategories. Pick the narrowest that fits.

     The peer-linter bullets above and their link-reference definitions
     are GENERATED from the `markdownlint:`/`rumdl:`/`mado:`/`panache:`/
     `obsidian-linter:`/`gomarklint:` front matter. Do not hand-write
     them; edit the front matter and regenerate (the same data feeds
     the coverage matrix):
       MDSMITH_UPDATE_PEER_LINKS=1 go test ./internal/rules \
         -run TestRuleREADMEPeerLinks
     One bullet per peer with a non-empty list, one entry per covered
     rule (nested when more than one), "(partial)" marking a partial
     cover. markdownlint/rumdl/mado link the MDxxx id (mado shares one
     `mado-rules` label -- it has no per-rule docs); gomarklint links
     its kebab rule id through one shared `gomarklint-rules` label (a
     single Rules page, and the bare name would collide with a shortcut
     peer's label); panache and
     obsidian-linter use a bare `rule-name` shortcut reference. A
     definition past the line limit wraps onto an indented URL line.
     A rule whose peer lists are all empty has no peer bullets.

     Add a Concept bullet when the rule has a dedicated concept page:
       - **Concept**: [NAME](../../../docs/background/concepts/NAME.md)
     Omit the Concept bullet when no concept page applies. -->
