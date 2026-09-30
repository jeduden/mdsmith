package schema

import (
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A front-matter block that failed to parse leaves every
// `\#(fmvar(...))` reference unresolved. The hint must name the parse
// error, as the path-pattern surface does, instead of claiming the
// field is missing: the author wrote it, it just did not parse.
func TestFilenameDiagnostic_ParseErrorNamesYAMLError(t *testing.T) {
	parseErr := errors.New("front matter: invalid YAML: boom")
	d := FilenameDiagnostic([]string{`\#(fmvar(id)).md`}, "x.md",
		nil, parseErr, false, "kind note")
	require.NotNil(t, d)
	assert.Equal(t, "front matter: invalid YAML: boom", d.Hint)
	assert.NotContains(t, d.Hint, "frontmatter value missing")
}

// The parse error only explains an unresolved reference. A basename
// that satisfies a plain sibling glob still passes, and a list with
// no reference ignores the error entirely.
func TestFilenameDiagnostic_ParseErrorKeepsORAndPlainGlobs(t *testing.T) {
	parseErr := errors.New("front matter: invalid YAML: boom")
	assert.Nil(t, FilenameDiagnostic(
		[]string{"README.md", `\#(fmvar(id)).md`}, "README.md",
		nil, parseErr, false, "kind note"))
	d := FilenameDiagnostic([]string{"notes-*.md"}, "x.md",
		nil, parseErr, false, "kind note")
	require.NotNil(t, d)
	assert.Empty(t, d.Hint, "a plain glob has no reference to explain")
}

// With a parse error every front-matter field reads as absent, so the
// CUE field check would add a spurious "<missing>" for each required
// field. The parse diagnostic already names the cause; the field
// check is skipped and the filename diagnostic names the YAML error.
func TestValidateWithParseErr_SkipsFieldCheckAndNamesYAMLError(t *testing.T) {
	sch := &Schema{
		Filename:    []string{`\#(fmvar(id)).md`},
		Frontmatter: map[string]string{"id": "string"},
		Source:      "kind note",
	}
	doc := newDocFile(t, "x.md", "---\nid: [unclosed\n---\n# T\n")
	parseErr := errors.New("front matter: invalid YAML: boom")
	msgs := diagsMessages(
		ValidateWithParseErr(doc, sch, nil, parseErr, false, makeDiagForTest))
	require.Len(t, msgs, 1, "got %v", msgs)
	assert.Contains(t, msgs[0], "filename:")
	assert.Contains(t, msgs[0], "front matter: invalid YAML: boom")
	assert.NotContains(t, msgs[0], "missing")

	// Without a parse error the field check still runs.
	msgs = diagsMessages(Validate(doc, sch, nil, false, makeDiagForTest))
	assert.Contains(t, msgs, "id: got <missing>, expected string")
}

// A reference inside a character class can resolve to an invalid
// glob (`[\#(fmvar(tag))]` with `tag: "-"` leaves `[-]`). The
// diagnostic quotes the pattern the author wrote, as the path-pattern
// surface does, and shows the resolved form in the hint.
func TestFilenameDiagnostic_InvalidResolvedGlobShowsAuthoredPattern(t *testing.T) {
	const pat = `[\#(fmvar(tag))].md`
	d := FilenameDiagnostic([]string{"README.md", pat}, "x.md",
		map[string]any{"tag": "-"}, nil, false, "kind note")
	require.NotNil(t, d)
	assert.Equal(t, "filename pattern", d.Field)
	assert.Equal(t, `"[\\#(fmvar(tag))].md"`, d.Actual)
	assert.Equal(t, "syntax error in pattern; with front matter applied: [-].md",
		d.Hint)
}

// A malformed plain glob has no front matter to apply: the hint is
// the syntax error alone, and Actual is the glob as written.
func TestFilenameDiagnostic_InvalidPlainGlobUnchanged(t *testing.T) {
	d := FilenameDiagnostic([]string{"[.md"}, "x.md",
		nil, nil, false, "kind note")
	require.NotNil(t, d)
	assert.Equal(t, `"[.md"`, d.Actual)
	assert.Equal(t, "syntax error in pattern", d.Hint)
}

// Under `cue-frontmatter` no value was applied, so a malformed entry
// carries no "with front matter applied" hint.
func TestFilenameDiagnostic_InvalidGlobUnderCUEHasNoAppliedHint(t *testing.T) {
	const pat = `\#(fmvar(id))[.md`
	d := FilenameDiagnostic([]string{pat}, "x.md",
		map[string]any{"id": "string"}, nil, true, "kind note")
	require.NotNil(t, d)
	assert.Equal(t, strconv.Quote(pat), d.Actual)
	assert.Equal(t, "syntax error in pattern", d.Hint)
}

// A reference whose path walks into a scalar (`fmvar(a.b)` with
// `a: x`) names that key as not a mapping instead of reporting the
// field as missing: `a` is there, it just holds no `b`.
func TestFmvarGlobValue_NonMapIntermediateIsNamed(t *testing.T) {
	_, err := fmvarGlobValue(map[string]any{"a": "x"}, "a.b")
	require.Error(t, err)
	assert.Equal(t, "`fmvar(a.b)`: front-matter key \"a\" is not a map",
		err.Error())
	_, err = fmvarGlobValue(map[string]any{"a": map[string]any{}}, "a.b")
	assert.EqualError(t, err, "`fmvar(a.b)`: frontmatter value missing")
}
