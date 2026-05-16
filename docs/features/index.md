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
