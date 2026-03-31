---
title: Coming from Hugo
summary: >-
  Key differences between Hugo templates and mdsmith
  directives for users familiar with Hugo.
---
# Coming from Hugo

If you are familiar with Hugo templates, this guide
covers the key differences in mdsmith.

## Placeholder syntax

Use `{title}` not `{{ .Title }}`. Field names are
case-sensitive and match front matter keys exactly.
There are no Go templates in user-facing syntax.

If you use the wrong case (e.g. `{Title}` when front
matter has `title`), mdsmith emits a "did you mean?"
hint.

## Schema is not a rendering template

The `schema` config key defines a validation
contract. It checks that documents have the required
headings and front matter fields. It does not render
output like a Hugo layout.

See
[Enforcing Document Structure](enforcing-structure.md)
for how schemas work.

## Generated content is committed

Content between directive markers is committed to
git. It is not gitignored. Run `mdsmith fix` to
regenerate, then commit the result.

This is different from Hugo, where templates render
at build time and output is typically gitignored.

## Schema composition

Schema files compose via `<?include?>`, which splices
headings from fragment files. This is not Hugo's
`partial` or `block` system.

See
[Schema composition](enforcing-structure.md#schema-composition)
for details.

## No nesting in normal files

Directives are flat. There is no equivalent of Hugo's
nested `block`/`define`. Same-type nesting is
prohibited.

## CUE paths, not Go template syntax

`{field}` uses CUE path semantics for nested front
matter access, not Go template dot notation.

### Where syntax is the same

Top-level field access looks identical:

| Hugo template          | mdsmith placeholder | Result      |
|------------------------|---------------------|-------------|
| `{{ .Title }}`         | `{title}`           | field value |
| `{{ .Params.status }}` | `{status}`          | field value |

### Where syntax differs

Nested access uses CUE dot paths instead of Go
template chaining:

| Hugo template              | mdsmith placeholder | Front matter           |
|----------------------------|---------------------|------------------------|
| `{{ .author.name }}`       | `{author.name}`     | `author: {name: "Jo"}` |
| `{{ index .tags 0 }}`      | `{tags.0}`          | `tags: ["go", "md"]`   |
| `{{ .Params.custom_key }}` | `{custom_key}`      | `custom_key: "value"`  |

Hugo uses `.Params` for custom front matter and
capitalizes standard keys (`.Title`). mdsmith uses
exact front matter key names — no `.Params` wrapper,
no capitalization.

## Directive params are YAML strings

Top-level directive parameters are parsed as strings
(lists of strings are also allowed). Structured
blocks like `columns:` in `<?catalog?>` accept typed
values (for example numbers) as defined by that
directive; using non-string scalars where only strings
are expected produces a diagnostic.
