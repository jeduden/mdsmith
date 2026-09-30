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

// Compose returns a nil *Schema when handed no inputs, so
// FrontmatterIsClosed guards its receiver. Drive that branch
// directly: a nil schema declares no front-matter constraint for the
// setting to apply to, so the historical closed default is the safe
// answer.
func TestFrontmatterIsClosed_NilSchema(t *testing.T) {
	var sch *Schema
	assert.True(t, sch.FrontmatterIsClosed())
}

// ---- direct helper tests ----

func TestComposeFrontmatterClosed(t *testing.T) {
	open, closed := false, true
	fm := map[string]string{"a": "string"}
	cases := []struct {
		name string
		in   []*Schema
		want bool
	}{
		{"no declaring source keeps the closed default",
			[]*Schema{{}}, true},
		{"every declaring source opens",
			[]*Schema{{Frontmatter: fm, FrontmatterClosed: &open}, {}}, false},
		{"an unset declaring source votes closed",
			[]*Schema{{Frontmatter: fm, FrontmatterClosed: &open}, {Frontmatter: fm}}, true},
		{"an explicit true wins",
			[]*Schema{
				{Frontmatter: fm, FrontmatterClosed: &closed},
				{Frontmatter: fm, FrontmatterClosed: &open},
			}, true},
	}
	for _, tc := range cases {
		out := &Schema{}
		composeFrontmatterClosed(out, tc.in)
		require.NotNil(t, out.FrontmatterClosed, tc.name)
		assert.Equal(t, tc.want, *out.FrontmatterClosed, tc.name)
	}
}

func TestParseInlineFrontmatterClosed(t *testing.T) {
	sch := &Schema{}
	require.NoError(t, parseInlineFrontmatterClosed(map[string]any{}, sch))
	assert.Nil(t, sch.FrontmatterClosed)

	sch = &Schema{Frontmatter: map[string]string{"a": "string"}}
	require.NoError(t, parseInlineFrontmatterClosed(
		map[string]any{"frontmatter-closed": false}, sch))
	require.NotNil(t, sch.FrontmatterClosed)
	assert.False(t, *sch.FrontmatterClosed)

	assert.ErrorContains(t, parseInlineFrontmatterClosed(
		map[string]any{"frontmatter-closed": "no"}, sch), "must be a boolean")
	assert.ErrorContains(t, parseInlineFrontmatterClosed(
		map[string]any{"frontmatter-closed": true}, &Schema{}),
		"non-empty `frontmatter:` map")
}

// ---- proto.md ----

// A proto.md's front-matter keys are document fields, so a
// `frontmatter-closed:` there would silently become a field
// constraint and leave the front matter closed. ParseFile — the
// proto.md parser behind composition and `extends:` — rejects it and
// says where the setting lives instead.
func TestParseFile_RejectsFrontmatterClosed(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"---\ntitle: string\nfrontmatter-closed: false\n---\n# ?\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.ErrorContains(t, err, "`frontmatter-closed:` is not supported in a proto.md schema")
	assert.ErrorContains(t, err, "proto.md")
}

func TestParseFile_RejectsFrontmatterClosedInExtendsChild(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "base.md", "---\ntitle: string\n---\n# ?\n")
	p := writeFile(t, dir, "proto.md",
		"---\nextends: base.md\nfrontmatter-closed: false\n---\n# ?\n")
	_, err := ParseFile(&FileReader{}, p)
	assert.ErrorContains(t, err, "`frontmatter-closed:` is not supported in a proto.md schema")
}

func TestRejectProtoFrontmatterClosed(t *testing.T) {
	assert.NoError(t, RejectProtoFrontmatterClosed(map[string]any{"title": "string"}))
	assert.NoError(t, RejectProtoFrontmatterClosed(nil))
	assert.Error(t, RejectProtoFrontmatterClosed(
		map[string]any{"frontmatter-closed": true}))
}
