package piparser_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"github.com/jeduden/mdsmith/internal/piparser"
)

// newPIParser builds a minimal goldmark parser with only the PI block
// parser registered, so the test exercises BlockParserPrioritized directly.
func newPIParser() parser.Parser {
	return parser.NewParser(parser.WithBlockParsers(piparser.BlockParserPrioritized()))
}

// findPINodes returns every ProcessingInstruction node in the tree.
func findPINodes(root ast.Node) []*piparser.ProcessingInstruction {
	var nodes []*piparser.ProcessingInstruction
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if node, ok := n.(*piparser.ProcessingInstruction); ok {
			nodes = append(nodes, node)
		}
		return ast.WalkContinue, nil
	})
	return nodes
}

func TestBlockParserPrioritized(t *testing.T) {
	pv := piparser.BlockParserPrioritized()
	require.NotNil(t, pv.Value, "forwarded PI block parser must not be nil")
	assert.Equal(t, 850, pv.Priority, "PI parser priority must match the canonical value")
}

func TestPIBlockParser_ParsesNode(t *testing.T) {
	src := []byte("<?foo?>\n")
	root := newPIParser().Parse(text.NewReader(src))

	pis := findPINodes(root)
	require.Len(t, pis, 1)
	assert.Equal(t, "foo", pis[0].Name)
	assert.Equal(t, piparser.KindProcessingInstruction, pis[0].Kind())
}

func TestPIBlockParser_MultiplePIs(t *testing.T) {
	src := []byte("<?foo?>\n\n<?bar\nbaz\n?>\n")
	root := newPIParser().Parse(text.NewReader(src))

	pis := findPINodes(root)
	require.Len(t, pis, 2)
	assert.Equal(t, "foo", pis[0].Name)
	assert.Equal(t, "bar", pis[1].Name)
}
