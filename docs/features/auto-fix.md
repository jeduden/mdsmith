---
title: "Auto-fix Markdown formatting"
summary: >-
  `mdsmith fix` rewrites whitespace, headings, code fences, bare URLs,
  list indentation, and table alignment in place, looping up to 10
  passes and stopping when edits stabilize. `mdsmith check` is the
  read-only CI sibling.
icon: wrench
link: "/reference/cli/fix/"
weight: 1
group: "Clean, consistent Markdown"
---
# Auto-fix Markdown formatting

`mdsmith fix` is the write side of the linter. It rewrites
trailing whitespace, heading style, code-fence delimiters, bare
URLs, list indentation, and table alignment directly in the file.

The fixer runs as a fixed-point loop: it applies every fixable
rule, re-parses, and repeats until the document stops changing or
it has run ten passes. Stabilization means one fix never undoes
another.

`mdsmith check` runs the same rules without writing. It is the
read-only sibling for CI, returning a non-zero exit code when any
rule fails so a pipeline can block the merge.

See the [`fix`](../reference/cli/fix.md) and
[`check`](../reference/cli/check.md) command references for flags
and exit codes.
