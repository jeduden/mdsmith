---
weight: 45
summary: >-
  Two schema keys that tie a document's front
  matter to the rest of its contract:
  `frontmatter-closed:`, which decides whether an
  undeclared front-matter key is an error, and
  `\#(fmvar(name))` interpolation inside
  `filename:` and `path-pattern:` globs, which
  makes a path agree with a front-matter value.
---
# Front-matter agreement

Two schema keys tie a document's front matter to
the rest of its contract. `frontmatter-closed:`
decides whether a key the schema never declared is
an error. `\#(fmvar(name))` interpolation lets a
`filename:` or `path-pattern:` glob require the
path to agree with a field's value.

Both sit alongside the
[section schema](section-schema.md), which covers
the `heading:` grammar the same schema block
carries.

## `frontmatter-closed`

Front matter is **closed by default**. A key absent
from `frontmatter:` reports:

```text
model: got "opus", expected not declared in schema
```

`frontmatter-closed:` states that default
explicitly, or turns it off:

| Value   | Effect                      |
| ------- | --------------------------- |
| absent  | closed (the default)        |
| `true`  | closed, stated explicitly   |
| `false` | open — undeclared keys pass |

Declared keys keep their constraints under either
setting. `false` only stops the "not declared"
report.

The key is valid only on a schema that also
declares a non-empty `frontmatter:` map. Without
one mdsmith emits no front-matter constraint at
all, so the setting would be dead; that pairing
parse-errors. This mirrors the `closed:` /
`sections:` guard in the section schema.

### Example: an APM prompt

APM's `.apm/prompts/*.prompt.md` preserves exactly
five front-matter keys. `apm compile` drops every
other one, but only at compile time — after the
file is written and shipped. Declaring the closure
catches a sixth key while the author is still in
the editor:

```yaml
kinds:
  apm-prompt:
    path-pattern: ".apm/prompts/*.prompt.md"
    schema:
      frontmatter-closed: true
      frontmatter:
        "description?": nonEmpty
        "input?": string
        "allowed-tools?": [...string]
        "model?": nonEmpty
        "argument-hint?": nonEmpty
```

Every key is optional (trailing `?`), so the kind
constrains which keys may appear without demanding
any of them.

### Layering

When two kinds claim one file, their schemas
compose. The composed front matter accepts a key
**either** kind declares, and stays closed unless
every source opens it. One kind's
`frontmatter-closed: false` cannot loosen another
kind's contract.

Under `extends:` the rule is different: the child's
explicit value wins, and a child that says nothing
inherits the parent's. Inheritance is a
relationship the author wrote on purpose;
composition merges kinds that met on one file.

## `fmvar` in path and filename globs

`\#(fmvar(name))` — the same helper the `regex:`
matcher uses for heading text — also works in a
schema's `filename:` globs and in a kind's
`path-pattern:`. Both resolve the reference against
the document's own front matter before matching.

`digits`, the matcher's other helper, is rejected
here: a glob has no capture group to read back.

The resolved value's glob metacharacters are
escaped, so it matches literally. A `name` of `a*b`
matches the directory `a*b` and not `axxb` — the
glob analogue of the regex matcher's
`regexp.QuoteMeta`.

### Example: an APM skill

APM's `.apm/skills/<name>/SKILL.md` requires the
`name` field to equal the directory name. A static
glob cannot express that:

```yaml
kinds:
  apm-skill:
    path-pattern: ".apm/skills/\\#(fmvar(name))/SKILL.md"
```

With `name: code-review` the kind accepts
`.apm/skills/code-review/SKILL.md`. Under
`.apm/skills/reviewer/SKILL.md` it reports the
mismatch and shows the expansion:

```text
path: got ".apm/skills/reviewer/SKILL.md", expected
path matching glob .apm/skills/\#(fmvar(name))/SKILL.md
  (with front matter applied: .apm/skills/code-review/SKILL.md)
```

### Unresolved references

A document with no `name` field at all does not
match some degenerate path. It reports which
reference could not be resolved:

```text
  (`fmvar(name)`: frontmatter value missing)
```

A malformed reference is caught earlier, at config
load. A non-identifier key must be quoted —
`fmvar("my-key")`, not `fmvar(my-key)` — and the
error names the kind and its pattern.

## See also

- [Section schema](section-schema.md) — the
  `heading:` grammar and the rest of the
  schema-level fields.
- [Schema field types](schema-types.md) — the
  named shortcuts (`nonEmpty`, `date`, …) the
  examples above use.
- [File kinds](../guides/file-kinds.md) — how a
  file is assigned to a kind in the first place.
