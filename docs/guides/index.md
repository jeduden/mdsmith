---
title: Guides
weight: 20
summary: >-
  User guides for mdsmith directives, structure
  enforcement, and migration.
---
# Guides

<?catalog
glob:
  - "**/*.md"
  - "!index.md"
sort: title
header: |
  | Guide | Description |
  |-------|-------------|
row: "| [{title}]({filename}) | {summary} |"
?>
| Guide                                                                               | Description                                                                                                                                                                                                                                                                           |
|-------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Build directive](directives/build.md)                                              | How to use the build directive to declare artifact outputs, keep generated bodies in sync, and configure user-declared recipes.                                                                                                                                                       |
| [Choosing Readability, Conciseness, and Token Budget Metrics](metrics-tradeoffs.md) | Trade-offs and threshold guidance for readability, structure, length, and token budgets.                                                                                                                                                                                              |
| [Coming from Hugo](directives/hugo-migration.md)                                    | Key differences between Hugo templates and mdsmith directives for users familiar with Hugo.                                                                                                                                                                                           |
| [Enforcing Document Structure with Schemas](directives/enforcing-structure.md)      | How to use schemas, require, and allow-empty-section to validate headings, front matter, and filenames.                                                                                                                                                                               |
| [File Kinds](file-kinds.md)                                                         | How to declare file kinds, assign files to them, and read the merged rule config that results.                                                                                                                                                                                        |
| [Generating Content with Directives](directives/generating-content.md)              | How to use catalog and include directives to generate and embed content in Markdown files.                                                                                                                                                                                            |
| [Installation](install.md)                                                          | Every channel that ships the mdsmith binary, the VS Code extension, or the Claude Code plugin — npm, PyPI, asdf, mise, the GitHub release, the Visual Studio Marketplace plus Open VSX, and the in-repository Claude Code marketplace — and which channel to pick for which workflow. |
| [Schemas](schemas.md)                                                               | Declare a document-structure schema inline on a kind or in a proto.md file, validate headings and front matter, and tighten rule config per section.                                                                                                                                  |
| [VS Code Integration](editors/vscode.md)                                            | Install the mdsmith VS Code extension, configure how it spawns `mdsmith lsp`, and read diagnostics inline as you edit Markdown files.                                                                                                                                                 |
<?/catalog?>
