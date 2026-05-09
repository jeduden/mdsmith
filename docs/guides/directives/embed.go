// Package directives embeds the directive guide Markdown files so the
// LSP hover provider can serve them from the compiled binary without
// requiring the source tree to be present at runtime.
package directives

import "embed"

// FS exposes the directive guide Markdown files for embedding.
//
//go:embed *.md
var FS embed.FS
