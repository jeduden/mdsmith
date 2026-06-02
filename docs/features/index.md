---
title: "Why mdsmith"
summary: >-
  The mdsmith feature overview shared by the repository README and
  the website. Each capability links to a fuller page with rules and
  examples.
---
# Why mdsmith

mdsmith is a Markdown linter and formatter written in Go. It
checks style, readability, structure, and cross-file integrity,
and auto-fixes what fixes cleanly. Where markdownlint-compatible
linters stop at per-file style, mdsmith adds the cross-file graph,
generated sections, structure schemas, and readability budgets.
Together they keep a whole docs tree consistent as it grows, so
the same Markdown can drive your README, your docs site, and
downstream pipelines.

One rule engine runs everywhere you work: in CI, in your editor
through `mdsmith lsp`, and in your coding agent through a Claude
Code plugin. The check that blocks a merge is the same one you see
as you type, so feedback never depends on which tool you opened.

## Clean, consistent Markdown

Catch style, formatting, and readability problems on every file.
`mdsmith fix` rewrites the ones with a single correct fix;
`mdsmith check` is the read-only gate for CI.

**[Auto-fix Markdown formatting](auto-fix.md).**
`mdsmith fix` rewrites whitespace, headings, code fences, bare
URLs, list indentation, and table alignment in place, looping
until edits stabilize. `mdsmith check` runs the same rules
read-only for CI.

**[Conventions and flavors](markdown-conventions.md).**
Pin one convention to apply a curated rule preset and a target
renderer flavor together. `MDS034` flags syntax the flavor will
not render; a placeholder vocabulary leaves template tokens like
`{name}` alone.

**[Size and readability limits](size-and-readability.md).**
Cap file, section, and token-budget size, enforce a reading grade
and sentence count, and flag verbatim copy-paste between files.
Three rules ship on by default; two are opt-in.

## One engine, every surface

The same engine runs in CI, in your editor, and in your coding
agent, from one fast static binary you can install through any
channel.

**[Live diagnostics wherever you write](live-diagnostics.md).**
`mdsmith lsp` serves diagnostics, quick-fixes, and navigation
(definition, references, symbol search, and a call hierarchy) to
any LSP-aware editor over stdio.

**[Editors and agents](editor-agent-integration.md).**
A bundled VS Code extension and a Claude Code plugin drive that
same server, so diagnostics, fix-on-save, and navigation reach
your editor and your agent with no separate install. The `.vsix`
is republished to Open VSX for Cursor, VSCodium, and Theia.

**[Fast on every run](performance.md).**
One static Go binary, no runtime to start. The workspace walk runs
across all cores, and includes are linted once. A full check of
this repository's ~720 files takes about 1.3 s, roughly 4x faster
than Node markdownlint.

**[Installs everywhere](install-everywhere.md).**
The same version-stamped binary ships through go install, npm,
pip, uvx, Homebrew, mise, asdf, and GitHub Releases. No
postinstall network call, so locked-down CI installs offline.

## A connected docs tree

mdsmith reads the links, includes, and headings that tie your
files together, so a rename or a move never strands a reference.

**[Cross-file integrity](cross-file-integrity.md).**
`MDS027` flags broken links and missing anchors across the
workspace, `MDS020` validates each file against its section
schema, and `MDS033` keeps files in their allowed folders.

**[Rename without breaking links](rename.md).**
Rename a heading and mdsmith rewrites every workspace anchor link
that resolved to its slug in one atomic edit. Link-reference
labels rename with their uses; a colliding slug fails loudly
instead of breaking links.

**[See the dependency graph](dependency-graph.md).**
`mdsmith deps` lists what a file pulls in (includes, catalogs,
build sources, and links), or every file that points at it with
`--incoming`. The editor walks the same graph as a call hierarchy.

**[File kinds and schemas](file-kinds-schemas.md).**
Tag each file with a `kind`, then validate its headings, section
order, and front matter against a schema. Declare the schema
inline on the kind or share it from a `proto.md` template, so a
whole directory obeys one contract.

## Markdown as a single source of truth

Each file stays the single source of truth. mdsmith keeps the
generated parts in sync, and can project the file out as JSON,
YAML, or msgpack.

**[Self-maintaining sections](self-maintaining-sections.md).**
On `mdsmith fix`, `<?toc?>` rebuilds a heading table of contents,
`<?catalog?>` generates an index from front matter, and
`<?include?>` splices in another file. A Git merge driver resolves
conflicts inside those blocks.

**[Build artifacts in sync](build-artifacts.md).**
The `<?build?>` directive declares an artifact and a recipe;
`mdsmith fix` rebuilds the section body from the recipe output so
the doc never quotes a stale file. `MDS040` shell-safety-checks
the recipe without running it.

**[Markdown as a data source](markdown-as-data.md).**
`mdsmith extract` projects a schema-conformant file to a JSON,
YAML, or msgpack tree, and `<?include extract:?>` reads one value
back into another file. `mdsmith export` writes a portable,
directive-free copy that renders on any Markdown tool.

## Built for your pipeline

Release gates, a Git merge driver, transparent config, and a
coverage-gated build make mdsmith safe to wire into a shared
repository.

**[Gate releases on doc status](release-gating.md).**
`mdsmith list query 'status: "✅"' plan/` selects files by a CUE
expression on front matter, and `mdsmith metrics rank` orders
files by any shared metric. Both print plain lines ready to pipe
into a release script.

**[Git-native, conflict-free](git-native.md).**
A Git merge driver re-runs the directive and keeps the regenerated
body when two branches both touch a generated block. A
pre-merge-commit hook re-runs `mdsmith fix` and re-stages the
result, so generated content never blocks a merge.

**[Config you can explain](config-transparency.md).**
Config layers deep-merge rule by rule: defaults, convention,
kinds, then per-glob overrides. `mdsmith check --explain` and
`mdsmith kinds resolve` show which layer set each effective value.

**[Quality you can verify](quality.md).**
The CI, Go Report Card, and Codecov badges report live project
health. mdsmith lints its own docs with the rules it ships, and a
coverage gate blocks any merge that drops below the line.
