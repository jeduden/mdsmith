package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunMergeCoverage(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.cov")
	b := filepath.Join(dir, "b.cov")
	out := filepath.Join(dir, "m.cov")
	require.NoError(t, os.WriteFile(a,
		[]byte("mode: atomic\nx.go:1.1,2.2 1 1\n"), 0o644))
	require.NoError(t, os.WriteFile(b,
		[]byte("mode: atomic\nx.go:1.1,2.2 1 0\n"), 0o644))

	assert.Equal(t, 0, run([]string{"merge-coverage", "-o", out, a, b}))
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(got), "x.go:1.1,2.2 1 1")

	// Missing -o or inputs is a usage error.
	assert.Equal(t, 2, run([]string{"merge-coverage"}))
	assert.Equal(t, 2, run([]string{"merge-coverage", "-o", out}))
	// A bad flag is a usage error too.
	assert.Equal(t, 2, run([]string{"merge-coverage", "--bogus"}))
	// A merge failure (missing input) is a runtime error.
	assert.Equal(t, 1, run([]string{
		"merge-coverage", "-o", out, filepath.Join(dir, "missing.cov"),
	}))
}
