package schema

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSchemaDiagnostic_Format_AllFields covers the full
// SchemaDiagnostic render with field, actual, expected, hint, and
// schema reference present. The output is a two-block message: the
// violation summary first, then the trailing schema-ref line so it
// stays greppable.
func TestSchemaDiagnostic_Format_AllFields(t *testing.T) {
	d := SchemaDiagnostic{
		Field:     "status",
		Actual:    `"draft"`,
		Expected:  `one of: "open", "in-progress", "done"`,
		Hint:      `did you mean "open"?`,
		SchemaRef: "plan/proto.md:4",
	}
	got := d.Format()
	want := "status: got \"draft\", expected one of: \"open\", \"in-progress\", \"done\"\n" +
		"  (did you mean \"open\"?)"
	assert.Equal(t, want, got, "Format no longer carries the schema trailer")

	// The schema reference now rides on a structured related location.
	rl, ok := d.related()
	assert.True(t, ok)
	assert.Equal(t, "plan/proto.md", rl.File)
	assert.Equal(t, 4, rl.Line)
	assert.Equal(t, "required by schema", rl.Message)
}

// TestSchemaDiagnostic_Format_NoActual covers the branch
// where Field and Expected are set but Actual is empty: the
// rendered message uses ": expected ..." rather than the
// "got X, expected Y" form, so absent-but-known constraints
// (e.g. a wildcard-text scope) read cleanly.
func TestSchemaDiagnostic_Format_NoActual(t *testing.T) {
	d := SchemaDiagnostic{
		Field:     "## Goal",
		Expected:  "section to be present",
		SchemaRef: "kind plan",
	}
	want := "## Goal: expected section to be present"
	assert.Equal(t, want, d.Format())

	// "kind plan" is a descriptive label, not a file path, so the
	// related location is message-only (no navigable file/line).
	rl, ok := d.related()
	assert.True(t, ok)
	assert.Equal(t, "", rl.File)
	assert.Equal(t, "kind plan", rl.Message)
}

// TestSchemaDiagnostic_Format_FieldOnly covers the very
// minimal render: just a field name with no actual,
// expected, hint, or schema ref. The renderer should still
// produce a non-empty string.
func TestSchemaDiagnostic_Format_FieldOnly(t *testing.T) {
	d := SchemaDiagnostic{Field: "field"}
	assert.Equal(t, "field", d.Format())
}

// TestSchemaDiagnostic_Related_Variants pins how each SchemaRef shape
// maps onto a structured related location: file:line and file-only
// refs are navigable; descriptive labels (with spaces or no path
// separator) become message-only; an empty ref yields no location.
func TestSchemaDiagnostic_Related_Variants(t *testing.T) {
	cases := []struct {
		name     string
		ref      string
		wantOK   bool
		wantFile string
		wantLine int
		wantMsg  string
	}{
		{"file with line", "plan/proto.md:4", true, "plan/proto.md", 4, "required by schema"},
		{"file without line", "plan/proto.md", true, "plan/proto.md", 0, "required by schema"},
		{"inline label", "inline kind schema", true, "", 0, "inline kind schema"},
		{"path-pattern label", "kinds[plan] / path-pattern", true, "", 0, "kinds[plan] / path-pattern"},
		{"single word label", "schema", true, "", 0, "schema"},
		{"colon but non-numeric", "kind:rfc", true, "", 0, "kind:rfc"},
		{"no ref", "", false, "", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rl, ok := SchemaDiagnostic{SchemaRef: c.ref}.related()
			require.Equal(t, c.wantOK, ok)
			if !c.wantOK {
				return
			}
			assert.Equal(t, c.wantFile, rl.File)
			assert.Equal(t, c.wantLine, rl.Line)
			assert.Equal(t, c.wantMsg, rl.Message)
		})
	}
}

// TestSchemaDiagnostic_Related_Deprecated covers the deprecation
// label on a navigable ref.
func TestSchemaDiagnostic_Related_Deprecated(t *testing.T) {
	rl, ok := SchemaDiagnostic{SchemaRef: "plan/proto.md:4", Deprecated: true}.related()
	require.True(t, ok)
	assert.Equal(t, "deprecated by schema", rl.Message)
}

// TestSchemaDiagnostic_Emit_AttachesRelated checks that Emit runs the
// message through MakeDiag and attaches the schema reference as a
// related location rather than leaving it in the message body.
func TestSchemaDiagnostic_Emit_AttachesRelated(t *testing.T) {
	mk := func(file string, line int, msg string) lint.Diagnostic {
		return lint.Diagnostic{File: file, Line: line, Message: msg, RuleID: "MDS020"}
	}
	d := SchemaDiagnostic{
		Field: "status", Actual: `"x"`, Expected: `one of: "a"`,
		SchemaRef: "plan/proto.md:4",
	}
	got := d.Emit(mk, "task.md", 7)
	assert.Equal(t, "task.md", got.File)
	assert.Equal(t, 7, got.Line)
	assert.NotContains(t, got.Message, "schema:", "schema ref no longer in message")
	require.Len(t, got.RelatedLocations, 1)
	assert.Equal(t, "plan/proto.md", got.RelatedLocations[0].File)
	assert.Equal(t, 4, got.RelatedLocations[0].Line)
}

// TestSchemaDiagnostic_Emit_NoRefNoRelated checks that a diagnostic
// without a SchemaRef gets no related location.
func TestSchemaDiagnostic_Emit_NoRefNoRelated(t *testing.T) {
	mk := func(file string, line int, msg string) lint.Diagnostic {
		return lint.Diagnostic{File: file, Line: line, Message: msg}
	}
	got := SchemaDiagnostic{Field: "x"}.Emit(mk, "a.md", 1)
	assert.Nil(t, got.RelatedLocations)
}

// TestFormatActual_FallbackOnMarshalError exercises the
// final fmt.Sprintf("%v", v) fallback. json.Marshal fails
// on channels, functions, and complex values; the helper
// degrades to the default formatter so the caller still
// gets a printable string.
func TestFormatActual_FallbackOnMarshalError(t *testing.T) {
	// Channels can't be JSON-marshalled; the helper falls
	// through to %v which produces a pointer-shaped string.
	got := formatActual(make(chan int))
	assert.NotEmpty(t, got)
}

// TestSchemaDiagnostic_Format_NoHint covers the no-hint branch:
// when the extractor cannot suggest a fix, the message ends at the
// expected line and goes straight to the schema reference.
func TestSchemaDiagnostic_Format_NoHint(t *testing.T) {
	d := SchemaDiagnostic{
		Field:     "id",
		Actual:    `"BAD"`,
		Expected:  "string matching ^RFC-[0-9]{4}$",
		SchemaRef: "kind rfc",
	}
	got := d.Format()
	want := "id: got \"BAD\", expected string matching ^RFC-[0-9]{4}$"
	assert.Equal(t, want, got)
}

// TestRenderExpected_ShapeTable exercises every shape listed in the
// plan 147 expected-value table; the fallback "raw expression"
// branch is covered by the unknown-shape case.
func TestRenderExpected_ShapeTable(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "string-disjunction",
			expr: `"a" | "b" | "c"`,
			want: `one of: "a", "b", "c"`,
		},
		{
			name: "regex",
			expr: `=~"^FOO-[0-9]{4}$"`,
			want: `string matching ^FOO-[0-9]{4}$`,
		},
		{
			name: "regex-with-string-prefix",
			expr: `string & =~"^A$"`,
			want: `string matching ^A$`,
		},
		{
			name: "int-range-inclusive",
			expr: `int & >=1 & <=5`,
			want: `int between 1 and 5`,
		},
		{
			name: "int-range-exclusive",
			expr: `int & >0 & <10`,
			want: `int between 1 and 9`,
		},
		{
			name: "int-lower-only",
			expr: `int & >=1`,
			want: `int >= 1`,
		},
		{
			name: "non-empty-string",
			expr: `string & != ""`,
			want: `non-empty string`,
		},
		{
			name: "bool",
			expr: `bool`,
			want: `true or false`,
		},
		{
			name: "fallback-raw",
			expr: `[...string]`,
			want: `[...string]`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, RenderExpected(tc.expr))
		})
	}
}

// TestParseFMBlockKeyLines_BlockScalarWithFenceSequence
// regresses a Copilot review observation: the closing front-
// matter fence was previously located via bytes.Index, which
// would truncate early if a YAML block scalar value contained
// the literal `---\n` sequence (e.g. a `notes: |` block whose
// body included an em-dash row). TrimSuffix on the canonical
// closing fence is both safer and simpler.
func TestParseFMBlockKeyLines_BlockScalarWithFenceSequence(t *testing.T) {
	// `notes:` is a block scalar whose value contains "---\n"
	// inside the body. The closing `---\n` fence sits at the
	// end. The walker must still see both top-level keys and
	// place each on its source line.
	fm := []byte(
		"---\n" +
			"id: 1\n" +
			"notes: |\n" +
			"  ---\n" +
			"  some literal text\n" +
			"status: open\n" +
			"---\n")
	lines := parseFMBlockKeyLines(fm)
	require.NotNil(t, lines)
	assert.Equal(t, 2, lines["id"])
	assert.Equal(t, 3, lines["notes"])
	assert.Equal(t, 6, lines["status"])
}

// TestValidateFrontmatterDiags_CompileFailureCarriesSchemaRef
// regresses the Copilot review observation that early-return
// diagnostics for unrecoverable CUE/JSON failures used to drop
// the schema reference. Every diagnostic the validator emits now
// uses SchemaDiagnostic, so the source rides on a structured
// related location rather than the message body (plan 230).
func TestValidateFrontmatterDiags_CompileFailureCarriesSchemaRef(t *testing.T) {
	sch := &Schema{
		Source: "kind broken",
		Frontmatter: map[string]string{
			// Deliberately malformed CUE expression so
			// CompileString fails on the schema itself.
			"id": `int &`,
		},
	}
	doc := newDocFile(t, "doc.md", "# T\n")
	diags := ValidateFrontmatterDiags(doc, sch, map[string]any{"id": 1}, makeDiagForTest)
	require.NotEmpty(t, diags)
	require.Len(t, diags[0].RelatedLocations, 1)
	assert.Equal(t, "kind broken", diags[0].RelatedLocations[0].Message)
}

// TestRenderExpected_IntRangeOverflowGuard regresses two
// Copilot review observations:
//
//   - `int & >MaxInt` would previously do `MaxInt + 1` and
//     silently wrap to MinInt, producing a meaningless inclusive
//     bound.
//   - Converting it to `int >= MaxInt` is also wrong: the
//     original constraint `int > MaxInt` is unsatisfiable, but
//     `int >= MaxInt` accepts MaxInt itself.
//
// The renderer now keeps the exclusive form when the bound
// can't be converted without overflow, so the rendered message
// preserves the original semantics.
func TestRenderExpected_IntRangeOverflowGuard(t *testing.T) {
	maxStr := strconv.Itoa(math.MaxInt)
	minStr := strconv.Itoa(math.MinInt)

	// >MaxInt: cannot become >=MaxInt+1, so render `int > MaxInt`
	// (exclusive) rather than the misleading inclusive form.
	rendered := RenderExpected("int & >" + maxStr)
	assert.Equal(t, "int > "+maxStr, rendered,
		"upper-overflow guard should keep the exclusive form")

	// <MinInt: mirror case for the upper bound.
	rendered = RenderExpected("int & <" + minStr)
	assert.Equal(t, "int < "+minStr, rendered,
		"lower-overflow guard should keep the exclusive form")
}

// TestRenderHint_StringDisjunctionTypo covers the Levenshtein
// path: an actual within distance 2 of a literal alternative gets
// suggested.
func TestRenderHint_StringDisjunctionTypo(t *testing.T) {
	expr := `"draft" | "open" | "done"`
	assert.Equal(t, `did you mean "draft"?`, RenderHint(expr, "draf"))
	assert.Equal(t, `did you mean "open"?`, RenderHint(expr, "ope"))
}

// TestRenderHint_StringDisjunctionTooFar exercises the silent
// branch: an actual far from any literal returns no hint so the
// message doesn't add noise.
func TestRenderHint_StringDisjunctionTooFar(t *testing.T) {
	expr := `"draft" | "open" | "done"`
	assert.Empty(t, RenderHint(expr, "completely-different"))
}

// TestRenderHint_IntRange covers the numeric-range suggestion:
// a value just outside the range is hinted with the nearest bound.
func TestRenderHint_IntRange(t *testing.T) {
	expr := `int & >=1 & <=5`
	assert.Equal(t, "try 1", RenderHint(expr, float64(0)))
	assert.Equal(t, "try 5", RenderHint(expr, float64(6)))
	// In-range values get no hint.
	assert.Empty(t, RenderHint(expr, float64(3)))
}

// TestRenderHint_HugeActualSkipsLev regresses the Copilot review
// finding that levenshtein previously materialised a full rune
// slice before consulting maxLevInput. A multi-megabyte actual
// value should short-circuit without allocating its rune
// representation, and the returned hint should still be empty
// because the input is too dissimilar from any literal.
func TestRenderHint_HugeActualSkipsLev(t *testing.T) {
	expr := `"open" | "done"`
	huge := strings.Repeat("a", 1<<20) // 1 MiB
	assert.Empty(t, RenderHint(expr, huge),
		"oversized actual should not produce a hint")
}

// TestValidateFrontmatterDiags_OnePerError covers acceptance
// criterion 2: three FM violations produce three diagnostics, each
// anchored to its own field.
func TestValidateFrontmatterDiags_OnePerError(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"a": `"x" | "y"`,
			"b": `int & >=1`,
			"c": `=~"^Z"`,
		},
	}
	sch, err := ParseInline(raw, "kind t")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n")
	docFM := map[string]any{
		"a": "wrong",
		"b": float64(0),
		"c": "nope",
	}
	diags := ValidateFrontmatterDiags(doc, sch, docFM, makeDiagForTest)
	require.Len(t, diags, 3)
	fields := map[string]bool{}
	for _, d := range diags {
		// The first segment of each message is the field name
		// followed by ": got ...".
		fields[firstField(d.Message)] = true
	}
	assert.True(t, fields["a"], "missing diagnostic for field a")
	assert.True(t, fields["b"], "missing diagnostic for field b")
	assert.True(t, fields["c"], "missing diagnostic for field c")
}

func firstField(msg string) string {
	for i := 0; i < len(msg); i++ {
		if msg[i] == ':' {
			return msg[:i]
		}
	}
	return msg
}

// TestValidateFrontmatterDiags_PopulatesRuleIDAndName regresses
// acceptance criterion 7: every emitted diagnostic carries
// MDS020/required-structure so the LSP can map them to Source and
// Code on the wire.
func TestValidateFrontmatterDiags_PopulatesRuleIDAndName(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"id": `int & >=1`,
		},
	}
	sch, err := ParseInline(raw, "kind t")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n")
	docFM := map[string]any{"id": "not-an-int"}
	mkDiag := func(file string, line int, msg string) lint.Diagnostic {
		return lint.Diagnostic{
			File:     file,
			Line:     line,
			RuleID:   "MDS020",
			RuleName: "required-structure",
			Message:  msg,
		}
	}
	diags := ValidateFrontmatterDiags(doc, sch, docFM, mkDiag)
	require.NotEmpty(t, diags)
	for _, d := range diags {
		assert.Equal(t, "MDS020", d.RuleID)
		assert.Equal(t, "required-structure", d.RuleName)
	}
}

// TestSchemaRef_FileWithLine covers the proto.md schema-ref form:
// when the schema parser captured the per-key line in
// FrontmatterLines, the rendered ref appends ":<line>" so users
// jump straight to the constraint.
func TestSchemaRef_FileWithLine(t *testing.T) {
	sch := &Schema{
		Source: "plan/proto.md",
		Frontmatter: map[string]string{
			"status": `"open" | "done"`,
		},
		FrontmatterLines: map[string]int{
			"status": 4,
		},
	}
	assert.Equal(t, "plan/proto.md:4", FormatSchemaRef(sch, "status"))
}

// TestSchemaRef_WithoutLine covers the fallback branch: an inline
// schema (no per-key lines) emits just the source label.
func TestSchemaRef_WithoutLine(t *testing.T) {
	sch := &Schema{Source: "kind rfc"}
	assert.Equal(t, "kind rfc", FormatSchemaRef(sch, "status"))
}

// TestValidateFrontmatterDiags_MissingRequiredFieldShowsMissing
// regresses a Copilot review comment: a required field absent
// from the document should surface as `<missing>` in the actual
// slot rather than dropping that segment entirely, so every
// diagnostic answers field / got / expected.
func TestValidateFrontmatterDiags_MissingRequiredFieldShowsMissing(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"id": `int & >=1`,
		},
	}
	sch, err := ParseInline(raw, "kind t")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n")
	diags := ValidateFrontmatterDiags(doc, sch, nil, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "id: got <missing>")
	assert.Contains(t, diags[0].Message, "expected int >= 1")
}

// TestValidateFrontmatterDiags_AnchorsAtKeyLine regresses the
// Copilot finding that FM diagnostics were anchored at line 1
// regardless of where the offending key lived. With doc FM
// parsing we now point at the key's actual source line so the
// editor squiggle lands on the right row.
func TestValidateFrontmatterDiags_AnchorsAtKeyLine(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"id":     `int & >=1`,
			"status": `"open" | "done"`,
		},
	}
	sch, err := ParseInline(raw, "kind t")
	require.NoError(t, err)
	// status sits on file line 3: line 1 = "---", line 2 = id,
	// line 3 = status, line 4 = "---", line 5 = body.
	doc := newDocFile(t, "doc.md",
		"---\nid: 1\nstatus: bogus\n---\n# Body\n")
	docFM := map[string]any{"id": float64(1), "status": "bogus"}
	diags := ValidateFrontmatterDiags(doc, sch, docFM, makeDiagForTest)
	require.NotEmpty(t, diags)
	var statusLine int
	for _, d := range diags {
		if strings.HasPrefix(d.Message, "status:") {
			statusLine = d.Line
		}
	}
	assert.Equal(t, 3, statusLine,
		"status diagnostic should be anchored at the key's source line")
}

// TestFormatActual_DeterministicMap regresses the comment/behavior
// mismatch noted in code review: map values now JSON-marshal so
// the rendered output is stable across runs.
func TestFormatActual_DeterministicMap(t *testing.T) {
	v := map[string]any{"b": 2, "a": 1, "c": 3}
	// Two renders must match — Go's default map formatter does not
	// guarantee key order, but JSON marshalling does.
	got1 := formatActual(v)
	got2 := formatActual(v)
	assert.Equal(t, got1, got2)
	assert.Equal(t, `{"a":1,"b":2,"c":3}`, got1)
}

// TestLookupFM_HandlesListIndices regresses a Copilot review
// finding: a CUE error path that includes a list index (e.g.
// `tags.1`) used to bottom out at the slice step because
// lookupFM only descended through map[string]any. The diagnostic
// then claimed `<missing>` for a value that was actually
// present. lookupFM now parses numeric segments as list indices
// and returns the leaf value.
func TestLookupFM_HandlesListIndices(t *testing.T) {
	fm := map[string]any{
		"tags": []any{"alpha", "beta", "gamma"},
		"nested": map[string]any{
			"items": []any{
				map[string]any{"id": "x"},
				map[string]any{"id": "y"},
			},
		},
	}
	v, ok := lookupFM(fm, []string{"tags", "1"})
	require.True(t, ok)
	assert.Equal(t, "beta", v)

	v, ok = lookupFM(fm, []string{"nested", "items", "1", "id"})
	require.True(t, ok)
	assert.Equal(t, "y", v)

	// Out-of-range index reports absent.
	_, ok = lookupFM(fm, []string{"tags", "99"})
	assert.False(t, ok)

	// Non-numeric segment on a list reports absent.
	_, ok = lookupFM(fm, []string{"tags", "name"})
	assert.False(t, ok)
}

// TestValidateFrontmatterDiags_NullDistinctFromMissing regresses
// the Copilot review observation that an explicit YAML null was
// rendering as `<missing>`, conflating "field present but null"
// with "field absent". The two cases should now produce distinct
// "got" values so users can tell which problem they have.
func TestValidateFrontmatterDiags_NullDistinctFromMissing(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"id": `int & >=1`,
		},
	}
	sch, err := ParseInline(raw, "kind t")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n")

	// Explicit null: docFM has the key set to nil.
	nullDiags := ValidateFrontmatterDiags(doc, sch, map[string]any{"id": nil}, makeDiagForTest)
	require.NotEmpty(t, nullDiags)
	assert.Contains(t, nullDiags[0].Message, "id: got null")
	assert.NotContains(t, nullDiags[0].Message, "<missing>")

	// Absent: docFM does not carry the key at all.
	missingDiags := ValidateFrontmatterDiags(doc, sch, map[string]any{}, makeDiagForTest)
	require.NotEmpty(t, missingDiags)
	assert.Contains(t, missingDiags[0].Message, "id: got <missing>")
	assert.NotContains(t, missingDiags[0].Message, "got null")
}
