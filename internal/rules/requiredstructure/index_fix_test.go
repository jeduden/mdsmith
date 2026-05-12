package requiredstructure

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFix_WritesIndexSideOutput verifies the end-to-end Fix path:
// given an inline schema with an `index:` block, calling Rule.Fix
// writes the JSON file next to the source. This is the contract
// `mdsmith fix` relies on; the runner calls Fix only after Check
// reports the index is stale.
func TestFix_WritesIndexSideOutput(t *testing.T) {
	dir := t.TempDir()
	src := "# Title\n\n## Goal\n\n## Tasks\n"
	docPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(docPath, []byte(src), 0o644))

	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"inline-schema": map[string]any{
			"index": map[string]any{
				"output":  "doc-index.json",
				"include": []any{"headings"},
			},
		},
	}))

	f, err := lint.NewFile(docPath, []byte(src))
	require.NoError(t, err)
	out := r.Fix(f)
	assert.Equal(t, src, string(out), "Fix must return source unchanged")

	idxData, err := os.ReadFile(filepath.Join(dir, "doc-index.json"))
	require.NoError(t, err, "index file should exist after Fix")

	var got map[string][]schema.IndexHeading
	require.NoError(t, json.Unmarshal(idxData, &got))
	require.Len(t, got["headings"], 3)
	assert.Equal(t, "title", got["headings"][0].Slug)
}

// TestCheck_ReportsStaleIndex verifies the read-only contract for
// `mdsmith check`: when the index is missing the rule emits a
// diagnostic so `mdsmith fix` is triggered, but Check itself does
// not write the file.
func TestCheck_ReportsStaleIndex(t *testing.T) {
	dir := t.TempDir()
	src := "# Title\n\n## Goal\n"
	docPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(docPath, []byte(src), 0o644))

	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"inline-schema": map[string]any{
			"index": map[string]any{
				"output":  "doc-index.json",
				"include": []any{"headings"},
			},
		},
	}))

	f, err := lint.NewFile(docPath, []byte(src))
	require.NoError(t, err)
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "missing")
	_, statErr := os.Stat(filepath.Join(dir, "doc-index.json"))
	require.Error(t, statErr, "Check must not write the index file")
}
