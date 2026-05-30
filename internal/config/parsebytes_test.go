package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseBytesAppliesConvention verifies ParseBytes runs the same
// post-parse pipeline as Load: convention application, validation, and
// inline-kind tagging — but from an in-memory YAML string with no disk
// access. This is the path the WASM session takes for its configYAML.
func TestParseBytesAppliesConvention(t *testing.T) {
	cfg, err := ParseBytes([]byte("rules:\n  line-length:\n    max: 42\n"))
	require.NoError(t, err)
	require.NotNil(t, cfg)

	rc, ok := cfg.Rules["line-length"]
	require.True(t, ok, "line-length rule present")
	require.NotNil(t, rc.Settings)
	assert.EqualValues(t, 42, rc.Settings["max"])
}

// TestParseBytesEmpty yields a usable config from empty input — no
// rules section means defaults stay in force where the engine fills
// them in.
func TestParseBytesEmpty(t *testing.T) {
	cfg, err := ParseBytes([]byte(""))
	require.NoError(t, err)
	require.NotNil(t, cfg)
}

// TestParseBytesInlineKind verifies a kind declared inline in the YAML
// body survives ParseBytes (no disk .mdsmith/kinds needed).
func TestParseBytesInlineKind(t *testing.T) {
	yml := "kinds:\n  doc:\n    path-pattern: \"docs/**/*.md\"\n"
	cfg, err := ParseBytes([]byte(yml))
	require.NoError(t, err)
	_, ok := cfg.Kinds["doc"]
	assert.True(t, ok, "inline kind 'doc' present after ParseBytes")
}

// TestParseBytesInvalidYAML surfaces a parse error rather than a
// silently-empty config.
func TestParseBytesInvalidYAML(t *testing.T) {
	_, err := ParseBytes([]byte("rules: [unterminated"))
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "parsing"),
		"error mentions parsing: %v", err)
}
