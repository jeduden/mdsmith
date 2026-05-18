package lint

import "github.com/jeduden/mdsmith/pkg/markdown"

// ProcessingInstruction is the custom AST block node for
// <?name ... ?> blocks. It is an alias for the canonical type in
// pkg/markdown so the linter's many callers (type switches in
// schema, index, export, rename, gensection, …) keep working without
// importing the public package directly while the node definition
// lives in exactly one place.
type ProcessingInstruction = markdown.ProcessingInstruction

// KindProcessingInstruction is the ast.NodeKind for
// ProcessingInstruction, re-exported from pkg/markdown so there is a
// single registered kind value in the tree.
var KindProcessingInstruction = markdown.KindProcessingInstruction
