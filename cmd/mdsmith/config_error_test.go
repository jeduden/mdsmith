package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintConfigError_PlainErrorKeepsPrefix(t *testing.T) {
	var buf bytes.Buffer
	printConfigError(&buf, errors.New("boom"))
	assert.Equal(t, "mdsmith: boom\n", buf.String())
}

func TestPrintConfigError_UnpositionedLoadErrorKeepsPrefix(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "missing.yml"))
	require.Error(t, err)
	var buf bytes.Buffer
	printConfigError(&buf, err)
	assert.Contains(t, buf.String(), "mdsmith: reading config file:")
}

func TestPrintConfigError_PositionedPathOutsideCwdStaysAbsolute(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yml")
	require.NoError(t, os.WriteFile(p, []byte("kinds:\n  a:\n    extends: b\n"), 0o644))
	_, err := config.Load(p)
	require.Error(t, err)
	t.Chdir(t.TempDir())
	var buf bytes.Buffer
	printConfigError(&buf, err)
	assert.Equal(t, p+`:3:5 config kind "a": extends references undeclared kind "b"`+"\n", buf.String())
}

func TestDisplayPath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	assert.Equal(t, filepath.Join("sub", "c.yml"), displayPath(filepath.Join(dir, "sub", "c.yml")))
	// A relative path cannot be made relative to the absolute cwd.
	assert.Equal(t, "c.yml", displayPath("c.yml"))
	parent := filepath.Join(filepath.Dir(dir), "c.yml")
	assert.Equal(t, parent, displayPath(parent))
}

func TestDisplayPath_GetwdError(t *testing.T) {
	chdirToRemoved(t)
	assert.Equal(t, "/x/c.yml", displayPath("/x/c.yml"))
}
