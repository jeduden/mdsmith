---
title: Guides
summary: >-
  User guides for mdsmith directives, structure
  enforcement, and migration.
---
# Guides

<?catalog
glob:
  - "directives/*.md"
  - "metrics-tradeoffs.md"
sort: title
header: |
  | Guide | Description |
  |-------|-------------|
row: "| [{title}]({filename}) | {summary} |"
?>
| Guide                                                                               | Description                                                                                             |
|-------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------|
| [Choosing Readability, Conciseness, and Token Budget Metrics](metrics-tradeoffs.md) | Trade-offs and threshold guidance for readability, structure, length, and token budgets.                |
| [Coming from Hugo](directives/hugo-migration.md)                                    | Key differences between Hugo templates and mdsmith directives for users familiar with Hugo.             |
| [Enforcing Document Structure with Schemas](directives/enforcing-structure.md)      | How to use schemas, require, and allow-empty-section to validate headings, front matter, and filenames. |
| [Generating Content with Directives](directives/generating-content.md)              | How to use catalog and include directives to generate and embed content in Markdown files.              |
<?/catalog?>
