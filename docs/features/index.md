---
title: "Why mdsmith"
summary: >-
  The mdsmith feature overview shared by the repository README and
  the website. Each capability links to a fuller page with rules and
  examples.
---
# Why mdsmith

mdsmith is one rule engine behind every surface: the CLI, the LSP
server, the VS Code extension, and the Claude Code plugin all run
the same checks. This page is the shared overview. The README
includes it; the website renders it and links each card to a
fuller page.

**[Auto-fix Markdown formatting](auto-fix.md).**
`mdsmith fix` rewrites whitespace, headings, code fences, bare
URLs, list indentation, and table alignment in place. It loops
until edits stabilize. `mdsmith check` is the read-only CI
sibling.

**[Live diagnostics wherever you write](live-diagnostics.md).**
`mdsmith lsp` emits diagnostics, quick-fixes, and navigation. Any
LSP-aware editor can consume it. The VS Code extension and the
Claude Code plugin surface the same data.

**[Rename without breaking links](rename.md).**
Rename a heading and every workspace anchor link to it is
rewritten in one atomic edit. Link-ref labels rename with their
uses. A colliding slug fails loudly instead of breaking links.

**[See the dependency graph](dependency-graph.md).**
`mdsmith deps` lists what a file pulls in, or what depends on it.
The LSP call-hierarchy walks the same `<?include?>`, `<?catalog?>`,
`<?build?>`, and link graph in your editor.

**[Cross-file integrity](cross-file-integrity.md).**
Built-in rules flag broken links and missing anchors, enforce
per-file section schemas, and keep Markdown in the right folders.
Schemas can be inline on a file kind or shared via `proto.md`
files.

**[Guardrails for AI-generated docs](ai-guardrails.md).**
Cap file, section, and token-budget size. Enforce reading grade
and sentence count. Flag verbatim copy-paste across files.

**[Self-maintaining sections](self-maintaining-sections.md).**
On `mdsmith fix`, `<?toc?>` rebuilds a heading TOC, `<?catalog?>`
generates an index from front matter, and `<?include?>` splices
in another file. A Git merge driver resolves conflicts in those
blocks.

**[Gate releases on doc status](release-gating.md).**
`mdsmith list query` selects files by a CUE expression on front
matter. `mdsmith metrics rank` ranks files by any shared metric.
Both pipe straight into a release script.

**[Fast on every run](performance.md).**
A single static Go binary with no runtime to boot. The workspace
walk runs in parallel and embeds are linted once, so CI and
editor feedback stay instant.

**[Quality you can verify](quality.md).**
The build, Go Report Card, and coverage badges at the top of the
README report live project health. mdsmith lints its own docs
with the rules it ships, and a coverage gate blocks merges that
drop below the line.

**[File kinds and schemas](file-kinds-schemas.md).**
Tag each file with a kind, then validate its headings and front
matter against a schema declared inline on the kind or shared via
a `proto.md` template. A whole directory obeys one contract.

**[Conventions and flavors](markdown-conventions.md).**
Pin a convention to get a curated rule preset and a target
renderer flavor in one switch. `MDS034` flags syntax the flavor
will not render. A placeholder vocabulary spares template tokens.

**[Build artifacts in sync](build-artifacts.md).**
`<?build?>` declares an artifact and a recipe. `mdsmith fix`
keeps the section body in sync with the recipe output. `MDS040`
shell-safety-checks the recipe without running it.

**[Git-native, conflict-free](git-native.md).**
A merge driver auto-resolves conflicts inside generated blocks.
A pre-merge-commit hook re-runs `mdsmith fix` and re-stages the
result, so generated content never blocks a merge.

**[Config you can explain](config-transparency.md).**
Config layers deep-merge rule by rule: defaults, convention,
kinds, then overrides. `--explain` and `mdsmith kinds resolve`
show which layer set each effective value.

**[Editors and agents](editor-agent-integration.md).**
A bundled VS Code extension and Claude Code plugins drive the
same `mdsmith lsp` server, so diagnostics, fix-on-save, and
navigation reach your editor and your coding agent unchanged.

**[Installs everywhere](install-everywhere.md).**
One version-stamped Go binary ships through go install, npm,
pip, uvx, mise, asdf, and GitHub Releases. No postinstall
network call, so locked-down CI installs offline.
