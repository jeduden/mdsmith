// Package lint models a parsed Markdown file: its source, AST, front
// matter, diagnostics, caches, and prose ranges. Every type here is a
// facet of that one subject — File and Diagnostic value types, the
// code-block and prose-range AST projections, front-matter extraction,
// workspace file discovery, and the parse and run caches.
//
// The three standalone utilities that once lived here — gitignore
// matching, byte-limit guards, and processing-instruction parsing — now
// live in internal/gitignore, internal/bytelimit, and internal/piparser
// respectively (plan/224).
package lint
