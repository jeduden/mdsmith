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
