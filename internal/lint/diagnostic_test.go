package lint

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isGCPointerKind reports whether a reflect.Kind needs GC pointer
// scanning (string/slice/map/ptr/interface headers all carry at
// least one pointer word; UnsafePointer is a bare pointer word too).
func isGCPointerKind(k reflect.Kind) bool {
	switch k {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Ptr,
		reflect.Interface, reflect.Chan, reflect.Func,
		reflect.UnsafePointer:
		return true
	default:
		return false
	}
}

// assertPointerFieldsLeading fails if typ declares a pointer-
// containing field after a scalar field. Go's GC computes a struct's
// ptrdata as the byte offset through its last pointer-containing
// field, so a scalar sandwiched between pointer fields forces the
// scanner to walk (and the type's metadata to describe) bytes that
// hold no pointer — see docs/development/high-performance-go.md
// "Struct layout". Grouping every pointer field first keeps ptrdata
// minimal.
func assertPointerFieldsLeading(t *testing.T, typ reflect.Type) {
	t.Helper()
	sawScalar := false
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if isGCPointerKind(f.Type.Kind()) {
			if sawScalar {
				t.Errorf("%s.%s is a pointer-containing field declared "+
					"after a scalar field; move it before every scalar "+
					"field to keep ptrdata minimal", typ.Name(), f.Name)
			}
			continue
		}
		sawScalar = true
	}
}

func TestDiagnosticFieldLayout_PointerFieldsLeading(t *testing.T) {
	assertPointerFieldsLeading(t, reflect.TypeOf(Diagnostic{}))
	assertPointerFieldsLeading(t, reflect.TypeOf(RelatedLocation{}))
	assertPointerFieldsLeading(t, reflect.TypeOf(Explanation{}))
}

func TestDiagnosticFields(t *testing.T) {
	d := Diagnostic{
		File:     "README.md",
		Line:     10,
		Column:   5,
		RuleID:   "MDS001",
		RuleName: "line-length",
		Severity: Error,
		Message:  "line too long (120 > 80)",
	}

	assert.Equal(t, "README.md", d.File)
	assert.Equal(t, 10, d.Line)
	assert.Equal(t, 5, d.Column)
	assert.Equal(t, "MDS001", d.RuleID)
	assert.Equal(t, "line-length", d.RuleName)
	assert.Equal(t, Error, d.Severity)
	assert.Equal(t, "line too long (120 > 80)", d.Message)
}

func TestDiagnosticRelatedLocations(t *testing.T) {
	d := Diagnostic{
		File:     "task.md",
		Line:     1,
		RuleID:   "MDS020",
		RuleName: "required-structure",
		Severity: Error,
		Message:  `status: got "draft", expected one of "open"`,
		RelatedLocations: []RelatedLocation{{
			File:    "plan/proto.md",
			Line:    4,
			Column:  3,
			Message: `schema requires one of: "open", "in-progress"`,
		}},
	}

	if assert.Len(t, d.RelatedLocations, 1) {
		rl := d.RelatedLocations[0]
		assert.Equal(t, "plan/proto.md", rl.File)
		assert.Equal(t, 4, rl.Line)
		assert.Equal(t, 3, rl.Column)
		assert.Equal(t, `schema requires one of: "open", "in-progress"`, rl.Message)
	}
}

func TestDiagnosticZeroValueHasNoRelatedLocations(t *testing.T) {
	// A diagnostic built without the new field keeps the old shape:
	// a nil related-locations slice.
	d := Diagnostic{File: "README.md", Line: 10, RuleID: "MDS001"}
	assert.Nil(t, d.RelatedLocations)
}

func TestSeverityConstants(t *testing.T) {
	assert.Equal(t, Severity("error"), Error)
	assert.Equal(t, Severity("warning"), Warning)
}

func TestLineRange_Contains(t *testing.T) {
	r := LineRange{From: 5, To: 8}
	assert.True(t, r.Contains(5), "start boundary")
	assert.True(t, r.Contains(6), "middle")
	assert.True(t, r.Contains(8), "end boundary")
	assert.False(t, r.Contains(4), "before range")
	assert.False(t, r.Contains(9), "after range")
}

// TestDiagnostic_DisplayLineClamp covers DisplayLine: a non-positive
// sentinel (plan 230's wholly-generated anchor) clamps to 1 for 1-based
// output, while a real line passes through unchanged.
func TestDiagnostic_DisplayLineClamp(t *testing.T) {
	assert.Equal(t, 1, Diagnostic{Line: 0}.DisplayLine(), "zero clamps to 1")
	assert.Equal(t, 1, Diagnostic{Line: -3}.DisplayLine(), "negative clamps to 1")
	assert.Equal(t, 7, Diagnostic{Line: 7}.DisplayLine(), "real line passes through")
}

func TestDedupeDiagnostics_nil(t *testing.T) {
	assert.Nil(t, DedupeDiagnostics(nil))
}

func TestDedupeDiagnostics_single(t *testing.T) {
	d := Diagnostic{File: "a.md", Line: 1, RuleID: "MDS001", Message: "x"}
	out := DedupeDiagnostics([]Diagnostic{d})
	require.Len(t, out, 1)
	assert.Equal(t, d, out[0])
}

func TestDedupeDiagnostics_removeDuplicates(t *testing.T) {
	d := Diagnostic{File: "a.md", Line: 1, RuleID: "MDS001", Message: "x"}
	out := DedupeDiagnostics([]Diagnostic{d, d})
	require.Len(t, out, 1)
}

func TestDedupeDiagnostics_keepsDistinct(t *testing.T) {
	d1 := Diagnostic{File: "a.md", Line: 1, RuleID: "MDS001", Message: "x"}
	d2 := Diagnostic{File: "a.md", Line: 2, RuleID: "MDS001", Message: "x"}
	out := DedupeDiagnostics([]Diagnostic{d1, d2})
	require.Len(t, out, 2)
}
