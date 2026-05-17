package extract

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A file resolving to two kinds composes both schemas; identical
// headings merge to the same key, distinct headings sit side by
// side in the projected tree.
func TestExtract_ComposedMergedTree(t *testing.T) {
	a := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{litScope("Goal")}}
	b := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{litScope("Status")}}
	composed, err := schema.Compose(a, b)
	require.NoError(t, err)

	f := doc(t, "## Goal\n\ng\n\n## Status\n\ns\n")
	mt := schema.BuildMatchTree(f, composed, nil)
	got, diags := Extract(f, composed, mt)
	require.Empty(t, diags)
	root := got.(map[string]any)
	assert.Contains(t, root, "goal")
	assert.Contains(t, root, "status")
}

// Two kinds that name the same heading do not double-project: the
// composed schema carries one scope, the tree one match, the
// projection one key.
func TestExtract_ComposedIdenticalHeadingNoCollision(t *testing.T) {
	a := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{litScope("Goal")}}
	b := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{litScope("Goal")}}
	composed, err := schema.Compose(a, b)
	require.NoError(t, err)

	f := doc(t, "## Goal\n\ng\n")
	mt := schema.BuildMatchTree(f, composed, nil)
	got, diags := Extract(f, composed, mt)
	require.Empty(t, diags)
	assert.Contains(t, got.(map[string]any), "goal")
}
