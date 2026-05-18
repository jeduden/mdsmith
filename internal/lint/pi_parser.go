package lint

import (
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/util"

	"github.com/jeduden/mdsmith/pkg/markdown"
)

// NewPIBlockParser returns a block parser for processing
// instructions. It forwards to pkg/markdown so the parser logic lives
// in one place; rules and re-parsing paths that referenced
// lint.NewPIBlockParser keep compiling unchanged.
func NewPIBlockParser() parser.BlockParser {
	return markdown.NewPIBlockParser()
}

// PIBlockParserPrioritized returns the PI parser with its priority
// for registration, forwarded from pkg/markdown.
func PIBlockParserPrioritized() util.PrioritizedValue {
	return markdown.PIBlockParserPrioritized()
}
