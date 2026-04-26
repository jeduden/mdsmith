package metrics

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCollect_AuthoredMetrics verifies that a host file with an <?include?>
// section reports the same metric values as the same host with the generated
// section emptied. This is the "authored-only" guarantee from plan 94.
func TestCollect_AuthoredMetrics(t *testing.T) {
	dir := t.TempDir()

	// hostFull.md has an <?include?> section with 100 lines of content.
	bigContent := ""
	for i := 0; i < 100; i++ {
		bigContent += "Generated line with some words here.\n"
	}
	hostFull := "# Host\n\nAuthor wrote this.\n\n" +
		"<?include\nfile: frag.md\n?>\n" +
		bigContent +
		"<?/include?>\n"
	fullPath := filepath.Join(dir, "hostFull.md")
	require.NoError(t, os.WriteFile(fullPath, []byte(hostFull), 0o644))

	// hostEmpty.md is the same file but with the generated section emptied.
	hostEmpty := "# Host\n\nAuthor wrote this.\n\n" +
		"<?include\nfile: frag.md\n?>\n" +
		"<?/include?>\n"
	emptyPath := filepath.Join(dir, "hostEmpty.md")
	require.NoError(t, os.WriteFile(emptyPath, []byte(hostEmpty), 0o644))

	defs := Defaults(ScopeFile)
	rows, err := Collect([]string{fullPath, emptyPath}, defs, 0)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	byPath := make(map[string]Row, 2)
	for _, row := range rows {
		byPath[row.Path] = row
	}

	full := byPath[fullPath]
	empty := byPath[emptyPath]

	for _, def := range defs {
		fv := full.Metrics[def.Name]
		ev := empty.Metrics[def.Name]
		assert.Equal(t, ev.Available, fv.Available,
			"metric %s: availability mismatch between full and empty", def.Name)
		if fv.Available && ev.Available {
			assert.InDelta(t, ev.Number, fv.Number, 1e-6,
				"metric %s: full host (%v) must equal empty host (%v) — authored-only",
				def.Name, fv.Number, ev.Number)
		}
	}
}

// TestCollect_NoDirectives_Unchanged verifies that Collect does not change
// metric values for a plain file that has no generated sections.
func TestCollect_NoDirectives_Unchanged(t *testing.T) {
	dir := t.TempDir()
	plain := "# Hello\n\nSome text here.\n"
	plainPath := filepath.Join(dir, "plain.md")
	require.NoError(t, os.WriteFile(plainPath, []byte(plain), 0o644))

	defs := Defaults(ScopeFile)
	rows, err := Collect([]string{plainPath}, defs, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	bytesVal := rows[0].Metrics["bytes"]
	require.True(t, bytesVal.Available, "bytes metric must be available")
	assert.InDelta(t, float64(len(plain)), bytesVal.Number, 1e-6,
		"bytes metric for plain file must equal raw file size")
}
