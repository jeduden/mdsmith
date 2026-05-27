// Package flavor exposes mdsmith's extended-syntax Markdown parsers,
// the Flavor identity, the per-flavor Feature support model, and the
// public Detect entry point.
//
// pkg/markdown answers "parse and produce" with the canonical
// CommonMark + <?...?> parser. This sub-package adds the GFM and
// pandoc-style extensions (tables, task lists, strikethrough,
// footnotes, definition lists, heading IDs, superscript, subscript,
// math block, inline math, abbreviations, plus GitHub alerts and
// bare URLs scanned from the CommonMark AST) and answers "which
// features does this document use, and which flavors accept them".
package flavor
