---
id: 182
title: Code block convention rules
status: "✅"
model: sonnet
depends-on: []
summary: >-
  Two new rules — MDS065 code-block-style (markdownlint
  MD046, indented vs fenced) and MDS066 commands-show-output
  (markdownlint MD014, leading $ on shell blocks with no
  shown output). Closes the last code-block gap.
---
# Code block convention rules

## Goal

Add the two markdownlint code-block rules mdsmith lacks:
enforce fenced vs indented code blocks, and flag `$`-
prefixed shell commands whose block shows no output. This
closes the MD046 / MD014 gap from the
[linter comparison](../docs/background/markdown-linters.md).

## Background

- MD046 code-block-style: a project should pick fenced or
  indented and stay consistent. mdsmith's MDS010 only
  governs the fence character (backtick vs tilde), not
  fenced-vs-indented, so this is a real gap.
- MD014 commands-show-output: a fenced block where every
  line starts with `$ ` and no output is shown should drop
  the `$ ` so readers can copy-paste.

goldmark distinguishes `*ast.FencedCodeBlock` from
`*ast.CodeBlock` (indented), which is all MD046 needs.
MD014 inspects the lines of a code block.

## Design

- MDS065 code-block-style (provisional), category `style`,
  default-enabled, config `style` ∈
  `consistent | fenced | indented` (default `fenced`).
  Autofix converts indented blocks to fenced when the
  style is `fenced`; the reverse is not auto-applied
  (losing the language tag is lossy).
- MDS066 commands-show-output (provisional), category
  `style`, default-enabled. Flag a block where every
  non-blank line matches `^\$ ` and there is no
  non-prefixed output line. Autofix strips the `$ `.
- Both skip directive bodies.

## Tasks

1. [x] Scaffold `internal/rules/codeblockstyle/` (MDS065).
2. [x] Scaffold `internal/rules/commandsshowoutput/` (MDS066).
3. [x] Implement detection and autofix for each.
4. [x] `rule.Configurable` for MDS065 `style`.
5. [x] Fixture tests under the provisional
   `internal/rules/MDS065-*` and `MDS066-*` directories.
6. [x] Rule READMEs; regenerate the docs catalog and index.
7. [x] Add the MD046 / MD014 rows to the
   [linter comparison](../docs/background/markdown-linters.md)
   and the markdownlint coverage matrix.

## Notes

- MDS065 autofix tags converted blocks `text` (rather than
  emitting an empty info string) so the result satisfies
  MDS011 (fenced-code-language). Users are expected to
  refine the tag.

## Acceptance Criteria

- [x] An indented code block is flagged under
      `style: fenced` and converted to a fenced block.
- [x] A `consistent` setting infers from the first block
      and flags later deviations.
- [x] A `$ cmd`-only block is flagged and the `$ ` is
      stripped by autofix.
- [x] A block that mixes commands and output is not
      flagged.
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
- [x] `mdsmith check .` passes
