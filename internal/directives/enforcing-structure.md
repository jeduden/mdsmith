---
title: enforcing structure
summary: >-
  Schema, require, and allow-empty-section make
  document structure a lint-time contract.
---
# `<?require?>` and `<?allow-empty-section?>`

These directives work with the
`required-structure` rule
([MDS020](../../docs/features/cross-file-integrity.md))
to enforce a per-kind document contract.

## `<?require?>`

Constrains the filename of a document that uses a
schema. Only recognized inside a schema file:

```markdown
<?require
filename: "[0-9]*_*.md"
?>
```

Files validated against the schema must have a
basename that matches the glob. Using
`<?require?>` in a normal file emits a
"no effect" warning.

## `<?allow-empty-section?>`

Suppresses the empty-section diagnostic for one
section. Place the marker inside the section you
want to leave empty. See
[MDS030](../../docs/features/cross-file-integrity.md)
for the diagnostic.

```markdown
## Compatibility

<?allow-empty-section?>
```

The marker is local to its file. Adding it to a
schema file does not propagate the suppression to
files that use the schema.

See the full
[enforcing-structure guide](../../docs/guides/directives/enforcing-structure.md)
for schema authoring, optional front matter fields,
and the extra-section wildcard.
