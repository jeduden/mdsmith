package config

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyPathString(t *testing.T) {
	assert.Equal(t, "", KeyPath(nil).String())
	assert.Equal(t, "rules", KeyPath{"rules"}.String())
	assert.Equal(t, "overrides[0].foreign-regions[2]",
		KeyPath{"overrides", 0, "foreign-regions", 2}.String())
	assert.Equal(t, "[1]", KeyPath{1}.String())
}

func TestIssueErrorAndUnwrap(t *testing.T) {
	iss := issueAt(KeyPath{"kinds", "plan"}, "kind %q: bad", "plan")
	assert.Equal(t, `kind "plan": bad`, iss.Error())
	assert.Equal(t, lint.Error, iss.Severity)
	assert.Equal(t, KeyPath{"kinds", "plan"}, iss.Path)

	wrapped := fmt.Errorf("validating config: %w", iss)
	var got *Issue
	require.True(t, errors.As(wrapped, &got))
	assert.Same(t, iss, got)
}

func TestYAMLResolver(t *testing.T) {
	src := []byte(`rules:
  line-length:
    max: 80
overrides:
  - glob: ["a.md"]
    foreign-regions:
      - start: "<!-- x -->"
        end: "<!-- x -->"
`)
	r := newYAMLResolver(src)

	tests := []struct {
		name     string
		path     KeyPath
		line     int
		col      int
		resolved bool
	}{
		{"top key", KeyPath{"rules"}, 1, 1, true},
		{"nested key", KeyPath{"rules", "line-length", "max"}, 3, 5, true},
		{"sequence item", KeyPath{"overrides", 0}, 5, 5, true},
		{"deep", KeyPath{"overrides", 0, "foreign-regions", 0, "end"}, 8, 9, true},
		{"missing tail falls back to deepest", KeyPath{"rules", "nope", "x"}, 1, 1, true},
		{"index out of range falls back", KeyPath{"overrides", 3}, 4, 1, true},
		{"index into mapping falls back", KeyPath{"rules", 0}, 1, 1, true},
		{"key into sequence falls back", KeyPath{"overrides", "x"}, 4, 1, true},
		{"unknown root", KeyPath{"nope"}, 0, 0, false},
		{"unsupported element type", KeyPath{1.5}, 0, 0, false},
		{"empty path", nil, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, col, ok := r.Resolve(tt.path)
			assert.Equal(t, tt.resolved, ok)
			assert.Equal(t, tt.line, line)
			assert.Equal(t, tt.col, col)
		})
	}
}

func TestYAMLResolverUnparseable(t *testing.T) {
	for _, src := range []string{"", "a: [", "- just\n- a list\n"} {
		_, _, ok := newYAMLResolver([]byte(src)).Resolve(KeyPath{"a"})
		assert.False(t, ok, "src %q", src)
	}
}
