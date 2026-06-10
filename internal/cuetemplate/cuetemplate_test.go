package cuetemplate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestCompile_RejectsEmptyExpression keeps the contract clear:
// an empty expression body is a programming error from the
// caller, not a silent no-op.
func TestCompile_RejectsEmptyExpression(t *testing.T) {
	_, err := Compile("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty cue expression")
}

// TestCompile_RejectsSyntaxError verifies that syntactically
// invalid CUE is caught at compile time rather than at every
// per-file Render.
func TestCompile_RejectsSyntaxError(t *testing.T) {
	_, err := Compile("strings.Join([for x in")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid cue expression")
}

// TestTemplate_Render_ScalarInterpolation exercises the
// happy-path: a string-interpolation expression that names two
// top-level frontmatter fields by their bare keys.
func TestTemplate_Render_ScalarInterpolation(t *testing.T) {
	tpl, err := Compile(`"\(id) - \(name)"`)
	require.NoError(t, err)
	got, err := tpl.Render(map[string]any{
		"id":   "MDS001",
		"name": "line-length",
	})
	require.NoError(t, err)
	assert.Equal(t, "MDS001 - line-length", got)
}

// TestTemplate_Render_ListComprehensionAndJoin is the
// motivating use case: project a list-of-structs field into a
// single comma-joined cell using CUE's list comprehension and
// strings.Join.
func TestTemplate_Render_ListComprehensionAndJoin(t *testing.T) {
	tpl, err := Compile(`strings.Join([for m in markdownlint {"\(m.id) \(m.name)"}], ", ")`)
	require.NoError(t, err)
	got, err := tpl.Render(map[string]any{
		"markdownlint": []any{
			map[string]any{"id": "MD018", "name": "no-missing-space-atx"},
			map[string]any{"id": "MD019", "name": "no-multiple-space-atx"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "MD018 no-missing-space-atx, MD019 no-multiple-space-atx", got)
}

// TestTemplate_Render_ConditionalTernary covers the list-
// comprehension idiom CUE offers in place of a ternary, used
// here to map a bool to a status string.
func TestTemplate_Render_ConditionalTernary(t *testing.T) {
	tpl, err := Compile(`[if def {"on"}, if !def {"off"}][0]`)
	require.NoError(t, err)
	onCase, err := tpl.Render(map[string]any{"def": true})
	require.NoError(t, err)
	assert.Equal(t, "on", onCase)
	offCase, err := tpl.Render(map[string]any{"def": false})
	require.NoError(t, err)
	assert.Equal(t, "off", offCase)
}

// TestTemplate_Render_NonStringResultIsError keeps the
// contract narrow: the template must yield a string. A
// numeric or boolean result surfaces as a Render error so
// downstream writers never silently produce malformed
// markdown.
func TestTemplate_Render_NonStringResultIsError(t *testing.T) {
	tpl, err := Compile("42")
	require.NoError(t, err)
	_, err = tpl.Render(map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "concrete string")
}

// TestTemplate_Render_NonConcreteStringIsError covers a
// row-expr that does not yield a concrete string — here an
// open two-arm string disjunction. A blank cell is silently
// wrong, so Render must reject it. The in-house engine has no
// disjunction in the row subset and rejects the `|` operator
// outright (re-pinned from the former CUE "concrete string"
// wording — the message is the engine's stable contract, not
// CUE's); the contract that a non-concrete result fails loudly
// is unchanged.
func TestTemplate_Render_NonConcreteStringIsError(t *testing.T) {
	tpl, err := Compile(`"foo" | "bar"`)
	require.NoError(t, err)
	_, err = tpl.Render(map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported row operator")
}

// TestTemplate_Render_UnknownFieldIsError verifies that a
// reference to a frontmatter key that is missing from the
// given map surfaces as a Render error rather than rendering
// an empty string. Empty strings would be silently wrong for
// a missing required field.
func TestTemplate_Render_UnknownFieldIsError(t *testing.T) {
	tpl, err := Compile(`"\(missing)"`)
	require.NoError(t, err)
	_, err = tpl.Render(map[string]any{"id": "MDS001"})
	require.Error(t, err)
}

// TestTemplate_Render_StringsJoinAvailable confirms the CUE
// standard library is imported by default so callers can use
// strings.Join, strings.ToLower, etc. without per-template
// boilerplate.
func TestTemplate_Render_StringsJoinAvailable(t *testing.T) {
	tpl, err := Compile(`strings.Join(["a", "b", "c"], "-")`)
	require.NoError(t, err)
	got, err := tpl.Render(map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "a-b-c", got)
}

// TestTemplate_Render_NilFrontMatter accepts a nil map as
// equivalent to an empty map so callers do not have to
// allocate an empty map just to render a literal expression.
func TestTemplate_Render_NilFrontMatter(t *testing.T) {
	tpl, err := Compile(`"literal"`)
	require.NoError(t, err)
	got, err := tpl.Render(nil)
	require.NoError(t, err)
	assert.Equal(t, "literal", got)
}

// TestTemplate_Render_NonIdentifierKeyReachableViaFM pins
// the model for non-identifier-named frontmatter keys: they
// have no top-level alias (CUE syntax forbids one), but the
// `fm` struct exposes every key under its quoted name so
// `fm["my-key"]` selects it.
func TestTemplate_Render_NonIdentifierKeyReachableViaFM(t *testing.T) {
	tpl, err := Compile(`fm["markdownlint-cell"]`)
	require.NoError(t, err)
	got, err := tpl.Render(map[string]any{
		"id":                "MDS001",
		"markdownlint-cell": "MD013 line-length",
	})
	require.NoError(t, err)
	assert.Equal(t, "MD013 line-length", got)
}

// TestTemplate_Render_FMDirectListAccess exercises the
// headline use case for the fm-struct indirection: indexing
// directly into a list-typed frontmatter field without
// going through the top-level alias. The coverage matrix
// uses this shape via `\(fm.markdownlint[0].id)` when a
// rule's peer-mapping has a single entry.
func TestTemplate_Render_FMDirectListAccess(t *testing.T) {
	tpl, err := Compile(`"\(fm.markdownlint[0].id) \(fm.markdownlint[0].name)"`)
	require.NoError(t, err)
	got, err := tpl.Render(map[string]any{
		"markdownlint": []any{
			map[string]any{"id": "MD013", "name": "line-length"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "MD013 line-length", got)
}

// TestTemplate_Render_OutFieldCollisionDoesNotShadow pins
// the buildSource invariant that a frontmatter key whose
// name collides with the synthetic outField is filtered out
// before JSON emission. Without the filter the synthetic
// field would either silently override the data or create a
// self-reference cycle.
func TestTemplate_Render_OutFieldCollisionDoesNotShadow(t *testing.T) {
	tpl, err := Compile(`"\(id)"`)
	require.NoError(t, err)
	got, err := tpl.Render(map[string]any{
		"id":                   "MDS001",
		"mdsmith_template_out": "should-be-ignored",
		"fm":                   "should-also-be-ignored",
	})
	require.NoError(t, err)
	assert.Equal(t, "MDS001", got)
}

// TestTemplate_Render_StringsKeyDoesNotShadowImport keeps
// the `strings` package usable from row-expr even when a
// matched file's front matter happens to declare a key
// named `strings`. The frontmatter value is reachable
// through fm.strings.
func TestTemplate_Render_StringsKeyDoesNotShadowImport(t *testing.T) {
	tpl, err := Compile(`strings.Join([fm.strings, fm.id], "-")`)
	require.NoError(t, err)
	got, err := tpl.Render(map[string]any{
		"id":      "MDS001",
		"strings": "a literal value",
	})
	require.NoError(t, err)
	assert.Equal(t, "a literal value-MDS001", got)
}

// TestTemplate_Render_CUEKeywordKeyIsQuoted ensures a
// frontmatter key that collides with a CUE reserved word is
// emitted as a quoted label, so the generated CUE source
// stays syntactically valid even when the expression does
// not reference that field. Without this guard a key like
// `for` or `import` would produce "missing ','" parse
// errors at every Render call.
func TestTemplate_Render_CUEKeywordKeyIsQuoted(t *testing.T) {
	tpl, err := Compile(`"\(id)"`)
	require.NoError(t, err)
	got, err := tpl.Render(map[string]any{
		"id":     "MDS001",
		"for":    "reserved-1",
		"import": "reserved-2",
	})
	require.NoError(t, err)
	assert.Equal(t, "MDS001", got)
}

// TestTemplate_Render_MarshalErrorReturnsError pins that a
// frontmatter value json.Marshal cannot serialise produces an
// error return from Render rather than a panic. A chan is the
// canonical non-JSON-marshalable Go type.
func TestTemplate_Render_MarshalErrorReturnsError(t *testing.T) {
	tpl, err := Compile(`"literal"`)
	require.NoError(t, err)
	_, err = tpl.Render(map[string]any{"bad": make(chan int)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "encoding frontmatter")
}

// TestTemplate_Render_NonFiniteFrontMatterReturnsError covers
// the trigger reachable from plain Markdown, decoding the YAML
// for real: yaml.v3 turns the scalars `.inf` and `.nan` into
// float64 values json.Marshal rejects, so such front matter
// must surface as a render error, not a panic.
func TestTemplate_Render_NonFiniteFrontMatterReturnsError(t *testing.T) {
	tpl, err := Compile(`"literal"`)
	require.NoError(t, err)
	for _, src := range []string{"weight: .inf\n", "score: .nan\n"} {
		var fm map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(src), &fm), src)
		_, err := tpl.Render(fm)
		require.Error(t, err, src)
		assert.Contains(t, err.Error(), "encoding frontmatter", src)
	}
}
