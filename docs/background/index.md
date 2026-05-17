---
title: Concepts
weight: 50
summary: >-
  The mental model behind mdsmith — how flavor, rule,
  convention, and kind relate, how generated sections
  work, the placeholder grammar, and how it compares to
  other Markdown linters.
---
# Concepts

<?catalog
glob:
  - "**/*.md"
  - "!index.md"
sort: path
header: ""
row: "- [{summary}]({filename})"
?>
- [How "flavor" (a property of the renderer), "rule" (a single check), "convention" (a project-wide bundle), and "kind" (a per-file role tag) differ in mdsmith, the cases where they overlap, and how the four concepts compose.](concepts/flavor-rule-convention-kind.md)
- [How generated sections work — markers, directives, and fix behavior.](concepts/generated-section.md)
- [How the placeholder vocabulary lets rules treat template tokens as opaque rather than flagging them as content violations.](concepts/placeholder-grammar.md)
- [How mdsmith compares to other Markdown linters.](markdown-linters.md)
<?/catalog?>
