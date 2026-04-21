package markdownflavor

import (
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules/markdownflavor/ext"
)

var (
	parserOnce sync.Once
	parserMD   goldmark.Markdown
)

// Parser returns the shared goldmark parser used for dual parsing.
// It enables five built-in goldmark extensions (table, strikethrough,
// task list, footnote, and definition list) plus the heading-ID
// attribute parser — together covering six AST-detected features —
// and the five custom MDS034 extensions that cover superscript,
// subscript, math block, inline math, and abbreviations. The seventh
// feature that the rule tracks from the AST, bare-URL autolinks, is
// detected on the main CommonMark parse (see detectBareURLs).
//
// To keep MDS034 aligned with the rest of mdsmith, the dual parser
// also registers lint.PIBlockParserPrioritized so a
// <?include ... ?> block is treated as a processing-instruction
// node here — just as lint.NewFile does — rather than as an HTML
// block. Without this, a table fixture embedded inside a PI block
// would be flagged by MDS034 but invisible to every other rule.
//
// Linkify is intentionally not enabled: bare-URL autolinks are
// detected separately in detectBareURLs by scanning Text nodes from
// the main CommonMark parse, so adding Linkify would only duplicate
// work without changing the result.
//
// The parser is detection-only: we never render its output. Storing
// it as a package-level singleton avoids rebuilding the parser on
// every rule clone.
func Parser() goldmark.Markdown {
	parserOnce.Do(func() {
		parserMD = goldmark.New(
			goldmark.WithExtensions(
				extension.Table,
				extension.Strikethrough,
				extension.TaskList,
				extension.Footnote,
				extension.DefinitionList,
				ext.Superscript,
				ext.Subscript,
				ext.MathBlock,
				ext.MathInline,
				ext.Abbreviation,
			),
			goldmark.WithParserOptions(
				parser.WithAttribute(),
				parser.WithBlockParsers(
					lint.PIBlockParserPrioritized(),
				),
			),
		)
	})
	return parserMD
}
