package config

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// byteColOf returns the 1-based byte column of the first needle on
// 1-based line ln of src.
func byteColOf(t *testing.T, src string, ln int, needle string) int {
	t.Helper()
	lines := bytes.Split([]byte(src), []byte("\n"))
	require.Less(t, ln-1, len(lines))
	i := bytes.Index(lines[ln-1], []byte(needle))
	require.GreaterOrEqual(t, i, 0)
	return i + 1
}

// nonASCIIYAML puts multi-byte text before the offending `end` key on
// the same line, so a character column and a byte column differ.
const nonASCIIYAML = "foreign-regions: [{start: \"éé\", end: \"\"}]\n"

func TestLoad_YAMLColumnIsByteOffset(t *testing.T) {
	p := writeCfg(t, t.TempDir(), ".mdsmith.yml", nonASCIIYAML)
	_, err := Load(p)
	le := requireLoadError(t, err)
	assert.Equal(t, 1, le.Line)
	assert.Equal(t, byteColOf(t, nonASCIIYAML, 1, "end"), le.Column)
}

func TestParseBytes_YAMLColumnIsByteOffset(t *testing.T) {
	_, err := ParseBytes([]byte(nonASCIIYAML))
	le := requireLoadError(t, err)
	assert.Equal(t, 1, le.Line)
	assert.Equal(t, byteColOf(t, nonASCIIYAML, 1, "end"), le.Column)
}

func TestSidecarPosition_ColumnIsByteOffset(t *testing.T) {
	body := "rules: {}\nmeta: {a: \"éé\", b: 1}\n"
	kf := writeCfg(t, t.TempDir(), "plan.yml", body)
	want := byteColOf(t, body, 2, "b:")

	line, col, data := sidecarPosition(&Issue{File: kf, Path: KeyPath{"kinds", "plan", "meta", "b"}})
	assert.Equal(t, []byte(body), data)
	assert.Equal(t, 2, line)
	assert.Equal(t, want, col, "a path-resolved sidecar column")

	line, col, data = sidecarPosition(&Issue{File: kf, Line: 2, Column: want - 2})
	assert.Equal(t, []byte(body), data)
	assert.Equal(t, 2, line)
	assert.Equal(t, want, col, "a pre-resolved sidecar (decoder) column")

	line, col, data = sidecarPosition(&Issue{File: filepath.Join(t.TempDir(), "gone.yml"), Line: 3, Column: 4})
	assert.Nil(t, data)
	assert.Equal(t, 3, line)
	assert.Equal(t, 4, col, "an unreadable sidecar keeps the parser column")
}

func TestByteColumn(t *testing.T) {
	src := []byte("ab\nééx\n")
	cases := []struct {
		name            string
		line, col, want int
	}{
		{"ascii", 1, 2, 2},
		{"after two-byte runes", 2, 3, 5},
		{"past end of line clamps to the newline", 2, 9, 6},
		{"line out of range unchanged", 9, 3, 3},
		{"line zero unchanged", 0, 3, 3},
		{"column zero unchanged", 1, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, byteColumn(src, tc.line, tc.col))
		})
	}
}
