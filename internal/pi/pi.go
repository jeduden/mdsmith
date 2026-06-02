// Package pi re-exports the goldmark processing-instruction block node and
// parser from pkg/markdown. It gives the linter's many callers (type
// switches in schema, index, export, linkgraph, gensection, …) a single
// internal home for the <?name ... ?> node so the public package's
// definition lives in exactly one place.
package pi

import "github.com/jeduden/mdsmith/pkg/markdown"

// ProcessingInstruction is the custom AST block node for <?name ... ?>
// blocks. It is an alias for the canonical type in pkg/markdown so callers
// keep working without importing the public package directly while the node
// definition lives in exactly one place.
type ProcessingInstruction = markdown.ProcessingInstruction

// KindProcessingInstruction is the ast.NodeKind for ProcessingInstruction,
// re-exported from pkg/markdown so there is a single registered kind value
// in the tree.
var KindProcessingInstruction = markdown.KindProcessingInstruction
