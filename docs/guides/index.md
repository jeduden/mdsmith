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
| ----------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Build directive](directives/build.md)                                              | How to use the build directive to declare artifact outputs, keep generated bodies in sync, and configure user-declared recipes.                                                                                                                                                       |
| [Choosing Readability, Conciseness, and Token Budget Metrics](metrics-tradeoffs.md) | Trade-offs and threshold guidance for readability, structure, length, and token budgets.                                                                                                                                                                                              |
| [Coexist with Prettier](coexist-with-prettier.md)                                   | Run mdsmith alongside Prettier by ordering `mdsmith fix` before `prettier --write` in the same pre-commit hook.                                                                                                                                                                       |
| [Coexist with Vale and remark](coexist-with-vale-and-remark.md)                     | Vale owns brand voice and prose style; remark owns Markdown AST transformations; mdsmith owns formatting, cross-file integrity, and generated sections. They sit side by side in CI without overlap.                                                                                  |
| [Coming from Hugo](directives/hugo-migration.md)                                    | Key differences between Hugo templates and mdsmith directives for users familiar with Hugo.                                                                                                                                                                                           |
| [Enforcing Document Structure with Schemas](directives/enforcing-structure.md)      | How to use schemas, require, and allow-empty-section to validate headings, front matter, and filenames.                                                                                                                                                                               |
| [Extract Markdown as data](extract-markdown-as-data.md)                             | When a Markdown file's payload is prose, put it in the body under H2 sections — not in YAML frontmatter. `mdsmith extract` projects body structure into a JSON tree the same way it projects frontmatter, so the file stays editable as Markdown.                                     |
| [File Kinds](file-kinds.md)                                                         | How to declare file kinds, assign files to them, and read the merged rule config that results.                                                                                                                                                                                        |
| [Generating Content with Directives](directives/generating-content.md)              | How to use catalog and include directives to generate and embed content in Markdown files.                                                                                                                                                                                            |
| [Installation](install.md)                                                          | Every channel that ships the mdsmith binary, the VS Code extension, or the Claude Code plugin — npm, PyPI, asdf, mise, the GitHub release, the Visual Studio Marketplace plus Open VSX, and the in-repository Claude Code marketplace — and which channel to pick for which workflow. |
| [mdsmith for VS Code](editors/vscode.md)                                            | Install the mdsmith VS Code extension and use its inline diagnostics, quick fixes, fix-on-save, and cross-file navigation — one bundled binary, no extra setup.                                                                                                                       |
| [Migrating from markdownlint](migrate-from-markdownlint.md)                         | Move a project from markdownlint-cli or markdownlint-cli2 to mdsmith — the rule mapping, the config rewrite, and the markdownlint-to-mdsmith rule correspondence.                                                                                                                     |
| [Neovim Integration](editors/neovim.md)                                             | Wire `mdsmith lsp` into Neovim's built-in LSP client so diagnostics, code actions, and navigation work inline with no extra plugin.                                                                                                                                                   |
| [Progressive Disclosure for AI Agents](progressive-disclosure.md)                   | Use `<?catalog?>` with a per-file `summary` front matter field to emit a one-line index of a directory, so AI coding agents read a few thousand tokens of metadata up front and only `Read` the files a task actually touches.                                                        |
| [Schemas](schemas.md)                                                               | Declare a document-structure schema inline on a kind or in a proto.md file, validate headings and front matter, and tighten rule config per section.                                                                                                                                  |
<?/catalog?>
