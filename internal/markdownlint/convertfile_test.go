package markdownlint

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/rules"
)

func TestConvertFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".markdownlint.json")
	require.NoError(t, os.WriteFile(path,
		[]byte(`{"MD013": {"line_length": 120}, "MD024": {"siblings_only": true}}`), 0o644))

	data, notes, err := ConvertFile(path)
	require.NoError(t, err)

	assert.Contains(t, string(data), "max: 120")
	require.NotEmpty(t, notes)
	assert.Contains(t, notes[0], "siblings_only")
}

func TestConvertFile_ReadError(t *testing.T) {
	_, _, err := ConvertFile(filepath.Join(t.TempDir(), "nope.json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading")
	assert.Contains(t, err.Error(), "nope.json")
}

func TestConvertFile_ParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".markdownlintrc")
	require.NoError(t, os.WriteFile(path, []byte(`{"MD013" true}`), 0o644))

	_, _, err := ConvertFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".markdownlintrc")
	assert.Contains(t, err.Error(), "parsing markdownlint config")
}

func TestConvertFile_ConvertError(t *testing.T) {
	orig := listRules
	t.Cleanup(func() { listRules = orig })
	listRules = func() ([]rules.RuleInfo, error) { return nil, errors.New("boom") }

	dir := t.TempDir()
	path := filepath.Join(dir, ".markdownlint.yaml")
	require.NoError(t, os.WriteFile(path, []byte("MD041: false\n"), 0o644))

	_, _, err := ConvertFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading rule metadata")
}
