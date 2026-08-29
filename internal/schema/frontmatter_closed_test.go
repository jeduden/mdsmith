package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- parsing ----

func TestParseInline_FrontmatterClosedTrue(t *testing.T) {
	raw := map[string]any{
		"frontmatter":        map[string]any{"id": "string"},
		"frontmatter-closed": true,
	}
	sch, err := ParseInline(raw, "kind prompt")
	require.NoError(t, err)
	require.NotNil(t, sch.FrontmatterClosed)
	assert.True(t, *sch.FrontmatterClosed)
	assert.True(t, sch.FrontmatterIsClosed())
}

func TestParseInline_FrontmatterClosedFalse(t *testing.T) {
	raw := map[string]any{
		"frontmatter":        map[string]any{"id": "string"},
		"frontmatter-closed": false,
	}
	sch, err := ParseInline(raw, "kind prompt")
	require.NoError(t, err)
	require.NotNil(t, sch.FrontmatterClosed)
	assert.False(t, *sch.FrontmatterClosed)
	assert.False(t, sch.FrontmatterIsClosed())
}

func TestParseInline_FrontmatterClosedAbsentDefaultsClosed(t *testing.T) {
	raw := map[string]any{"frontmatter": map[string]any{"id": "string"}}
	sch, err := ParseInline(raw, "kind prompt")
	require.NoError(t, err)
	assert.Nil(t, sch.FrontmatterClosed)
	assert.True(t, sch.FrontmatterIsClosed(),
		"an absent `frontmatter-closed:` keeps the historical closed default")
}

func TestParseInline_FrontmatterClosedRejectsNonBool(t *testing.T) {
	raw := map[string]any{
		"frontmatter":        map[string]any{"id": "string"},
		"frontmatter-closed": "yes",
	}
	_, err := ParseInline(raw, "kind prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema.frontmatter-closed must be a boolean")
}

func TestParseInline_FrontmatterClosedRejectsFrontmatterlessSchema(t *testing.T) {
	raw := map[string]any{
		"frontmatter-closed": true,
		"sections":           []any{map[string]any{"heading": "Overview"}},
	}
	_, err := ParseInline(raw, "kind prompt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema.frontmatter-closed")
	assert.Contains(t, err.Error(), "`frontmatter:`")
}

// ---- FrontmatterCUE ----

func TestFrontmatterCUE_OpenFormDropsClose(t *testing.T) {
	open := false
	sch := &Schema{
		Frontmatter:       map[string]string{"id": "string"},
		FrontmatterClosed: &open,
	}
	assert.NotContains(t, sch.FrontmatterCUE(), "close(",
		"an open front-matter struct must not be wrapped in close()")
}

func TestFrontmatterCUE_ClosedFormKeepsClose(t *testing.T) {
	sch := &Schema{Frontmatter: map[string]string{"id": "string"}}
	assert.Contains(t, sch.FrontmatterCUE(), "close(")
}

// ---- validation ----

func TestValidate_FrontmatterClosedTrueFlagsUndeclaredKey(t *testing.T) {
	closed := true
	sch := &Schema{
		Frontmatter:       map[string]string{"description": "string"},
		FrontmatterClosed: &closed,
		Source:            "kind prompt",
	}
	doc := newDocFile(t, "a.prompt.md",
		"---\ndescription: \"x\"\nundeclared-key: \"opus\"\n---\n# T\n")
	diags := Validate(doc, sch,
		map[string]any{"description": "x", "undeclared-key": "opus"},
		false, makeDiagForTest)
	require.Len(t, diags, 1, "got %v", diagsMessages(diags))
	assert.Contains(t, diags[0].Message, "undeclared-key")
	assert.Contains(t, diags[0].Message, "not declared in schema")
}

func TestValidate_FrontmatterClosedFalseAllowsUndeclaredKey(t *testing.T) {
	open := false
	sch := &Schema{
		Frontmatter:       map[string]string{"description": "string"},
		FrontmatterClosed: &open,
		Source:            "kind prompt",
	}
	doc := newDocFile(t, "a.prompt.md",
		"---\ndescription: \"x\"\nextra: 1\n---\n# T\n")
	diags := Validate(doc, sch,
		map[string]any{"description": "x", "extra": 1},
		false, makeDiagForTest)
	assert.Empty(t, diags, "got %v", diagsMessages(diags))
}

func TestValidate_FrontmatterClosedFalseStillChecksDeclaredKeys(t *testing.T) {
	open := false
	sch := &Schema{
		Frontmatter:       map[string]string{"description": "string"},
		FrontmatterClosed: &open,
		Source:            "kind prompt",
	}
	doc := newDocFile(t, "a.prompt.md",
		"---\ndescription: 7\n---\n# T\n")
	diags := Validate(doc, sch,
		map[string]any{"description": 7}, false, makeDiagForTest)
	require.Len(t, diags, 1, "got %v", diagsMessages(diags))
	assert.Contains(t, diags[0].Message, "description")
}

// ---- composition across kinds ----

func TestCompose_FrontmatterClosedAcceptsKeyFromEitherKind(t *testing.T) {
	closed := true
	a := &Schema{
		Frontmatter:       map[string]string{"description": "string"},
		FrontmatterClosed: &closed,
		Source:            "kind a",
	}
	b := &Schema{
		Frontmatter:       map[string]string{"model": "string"},
		FrontmatterClosed: &closed,
		Source:            "kind b",
	}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.True(t, out.FrontmatterIsClosed())

	doc := newDocFile(t, "a.prompt.md",
		"---\ndescription: \"x\"\nmodel: \"opus\"\n---\n# T\n")
	diags := Validate(doc, out,
		map[string]any{"description": "x", "model": "opus"},
		false, makeDiagForTest)
	assert.Empty(t, diags,
		"a key declared by either composed kind is allowed; got %v",
		diagsMessages(diags))
}

func TestCompose_FrontmatterClosedStricterWins(t *testing.T) {
	open := false
	a := &Schema{
		Frontmatter:       map[string]string{"description": "string"},
		FrontmatterClosed: &open,
		Source:            "kind a",
	}
	b := &Schema{
		Frontmatter: map[string]string{"model": "string"},
		Source:      "kind b",
	}
	out, err := Compose(a, b)
	require.NoError(t, err)
	assert.True(t, out.FrontmatterIsClosed(),
		"one source leaving front matter closed keeps the composite closed")
}

func TestCompose_FrontmatterOpenWhenEverySourceOpens(t *testing.T) {
	open := false
	a := &Schema{
		Frontmatter:       map[string]string{"description": "string"},
		FrontmatterClosed: &open,
		Source:            "kind a",
	}
	b := &Schema{
		Frontmatter:       map[string]string{"model": "string"},
		FrontmatterClosed: &open,
		Source:            "kind b",
	}
	out, err := Compose(a, b)
	require.NoError(t, err)
	assert.False(t, out.FrontmatterIsClosed())
}

// ---- inheritance ----

func TestExtend_FrontmatterClosedChildWins(t *testing.T) {
	open := false
	parent := &Schema{
		Frontmatter: map[string]string{"description": "string"},
		Source:      "kind parent",
	}
	child := &Schema{
		Frontmatter:       map[string]string{"model": "string"},
		FrontmatterClosed: &open,
		Source:            "kind child",
	}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.False(t, out.FrontmatterIsClosed(),
		"the child's explicit `frontmatter-closed:` overrides the parent's default")
}

func TestExtend_FrontmatterClosedInheritsParent(t *testing.T) {
	open := false
	parent := &Schema{
		Frontmatter:       map[string]string{"description": "string"},
		FrontmatterClosed: &open,
		Source:            "kind parent",
	}
	child := &Schema{
		Frontmatter: map[string]string{"model": "string"},
		Source:      "kind child",
	}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.False(t, out.FrontmatterIsClosed(),
		"a child that says nothing inherits the parent's open front matter")
}

// A source that declares no `frontmatter:` map at all — a
// filename-only or sections-only kind — has no opinion on
// closedness. Counting FrontmatterIsClosed's default for it would
// let such a kind cancel another kind's explicit
// `frontmatter-closed: false` the moment both claim one file.
func TestCompose_FrontmatterlessSourceDoesNotCloseTheComposite(t *testing.T) {
	open := false
	a := &Schema{
		Frontmatter:       map[string]string{"description": "string"},
		FrontmatterClosed: &open,
		Source:            "kind a",
	}
	b := &Schema{
		Filename: []string{"*.prompt.md"},
		Source:   "kind b",
	}
	out, err := Compose(a, b)
	require.NoError(t, err)
	assert.False(t, out.FrontmatterIsClosed())

	doc := newDocFile(t, "a.prompt.md",
		"---\ndescription: \"x\"\nextra: 1\n---\n# T\n")
	diags := Validate(doc, out,
		map[string]any{"description": "x", "extra": 1},
		false, makeDiagForTest)
	assert.Empty(t, diags, "got %v", diagsMessages(diags))
}

// With no source declaring front matter at all the composite keeps
// the historical closed default; it is inert because the composed
// schema then emits no front-matter constraint.
func TestCompose_NoFrontmatterAnywhereKeepsClosedDefault(t *testing.T) {
	a := &Schema{Filename: []string{"*.md"}, Source: "kind a"}
	b := &Schema{Acronyms: &AcronymRule{KnownSafe: []string{"APM"}}, Source: "kind b"}
	out, err := Compose(a, b)
	require.NoError(t, err)
	assert.True(t, out.FrontmatterIsClosed())
}
