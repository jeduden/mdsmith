package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func relatedDiag() lint.Diagnostic {
	return lint.Diagnostic{
		File: "task.md", Line: 1, Column: 1,
		RuleID: "MDS020", RuleName: "required-structure",
		Severity: lint.Error,
		Message:  `status: got "draft", expected one of "open"`,
		RelatedLocations: []lint.RelatedLocation{{
			File: "plan/proto.md", Line: 4,
			Message: `schema requires one of: "open", "in-progress"`,
		}},
	}
}

func TestTextFormatter_RelatedTrailerPlain(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, (&TextFormatter{}).Format(&buf, []lint.Diagnostic{relatedDiag()}))
	out := buf.String()
	assert.Contains(t, out, "↳ plan/proto.md:4 — ")
	assert.Contains(t, out, `schema requires one of: "open", "in-progress"`)
	assert.NotContains(t, out, "\033[", "no color codes when Color is false")
}

func TestTextFormatter_RelatedTrailerColor(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, (&TextFormatter{Color: true}).Format(&buf, []lint.Diagnostic{relatedDiag()}))
	out := buf.String()
	assert.Contains(t, out, "↳ ")
	assert.Contains(t, out, "\033[2m", "dim sequence around the related body")
}

func TestTextFormatter_RelatedFileOnlyNoLine(t *testing.T) {
	d := relatedDiag()
	d.RelatedLocations = []lint.RelatedLocation{{File: "plan/proto.md", Message: "schema constraint"}}
	var buf bytes.Buffer
	require.NoError(t, (&TextFormatter{}).Format(&buf, []lint.Diagnostic{d}))
	out := buf.String()
	assert.Contains(t, out, "↳ plan/proto.md — schema constraint")
	assert.NotContains(t, out, "proto.md:0", "no :0 when line is unknown")
}

func TestTextFormatter_RelatedMessageOnlyNoFile(t *testing.T) {
	d := relatedDiag()
	d.RelatedLocations = []lint.RelatedLocation{{Message: "inline kind schema"}}
	var buf bytes.Buffer
	require.NoError(t, (&TextFormatter{}).Format(&buf, []lint.Diagnostic{d}))
	out := buf.String()
	assert.Contains(t, out, "↳ inline kind schema")
	assert.NotContains(t, out, " — ", "no separator when there is no location")
}

func TestTextFormatter_RelatedSanitizesControlChars(t *testing.T) {
	d := relatedDiag()
	d.RelatedLocations = []lint.RelatedLocation{{
		File: "a\nb.md", Line: 2, Message: "inject\x1b[31mred",
	}}
	var buf bytes.Buffer
	require.NoError(t, (&TextFormatter{}).Format(&buf, []lint.Diagnostic{d}))
	out := buf.String()
	assert.NotContains(t, out, "\n\n", "embedded newline stripped from related line")
	assert.NotContains(t, out, "\x1b[31m", "embedded ANSI stripped")
}

func TestTextFormatter_NoRelatedNoTrailer(t *testing.T) {
	d := relatedDiag()
	d.RelatedLocations = nil
	var buf bytes.Buffer
	require.NoError(t, (&TextFormatter{}).Format(&buf, []lint.Diagnostic{d}))
	assert.NotContains(t, buf.String(), "↳")
}

func TestJSONFormatter_RelatedLocations(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, (&JSONFormatter{}).Format(&buf, []lint.Diagnostic{relatedDiag()}))

	var got []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got, 1)
	rls, ok := got[0]["related_locations"].([]any)
	require.True(t, ok, "related_locations present")
	require.Len(t, rls, 1)
	rl := rls[0].(map[string]any)
	assert.Equal(t, "plan/proto.md", rl["file"])
	assert.Equal(t, float64(4), rl["line"])
	assert.Equal(t, `schema requires one of: "open", "in-progress"`, rl["message"])
}

func TestJSONFormatter_RelatedOmittedWhenAbsent(t *testing.T) {
	d := relatedDiag()
	d.RelatedLocations = nil
	var buf bytes.Buffer
	require.NoError(t, (&JSONFormatter{}).Format(&buf, []lint.Diagnostic{d}))
	out := buf.String()
	assert.False(t, strings.Contains(out, "related_locations"), "omitempty drops empty related list")
}

func TestTextFormatter_RelatedFileOnlyNoMessage(t *testing.T) {
	d := relatedDiag()
	d.RelatedLocations = []lint.RelatedLocation{{File: "plan/proto.md", Line: 7}}
	var buf bytes.Buffer
	require.NoError(t, (&TextFormatter{}).Format(&buf, []lint.Diagnostic{d}))
	out := buf.String()
	assert.Contains(t, out, "↳ plan/proto.md:7")
	assert.NotContains(t, out, " — ", "no separator when the location has no message")
}

func TestTextFormatter_RelatedEmptyLocationSkipped(t *testing.T) {
	d := relatedDiag()
	d.RelatedLocations = []lint.RelatedLocation{{}} // neither file nor message
	var buf bytes.Buffer
	require.NoError(t, (&TextFormatter{}).Format(&buf, []lint.Diagnostic{d}))
	assert.NotContains(t, buf.String(), "↳", "an empty related location renders nothing")
}
