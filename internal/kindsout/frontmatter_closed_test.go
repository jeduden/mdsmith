package kindsout

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
)

// closedKindsForTest builds an extends chain whose parent opens its
// front matter and whose child says nothing about it, so the
// effective `frontmatter-closed:` is inherited.
func closedKindsForTest(childSets any) map[string]config.KindBody {
	child := map[string]any{"frontmatter": map[string]any{"status": "string"}}
	if childSets != nil {
		child["frontmatter-closed"] = childSets
	}
	return map[string]config.KindBody{
		"base": {Schema: config.InlineSchema(map[string]any{
			"frontmatter-closed": false,
			"frontmatter":        map[string]any{"id": "string"},
		})},
		"child": {Extends: "base", Schema: config.InlineSchema(child)},
	}
}

// `frontmatter-closed:` decides whether an undeclared key is an
// error, so the extends audit reports its effective value and the
// kind that set it, like every front-matter leaf.
func TestMakeBodyJSON_FrontmatterClosedProvenance(t *testing.T) {
	kinds := closedKindsForTest(nil)
	out := MakeBodyJSON("child", kinds["child"], kinds)
	require.NotNil(t, out.EffectiveFrontmatterClosed)
	assert.Equal(t, FrontmatterClosedLeafJSON{Value: false, Source: "base"},
		*out.EffectiveFrontmatterClosed, "inherited from the parent")

	kinds = closedKindsForTest(true)
	out = MakeBodyJSON("child", kinds["child"], kinds)
	require.NotNil(t, out.EffectiveFrontmatterClosed)
	assert.Equal(t, FrontmatterClosedLeafJSON{Value: true, Source: "child"},
		*out.EffectiveFrontmatterClosed, "the child's explicit value wins")

	kinds = extendsKindsForTest()
	out = MakeBodyJSON("child", kinds["child"], kinds)
	assert.Nil(t, out.EffectiveFrontmatterClosed,
		"no kind in the chain sets it: nothing to attribute")
}

func TestWriteBodyText_FrontmatterClosedLeaf(t *testing.T) {
	kinds := closedKindsForTest(nil)
	var buf bytes.Buffer
	require.NoError(t, WriteBodyText(&buf, "child", kinds["child"], kinds))
	assert.Contains(t, buf.String(),
		"  effective-frontmatter-closed: false  # from base\n")
}

// The leaf line's write error surfaces like every other line's.
func TestWriteBodyText_FrontmatterClosedLeafWriteError(t *testing.T) {
	kinds := closedKindsForTest(nil)
	var buf bytes.Buffer
	require.NoError(t, WriteBodyText(&buf, "child", kinds["child"], kinds))
	lines := strings.Count(buf.String(), "\n")
	w := &failingWriter{err: errors.New("disk full"), after: lines - 1}
	err := WriteBodyText(w, "child", kinds["child"], kinds)
	assert.ErrorContains(t, err, "disk full")
}

// A chain that cannot resolve (a cycle) has no effective value.
func TestEffectiveFrontmatterClosed_ResolverErrorReturnsNil(t *testing.T) {
	kinds := map[string]config.KindBody{
		"a": {Extends: "b", Schema: config.InlineSchema(map[string]any{"frontmatter-closed": false})},
		"b": {Extends: "a"},
	}
	assert.Nil(t, effectiveFrontmatterClosed(kinds, "a"))
}
