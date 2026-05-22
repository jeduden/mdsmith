---
title: Reference
weight: 30
summary: >-
  Look up exact CLI commands, config glob and schema
  syntax, the built-in conventions, and the section-schema
  grammar.
---
# Reference

<?catalog
glob:
  - "**/*.md"
  - "!index.md"
sort: path
header: ""
row: "- [{summary}]({filename})"
?>
- [CLI commands, flags, exit codes, and output format.](cli.md)
- [List workspace links that point at a file.](cli/backlinks.md)
- [Lint Markdown files for style issues.](cli/check.md)
- [List a file's dependency-graph edges (includes, links, catalogs, builds).](cli/deps.md)
- [Write a portable, directive-free copy of a Markdown file.](cli/export.md)
- [Emit a schema-conformant Markdown file as a JSON/YAML/msgpack data tree.](cli/extract.md)
- [Auto-fix lint issues in Markdown files in place.](cli/fix.md)
- [Show built-in documentation for rules, metrics, and concept pages.](cli/help.md)
- [Generate a default `.mdsmith.yml` config in the current directory.](cli/init.md)
- [Inspect declared file kinds and resolve effective rule config per file.](cli/kinds.md)
- [Selection-style commands that walk the workspace and emit matches.](cli/list.md)
- [Run a Language Server Protocol server on stdio for editor integrations.](cli/lsp.md)
- [Git merge driver that resolves conflicts inside generated sections.](cli/merge-driver.md)
- [List and rank shared Markdown metrics (file length, token estimate, readability, …).](cli/metrics.md)
- [Install / manage a pre-merge-commit hook that runs `mdsmith fix` after a merge.](cli/pre-merge-commit.md)
- [Select Markdown files by a CUE expression on front matter.](cli/query.md)
- [Rename a heading or link-reference label and rewrite every dependent edit.](cli/rename.md)
- [Print the mdsmith build version and exit.](cli/version.md)
- [Built-in Markdown conventions, the rule presets each one applies, and how user config layers on top via deep-merge.](conventions.md)
- [Glob pattern syntax across mdsmith config, directives, and CLI argument expansion, with the supported exclusion semantics for each surface.](globs.md)
- [Named field-type shortcuts for inline schema frontmatter values — the registered names, the canonical CUE each one resolves to, and example usage.](schema-types.md)
- [Section-schema reference for inline `kinds.<name>.schema:` blocks. Covers the `heading:` discriminator, the `regex:` matcher (a Go RE2 body with `\#(digits)` and `\#(fmvar(...))` helpers), the `repeat: {min, max}` cardinality field, and the matching algorithm. `proto.md` files are parsed into the same shape by the schema package, but MDS020's file-schema check still uses its legacy parser; see the proto.md section below for what is and is not migrated.](section-schema.md)
- [mdsmith collects no telemetry, no usage analytics, no error reports, and no identifiers. The CLI and the LSP server make no outbound network calls at runtime.](telemetry.md)
<?/catalog?>
