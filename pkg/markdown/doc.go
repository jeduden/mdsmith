// Package markdown is mdsmith's public Markdown parse/produce surface.
//
// It owns the one goldmark parser configuration in the tree: the
// default CommonMark parsers plus mdsmith's <?...?> processing-
// instruction block, exposed both as the high-level [Parse] (front
// matter split off, body parsed to an AST) and the lower-level
// [ParseContext] / [NewParser] for callers that need the goldmark
// parser.Context. [Splice] is the producer: byte-exact span surgery
// on the original source rather than an AST-to-Markdown re-render, so
// its output does not fight mdsmith's edit-based fixer.
//
// The linter core (internal/lint) and the release tooling
// (internal/release) both consume this package so parsing decisions
// stay consistent across surfaces; internal/lint additionally
// re-exports the symbols here via type aliases and forwards so its
// many callers need not import this package directly. No code in this
// package imports the linter core: it depends only on goldmark and
// the standard library.
//
// As a public package this is a cross-system contract. Its
// compatibility policy lives in
// docs/development/markdown-library.md.
package markdown
