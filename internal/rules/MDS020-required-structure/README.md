---
id: MDS020
name: required-structure
status: ready
description: Document structure and front matter must match its schema.
---
# MDS020: required-structure

Document structure and front matter must match its
schema.

## Settings

| Setting           | Type      | Default          | Description                                                                   |
|-------------------|-----------|------------------|-------------------------------------------------------------------------------|
| `schema`          | string    | `""`             | Path to a schema file                                                         |
| `archetype`       | string    | `""`             | Archetype name; resolved against `archetype-roots`                            |
| `archetype-roots` | [ ]string | `["archetypes"]` | Directories searched for `<name>.md` schemas; earlier roots shadow later ones |

When both `schema` and `archetype` are empty the rule
skips structure and front matter validation, but still
warns on misplaced `<?require?>` directives. Use
overrides to apply schemas to specific file groups.

`schema` and `archetype` are mutually exclusive; set
only one.

### Archetypes

Archetypes are user-supplied schema files. Place each
schema at `<root>/<name>.md` under a directory listed
in top-level config `archetypes.roots`, or in the
rule's per-block `archetype-roots` setting. A missing
archetype errors with a message naming the roots
searched and the archetypes discovered.

Use the `mdsmith archetypes` CLI to bootstrap and
inspect the archetype directory:

```text
mdsmith archetypes init [dir]
mdsmith archetypes list
mdsmith archetypes show <name>
mdsmith archetypes path <name>
```

Archetype schemas resolve from the project root
filesystem. `<?include?>` fragments inside an
archetype still expand through the OS filesystem
relative to the process working directory. Keep
archetype schemas self-contained. Otherwise, run
`mdsmith` from the project root so includes resolve
correctly.

### Reserved filenames

Discovery skips repository metadata files so they
stay in the archetype directory without leaking into
the archetype namespace:

- `README.md`, `LICENSE.md`, `CONTRIBUTING.md`,
  `CODEOWNERS.md` (case-insensitive)
- Any filename beginning with `_` (scratch) or `.`
  (hidden)

Schema front matter may embed a CUE schema that
validates document front matter:

```yaml
id: '=~"^MDS[0-9]{3}$"'
name: 'string & != ""'
status: '"ready" | "not-ready"'
description: 'string & != ""'
```

### Require directive

Use `<?require?>` in the schema body to declare
constraints on files validated against this schema:

| Field      | Type   | Description                           |
|------------|--------|---------------------------------------|
| `filename` | string | Glob the document basename must match |

```markdown
<?require
filename: "[0-9]*_*.md"
?>
```

### Schema composition

Schema files can use `<?include?>` to share
structure across schemas. Included fragment
headings are spliced into the heading list at
the include position. Fragment front matter is
ignored. `<?require?>` from fragments is merged.

```markdown
# ?

## Goal

<?include
file: common/acceptance-criteria.md
?>
```

Cycle detection prevents circular includes.
Max include depth is 10.

### Optional fields

Append `?` to a schema front matter key to make it
optional. The field may be absent in the document,
but if present it must satisfy the type constraint:

```yaml
name: 'string & != ""'
"description?": string
```

Schema body controls section strictness:

- By default, extra sections are rejected.
- Add a heading with text `...` (for example `## ...`) to
  allow extra headings in that position until the next
  required heading anchor.

## Config

Enable with a schema for rule READMEs:

```yaml
overrides:
  - files: ["internal/rules/*/README.md"]
    rules:
      required-structure:
        schema: internal/rules/proto.md
```

Apply a user-authored archetype to all story files.
The archetype file must live at
`archetypes/story.md` (the default root):

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

Disable:

```yaml
rules:
  required-structure: false
```

## Examples

### Good

<?include
file: good/default.md
wrap: markdown
?>

```markdown
# My Plan

## Goal

Describe the goal here.

## Tasks

List tasks here.
```

<?/include?>

### Bad

<?include
file: bad/default.md
wrap: markdown
?>

```markdown
# My Plan

## Goal

Describe the goal here.
```

<?/include?>

## Diagnostics

| Condition           | Message                                                                       |
|---------------------|-------------------------------------------------------------------------------|
| section missing     | missing required section "## Settings"                                        |
| wrong level         | heading level mismatch for "Settings": expected h2, got h3                    |
| extra section       | unexpected section "## Extra" (expected "## Settings")                        |
| out of order        | section "## Tasks" out of order: expected after "## Goal"                     |
| heading sync        | heading does not match frontmatter: expected "MDS001" (from id), got "MDS002" |
| body sync           | body does not match frontmatter field "description": expected "..."           |
| front matter schema | front matter does not satisfy schema CUE constraints: ...                     |
| filename mismatch   | filename "foo.md" does not match required pattern "[0-9]*_*.md"               |
| misplaced require   | <?require?> is only recognized in schema files; this directive has no effect  |
| schema include loop | cyclic include: a.md -> b.md -> a.md                                          |

## Meta-Information

- **ID**: MDS020
- **Name**: `required-structure`
- **Status**: ready
- **Default**: enabled
- **Fixable**: no
- **Implementation**: [source](./)
- **Guide**:
  [directive guide](../../../docs/guides/directives/enforcing-structure.md)
- **Category**: meta
