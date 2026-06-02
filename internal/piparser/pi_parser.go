package piparser

import (
	"github.com/yuin/goldmark/util"

	"github.com/jeduden/mdsmith/pkg/markdown"
)

// BlockParserPrioritized returns the PI parser with its priority
// for registration, forwarded from pkg/markdown. Kept because
// internal/schema registers it directly; the parser logic itself
// lives in pkg/markdown.
func BlockParserPrioritized() util.PrioritizedValue {
	return markdown.PIBlockParserPrioritized()
}
