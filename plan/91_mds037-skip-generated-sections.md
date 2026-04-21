---
id: 91
title: MDS037 skips paragraphs inside generated sections
status: "🔲"
summary: >-
  Paragraphs inside `<?include?>` and `<?catalog?>`
  directive bodies are copies of content owned by
  another file; MDS037 must not count them as
  cross-file duplicates. Extends plan 50.
---
# MDS037 skips paragraphs inside generated sections

Extends [plan 50](50_redundancy-duplication-detection.md)
(MDS037 duplicated-content).

## Goal

Stop MDS037 from flagging paragraphs produced by the
include or catalog directive. The rule then runs
cleanly on a project that shares prose across agent
instruction files or renders catalog tables from
front matter.

## Context

MDS037 walks paragraphs via `ast.KindParagraph` and
does not look at the surrounding processing
instructions. Two directives break that:

- `<?include file: X ?>` expands prose from `X`
  into the host file. After expansion the host file
  and `X` hold the same paragraph text on disk, and
  MDS037 reports each paragraph as duplicated.
  A single project often points two or three agent
  files at the same shared source; every run fires.
- `<?catalog glob: "..." ?>` renders a table or
  list whose row template pulls from each target's
  front matter (e.g. `{summary}`). When a target
  also states the same summary as a sentence in its
  body, the catalog row duplicates that sentence.
  The catalog block is a table today, so the line
  usually sits inside a table cell, but a row
  template can produce paragraph-shaped output too.

The ranges are easy to find. mdsmith parses the
open and close tags as `*lint.ProcessingInstruction`
siblings. The rule can map those to byte ranges
without re-parsing the file.

## Tasks

1. Walk top-level AST once to build a list of
   generated-section byte ranges. A range starts at
   the opening PI's segment end and closes at the
   closing PI's segment start.
2. In `extractParagraphs`, skip any paragraph whose
   first-line byte offset sits inside one of those
   ranges.
3. Likewise skip the paragraph when the rule walks
   *other* files during corpus indexing — otherwise
   a host file would still match against source text
   the other file only carries because of its own
   generated section.
4. Update the README to document this behavior and
   drop the recommendation to hand-exclude the host
   files.
5. Add tests: include-expanded paragraph in host is
   not flagged; catalog-rendered paragraph in host
   is not flagged; duplicates outside generated
   sections still fire.

## Acceptance Criteria

- [ ] A paragraph in a file's body whose bytes sit
      between `<?include?>` / `<?/include?>` markers
      does not produce an MDS037 diagnostic.
- [ ] Same for `<?catalog?>` / `<?/catalog?>`.
- [ ] A real duplicate outside any generated
      section still fires.
- [ ] `mdsmith check .` stays clean on this repo
      with MDS037 enabled in `.mdsmith.yml`.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
