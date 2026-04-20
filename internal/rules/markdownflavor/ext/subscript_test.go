package ext

import (
	"testing"

	"github.com/stretchr/testify/assert"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/extension"
)

func TestSubscriptParsesSingleTilde(t *testing.T) {
	doc := parseWith(t, "H~2~O is water.\n", Subscript)
	assert.NotNil(t, walkFindKind(doc, KindSubscript),
		"expected Subscript node for H~2~O")
}

// When both the built-in strikethrough extension and our subscript
// extension are enabled, `~x~` must be subscript (not strikethrough)
// and `~~x~~` must remain strikethrough. The subscript parser is
// registered with a higher priority (smaller number) so it gets the
// first chance at each `~` run.
func TestSubscriptCoexistsWithStrikethrough(t *testing.T) {
	doc := parseWith(t, "H~2~O and ~~old~~ text.\n", Subscript, extension.Strikethrough)
	assert.NotNil(t, walkFindKind(doc, KindSubscript),
		"single-tilde span must become Subscript")
	assert.NotNil(t, walkFindKind(doc, extast.KindStrikethrough),
		"double-tilde span must still become Strikethrough")
}

func TestSubscriptDoubleTildeIsNotSubscript(t *testing.T) {
	doc := parseWith(t, "a~~b~~c\n", Subscript)
	assert.Nil(t, walkFindKind(doc, KindSubscript),
		"`~~...~~` must not match subscript")
}

func TestSubscriptUnbalancedTilde(t *testing.T) {
	doc := parseWith(t, "a~b c\n", Subscript)
	assert.Nil(t, walkFindKind(doc, KindSubscript))
}

func TestSubscriptContent(t *testing.T) {
	src := "H~2~O\n"
	doc := parseWith(t, src, Subscript)
	node := walkFindKind(doc, KindSubscript)
	if assert.NotNil(t, node) {
		assert.Equal(t, "2", string(node.Text([]byte(src))))
	}
}

func TestSubscriptInsideCodeIgnored(t *testing.T) {
	doc := parseWith(t, "see `H~2~O` here.\n", Subscript)
	assert.Nil(t, walkFindKind(doc, KindSubscript))
}
