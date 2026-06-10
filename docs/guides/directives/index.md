---
title: Directives
summary: >-
  Guides to mdsmith's content directives — generating content
  with `<?catalog?>` and `<?include?>`, enforcing structure with
  schemas, declaring build artifacts, and moving from Hugo
  templates.
---
# Directives

<?catalog
glob:
  - "*.md"
  - "!index.md"
sort: title
header: ""
row: "- [{title}]({filename}) — {summary}"
?>
- [Build directive](build.md) — How to use the build directive to declare artifact outputs and source inputs, keep generated bodies in sync, and configure user-declared recipes.
- [Coming from Hugo](hugo-migration.md) — Key differences between Hugo templates and mdsmith directives for users familiar with Hugo.
- [Enforcing Document Structure with Schemas](enforcing-structure.md) — How to use schemas, require, and allow-empty-section to validate headings, front matter, and filenames.
- [Generating Content with Directives](generating-content.md) — How to use catalog and include directives to generate and embed content in Markdown files.
<?/catalog?>
