// Package directives embeds short, hover-sized stubs that document
// the directive vocabulary served by the LSP hover provider. The full
// user guides live under docs/guides/directives/; these stubs link
// out to them.
package directives

import "embed"

// FS exposes the directive hover stubs for embedding.
//
//go:embed *.md
var FS embed.FS
