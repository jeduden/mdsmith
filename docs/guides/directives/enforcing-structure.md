---
title: Enforcing Document Structure with Schemas
summary: >-
  How to use schemas, require, and
  allow-empty-section to validate headings, front
  matter, and filenames.
---
# Enforcing Document Structure with Schemas

mdsmith can validate that documents follow a required
structure — specific headings, front matter fields,
and filename patterns. This is configured through
schema files and the `required-structure` rule
([MDS020](../../../internal/rules/MDS020-required-structure/README.md)).

## When to use schemas

Use a schema when a set of files must follow the same
structure. Common examples:

- Rule READMEs that must all have the same sections
- Plan files that must include Goal, Tasks, and
  Acceptance Criteria
- API docs that must have Parameters and Examples

## Creating a schema

A schema file is a Markdown file whose headings
define the required structure. Front matter keys
define CUE constraints on document front matter.

```markdown
---
id: '=~"^MDS[0-9]{3}$"'
name: 'string & != ""'
status: '"ready" | "not-ready"'
---
# ?

## Goal

## Tasks

## Acceptance Criteria
```

The `# ?` heading matches any top-level heading. Each
`## ...` heading is required in documents validated
against this schema.

## Applying a schema to files

Configure the schema in `.mdsmith.yml` using
overrides:

```yaml
overrides:
  - files: ["internal/rules/*/README.md"]
    rules:
      required-structure:
        schema: internal/rules/proto.md
```

`mdsmith check` reports missing or extra sections,
wrong heading levels, front matter that violates CUE
constraints, and heading/body text that does not match
front matter values.

## Requiring filename patterns

Use `<?require?>` in a schema file to constrain
document filenames:

```markdown
<?require
filename: "[0-9]*_*.md"
?>
```

Documents validated against this schema must have a
basename matching the glob. If they don't, `check`
reports: `filename "foo.md" does not match required
pattern "[0-9]*_*.md"`.

**Schema-only**: `<?require?>` is only recognized in
schema files. Using it in a normal file emits:
`<?require?> is only recognized in schema files; this
directive has no effect`.

For full reference, see
[MDS020 required-structure](../../../internal/rules/MDS020-required-structure/README.md).

## Archetype templates

Instead of authoring `schema: path/to/schema.md` for
every override, collect reusable schemas in an
archetype directory and refer to them by name.

### Bootstrap the directory

```text
mdsmith archetypes init
```

This creates `./archetypes/` by default. Passing a
directory name creates exactly that directory — for
example, `mdsmith archetypes init templates` creates
`./templates/`. The directory gets an `example.md`
schema and a `README.md`. `init` does not touch
`.mdsmith.yml`; add the printed snippet yourself.

### Register roots and select an archetype

```yaml
archetypes:
  roots:
    - archetypes

overrides:
  - files: ["stories/**/*.md"]
    rules:
      required-structure:
        archetype: story
```

`archetypes.roots` is a list of directories searched
in order; earlier roots shadow later ones. When the
key is omitted, the single root `./archetypes` is
used.

### Inspect archetypes

```text
mdsmith archetypes list                # one per line: "<name>\t<path>"
mdsmith archetypes show <name>         # print schema source
mdsmith archetypes path <name>         # print resolved path
```

`archetype` and `schema` are mutually exclusive. For
schema composition across archetype files, use
`<?include?>` with paths relative to the archetype
file's directory.

## Allowing intentional empty sections

Some sections are intentionally left empty (for
example, a Compatibility section that exists for
future use). Use `<?allow-empty-section?>` to suppress
the empty-section diagnostic:

```markdown
## Compatibility

<?allow-empty-section?>

## Notes

This section has real content.
```

Without this marker,
[MDS030](../../../internal/rules/MDS030-empty-section-body/README.md)
reports: `section "## Compatibility" has no meaningful
body content`.

**Does not propagate**: Adding
`<?allow-empty-section?>` to a schema file does not
suppress the diagnostic in documents using that
schema. Each file must add its own marker.

## Schema composition

Schema files can use `<?include?>` to share structure
across schemas. Included fragment headings are spliced
into the heading list at the include position.
Fragment front matter is ignored. `<?require?>`
constraints from fragments are merged.

```markdown
# ?

## Goal

<?include
file: common/acceptance-criteria.md
?>
```

Cycle detection prevents circular includes. Max
include depth is 10.

## What schemas do not enforce

Schemas validate **headings and front matter only**.
They do not enforce:

- Directive presence (a `<?catalog?>` in a schema
  does not require documents to also contain one)
- Body content beyond heading structure
- Formatting rules (handled by other rules)

| Behavior                         | Schema file        | Normal file          |
|----------------------------------|--------------------|----------------------|
| `<?require?>` recognized         | yes                | no (warning emitted) |
| `<?allow-empty-section?>` effect | local to that file | local to that file   |
| `<?include?>` behavior           | splices headings   | embeds content       |

## Optional front matter fields

Append `?` to a schema front matter key to make it
optional. The field may be absent, but if present it
must satisfy the type constraint:

```yaml
name: 'string & != ""'
"description?": string
```

## Allowing extra sections

By default, extra sections are rejected. Add a
heading with text `...` to allow extra headings in
that position:

```markdown
## ...

## Required Section
```
