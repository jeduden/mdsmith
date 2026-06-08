---
title: Concepts
summary: >-
  Core mdsmith concepts — the engine API, the flavor/rule/
  convention/kind separation, the generated-section model, and
  the placeholder vocabulary.
---
# Concepts

<?catalog
glob:
  - "*.md"
  - "!index.md"
sort: title
header: ""
row: "- [{summary}]({filename})"
?>
- [The public `pkg/mdsmith` engine API — a `Session` that owns workspace, compiled config, and parse caches — and how it mirrors one-to-one as WebAssembly JavaScript bindings, including the open method namespace, the cache contract, and the WASM size budgets and limits.](engine-api.md)
- [How "flavor" (a property of the renderer), "rule" (a single check), "convention" (a project-wide bundle), and "kind" (a per-file role tag) differ in mdsmith, the cases where they overlap, and how the four concepts compose.](flavor-rule-convention-kind.md)
- [How generated sections work — markers, directives, and fix behavior.](generated-section.md)
- [How the placeholder vocabulary lets rules treat template tokens as opaque rather than flagging them as content violations.](placeholder-grammar.md)
<?/catalog?>
