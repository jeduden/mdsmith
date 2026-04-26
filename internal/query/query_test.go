package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var matchTests = []struct {
	name    string
	expr    string
	fm      map[string]any
	want    bool
	wantErr bool
}{
	{
		name: "matching field",
		expr: `status: "✅"`,
		fm:   map[string]any{"status": "✅", "id": 42},
		want: true,
	},
	{
		name: "non-matching field",
		expr: `status: "✅"`,
		fm:   map[string]any{"status": "🔲", "id": 42},
		want: false,
	},
	{
		name: "missing field",
		expr: `status: "✅"`,
		fm:   map[string]any{"id": 42},
		want: false,
	},
	{
		name: "nil front matter",
		expr: `status: "✅"`,
		fm:   nil,
		want: false,
	},
	{
		name: "schema-string proto value",
		expr: `status: "✅"`,
		fm:   map[string]any{"status": `"🔲" | "🔳" | "✅"`},
		want: false,
	},
	{
		name: "compound expression matches",
		expr: `status: "✅", id: >50`,
		fm:   map[string]any{"status": "✅", "id": 60},
		want: true,
	},
	{
		name: "compound expression partial fail",
		expr: `status: "✅", id: >50`,
		fm:   map[string]any{"status": "✅", "id": 30},
		want: false,
	},
	{
		name:    "invalid CUE expression",
		expr:    `status: [[[`,
		fm:      map[string]any{"status": "✅"},
		wantErr: true,
	},
	{
		name: "nested field matches",
		expr: `meta: status: "✅"`,
		fm:   map[string]any{"meta": map[string]any{"status": "✅"}},
		want: true,
	},
	{
		name: "nested field missing inner",
		expr: `meta: status: "✅"`,
		fm:   map[string]any{"meta": map[string]any{"title": "foo"}},
		want: false,
	},
	{
		name: "nested field missing outer",
		expr: `meta: status: "✅"`,
		fm:   map[string]any{"status": "✅"},
		want: false,
	},
}

func TestMatch(t *testing.T) {
	for _, tt := range matchTests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Match(tt.expr, tt.fm)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompile_Valid(t *testing.T) {
	m, err := Compile(`status: "✅"`)
	require.NoError(t, err)
	assert.True(t, m.Match(map[string]any{"status": "✅"}))
}

func TestCompile_Invalid(t *testing.T) {
	_, err := Compile(`status: [[[`)
	assert.Error(t, err)
}

// =====================================================================
// Phase 5: additional branch coverage
// =====================================================================

// TestCollectPaths_NonStructCUE exercises the collectPaths err != nil branch
// by compiling a CUE expression that is not a struct (a top-level scalar
// constraint). In that case, v.Fields() returns an error and collectPaths
// returns nil (no paths to verify), so Match always checks only unification.
func TestCollectPaths_NonStructCUE(t *testing.T) {
	// A top-level numeric constraint is valid CUE but not a struct.
	// collectPaths should return nil and Match should use unification only.
	m, err := Compile(`>=1 & <=10`)
	require.NoError(t, err)
	// A non-struct schema has no paths to verify. Match therefore falls back
	// to unification only, and a map-shaped front matter value cannot unify
	// with a top-level numeric constraint.
	result := m.Match(map[string]any{"value": 5})
	assert.False(t, result, "non-struct CUE schema should not match a map value")
}

// TestMatch_JSONMarshalError exercises the json.Marshal err != nil branch in Match.
// json.Marshal fails for types like channels or functions.
func TestMatch_JSONMarshalError(t *testing.T) {
	m, err := Compile(`status: "ready"`)
	require.NoError(t, err)
	// A chan value is not JSON-serializable → json.Marshal returns an error.
	result := m.Match(map[string]any{"status": make(chan int)})
	assert.False(t, result, "json.Marshal error should cause Match to return false")
}

// =====================================================================
// Placeholder grammar regression: CUE-pattern front matter
// =====================================================================

// TestMatch_CUEPatternFrontMatter verifies that a file with CUE-pattern
// front-matter values (as found in proto.md) can still be selected by
// the query matcher when the query uses field-existence checks.
//
// Files like proto.md hold CUE schema expressions as front-matter values
// (e.g. id: 'int & >=1'). The YAML parser reads them as ordinary strings,
// so readFrontMatterRaw produces map[string]any{"id": "int & >=1", ...}.
// The matcher should be able to select such files by field presence.
func TestMatch_CUEPatternFrontMatter(t *testing.T) {
	// Simulate the front matter of plan/proto.md after YAML parsing.
	fm := map[string]any{
		"id":      "int & >=1",
		"title":   `string & != ""`,
		"status":  `"🔲" | "🔳" | "✅" | "⛔"`,
		"summary": `string | *""`,
		"model":   `"haiku" | "sonnet" | "opus" | *""`,
	}

	// Query by field existence (underscore = any CUE value).
	m, err := Compile(`id: _`)
	require.NoError(t, err)
	assert.True(t, m.Match(fm), "file with CUE-pattern id field should be selectable by {id: _}")

	// Query by the literal CUE-expression string value.
	m2, err := Compile(`id: "int & >=1"`)
	require.NoError(t, err)
	assert.True(t, m2.Match(fm), "file with CUE-pattern id field should match literal string query")

	// Query for a field that is absent returns false.
	m3, err := Compile(`missing: _`)
	require.NoError(t, err)
	assert.False(t, m3.Match(fm), "absent field should not match")
}
