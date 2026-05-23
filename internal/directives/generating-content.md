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
sort: id
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
file. The included body is regenerated whenever the
source changes:

```markdown
<?include
file: docs/development/index.md
strip-frontmatter: "true"
heading-level: "absolute"
?>
<?/include?>
```

`strip-frontmatter` drops the leading YAML block.
`heading-level: absolute` keeps source heading
levels; `relative` rewrites them under the parent
heading at the include site.

See the full
[generating-content guide](../../docs/guides/directives/generating-content.md)
for sort orders, gitignore filtering, format
placeholders, and schema-mode include semantics.
