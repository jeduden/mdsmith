package cuetemplate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompile_RejectsEmptyExpression keeps the contract clear:
// an empty expression body is a programming error from the
// caller, not a silent no-op.
func TestCompile_RejectsEmptyExpression(t *testing.T) {
	_, err := Compile("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty CUE expression")
}

// TestCompile_RejectsSyntaxError verifies that syntactically
// invalid CUE is caught at compile time rather than at every
// per-file Render.
func TestCompile_RejectsSyntaxError(t *testing.T) {
	_, err := Compile("strings.Join([for x in")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid CUE expression")
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
	assert.Contains(t, err.Error(), "must evaluate to a string")
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

// TestTemplate_Render_NonIdentifierKeySilentlyDropped pins
// the limitation that frontmatter keys outside the CUE
// identifier shape (`^[A-Za-z][A-Za-z0-9_]*$`) cannot be
// referenced by bare name in an expression. Keys that fail
// the check are still emitted in quoted form so they remain
// valid CUE syntax — but a bare reference to them produces
// an unresolved-reference error. Callers should ensure the
// frontmatter keys they need at template scope are
// identifier-shaped.
func TestTemplate_Render_NonIdentifierKeySilentlyDropped(t *testing.T) {
	tpl, err := Compile(`"\(id)"`)
	require.NoError(t, err)
	got, err := tpl.Render(map[string]any{
		"id":                "MDS001",
		"markdownlint-cell": "MD013 line-length",
	})
	require.NoError(t, err)
	assert.Equal(t, "MDS001", got)
}
