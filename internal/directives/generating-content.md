---
title: catalog and include
summary: >-
  catalog builds file indexes; include embeds
  another file. mdsmith fix regenerates the body.
---
# `<?catalog?>` and `<?include?>`

These directives generate content inside Markdown.
`mdsmith fix` regenerates the body; `mdsmith check`
flags drift.

## `<?catalog?>`

Builds a list or table of files matched by globs.
Front-matter fields fill `{placeholder}` tokens in
the row template:

```markdown
<?catalog
glob:
  - "plan/*.md"
  - "!plan/proto.md"
sort: numeric:id
header: |
  | ID | Title |
  |----|-------|
row: "| {id} | [{title}]({filename}) |"
?>
<?/catalog?>
```

Without a `row` template, the directive emits
`- [<basename>](<path>)` for each match. Prefix a
glob with `!` to exclude.

## `<?include?>`

Splices another file's content into the current
file. `mdsmith fix` regenerates the body. `mdsmith
check` reports drift on out-of-date copies:

```markdown
<?include
file: docs/development/index.md
strip-frontmatter: "true"
heading-level: "absolute"
?>
<?/include?>
```

`strip-frontmatter` drops the leading YAML block.
`heading-level: "absolute"` shifts included headings
so they nest under the include site's parent
heading; omit it to keep source heading levels
unchanged. No other value is accepted.

See the full
[generating-content guide](../../docs/guides/directives/generating-content.md)
for sort orders, gitignore filtering, format
placeholders, and schema-mode include semantics.
