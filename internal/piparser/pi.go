// Package piparser extracts processing instructions from Markdown. It
// re-exports the goldmark <?name ... ?> block node and its parser from
// pkg/markdown so the linter's many callers (type switches in schema,
// index, export, linkgraph, gensection, …) share one internal home for
// the node without importing the public package directly.
package piparser

import "github.com/jeduden/mdsmith/pkg/markdown"

// ProcessingInstruction is the custom AST block node for
// <?name ... ?> blocks. It is an alias for the canonical type in
// pkg/markdown so callers keep working without importing the public
// package directly while the node definition lives in exactly one place.
type ProcessingInstruction = markdown.ProcessingInstruction

// KindProcessingInstruction is the ast.NodeKind for
// ProcessingInstruction, re-exported from pkg/markdown so there is a
// single registered kind value in the tree.
var KindProcessingInstruction = markdown.KindProcessingInstruction
