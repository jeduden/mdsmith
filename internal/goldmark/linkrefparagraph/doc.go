// Package linkrefparagraph is a fork of goldmark's
// linkReferenceParagraphTransformer with one change: the BlockReader
// used to parse link-reference definitions is owned by the transformer
// and re-Reset for every paragraph, instead of allocated fresh per
// paragraph as upstream does.
//
// Upstream (goldmark@v1.8.2): parser/link_ref.go:18 calls
// text.NewBlockReader(reader.Source(), lines) on every paragraph,
// producing one *blockReader allocation per paragraph in every parse.
// Goldmark's own inline pass (parser/parser.go:902) already runs ONE
// shared blockReader for every block via Reset, so the type itself is
// reuse-safe; the link-ref transformer is the lone holdout.
//
// The fork keeps a *text.BlockReader on the transformer struct.
// Transform re-Resets it for every paragraph. The transformer is
// no longer a global singleton — each parser instance gets its own
// transformer via NewTransformer(), which is goroutine-safe under
// mdsmith's parserPool (one parser per goroutine).
//
// Source: github.com/yuin/goldmark@v1.8.2/parser/link_ref.go,
// parser/link.go (parseLinkDestination, linkFindClosureOptions),
// parser/parser.go (astReference). MIT-licensed, see
// UPSTREAM_LICENSE.
package linkrefparagraph
