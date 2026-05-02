package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverFilesWithGeneratedContent(t *testing.T) {
	dir := t.TempDir()

	// Create test markdown files.
	files := map[string]string{
		"README.md":         "# Test\n\n<?catalog?>\n<?/catalog?>\n",
		"PLAN.md":           "# Plan\n\n<?include file: foo.md ?>\n",
		"doc.md":            "# Normal file\n\nNo directives here.\n",
		"guide.md":          "# Guide\n\n<?toc?>\n<?/toc?>\n",
		".hidden/secret.md": "# Hidden\n\n<?catalog?>\n",
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	// Test discovery.
	discovered := discoverFilesWithGeneratedContent(dir, 1024*1024)

	// Should find files with directives (but not hidden files).
	assert.Contains(t, discovered, "README.md")
	assert.Contains(t, discovered, "PLAN.md")
	assert.Contains(t, discovered, "guide.md")
	assert.NotContains(t, discovered, "doc.md")
	assert.NotContains(t, discovered, ".hidden/secret.md")
}

func TestDiscoverFilesWithGeneratedContent_EmptyRepo(t *testing.T) {
	dir := t.TempDir()

	// Empty repo with no markdown files.
	discovered := discoverFilesWithGeneratedContent(dir, 1024*1024)

	// Should fall back to defaults.
	assert.Equal(t, []string{"PLAN.md", "README.md"}, discovered)
}

func TestDiscoverFilesWithGeneratedContent_NoDirectives(t *testing.T) {
	dir := t.TempDir()

	// Create markdown files without directives.
	files := map[string]string{
		"README.md": "# Test\n\nNormal content.\n",
		"doc.md":    "# Doc\n\nMore content.\n",
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	// Test discovery.
	discovered := discoverFilesWithGeneratedContent(dir, 1024*1024)

	// Should fall back to defaults when no directives found.
	assert.Equal(t, []string{"PLAN.md", "README.md"}, discovered)
}
