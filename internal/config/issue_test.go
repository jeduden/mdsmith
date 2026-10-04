package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// issuePos resolves err's Issue against the YAML source src the way the
// load boundary does: a pre-resolved line wins, else the key path.
func issuePos(t *testing.T, src string, err error) (int, int) {
	t.Helper()
	require.Error(t, err)
	var iss *Issue
	require.True(t, errors.As(err, &iss), "error is not a config Issue: %v", err)
	if iss.Line > 0 {
		return iss.Line, iss.Column
	}
	line, col, ok := newYAMLResolver([]byte(src)).Resolve(iss.Path)
	require.True(t, ok, "path %s did not resolve", iss.Path)
	return line, col
}

func TestIssueWrapKeepsCause(t *testing.T) {
	cause := errors.New("boom")
	iss := issueWrap(KeyPath{"kinds", "a"}, cause)
	assert.Equal(t, "boom", iss.Error())
	assert.ErrorIs(t, iss, cause)
	assert.Nil(t, issueWrap(KeyPath{"x"}, nil))
}

func TestKindValidationPositions(t *testing.T) {
	tests := []struct {
		name string
		src  string
		line int
		col  int
	}{
		{"path-pattern", `kinds:
  plan:
    path-pattern: "plan/[a"
`, 3, 5},
		{"extends undeclared", `kinds:
  plan:
    extends: nope
`, 3, 5},
		{"schema both inline and file", `kinds:
  plan:
    schema:
      frontmatter:
        id: int
    rules:
      required-structure:
        schema: plan/proto.md
`, 3, 5},
		{"schema file and inline-schema", `kinds:
  plan:
    rules:
      required-structure:
        schema: plan/proto.md
        inline-schema:
          frontmatter:
            id: int
`, 4, 7},
		{"kind-assignment undeclared", `kinds:
  plan: {}
kind-assignment:
  - glob: ["a.md"]
    kinds: [plan, ghost]
`, 5, 19},
		{"extends incompatible frontmatter", `kinds:
  base:
    schema:
      frontmatter:
        id: int
  plan:
    extends: base
    schema:
      frontmatter:
        id: string
`, 8, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseBytes([]byte(tt.src))
			line, col := issuePos(t, tt.src, err)
			assert.Equal(t, tt.line, line, "err: %v", err)
			assert.Equal(t, tt.col, col, "err: %v", err)
		})
	}
}

func TestForeignRegionValidationPositions(t *testing.T) {
	tests := []struct {
		name string
		src  string
		line int
		col  int
	}{
		{"top-level empty start", `foreign-regions:
  - start: "<!-- a -->"
    end: "<!-- /a -->"
  - start: " "
    end: "<!-- /b -->"
`, 4, 5},
		{"top-level empty end", `foreign-regions:
  - start: "<!-- a -->"
    end: ""
`, 3, 5},
		{"override start equals end", `overrides:
  - glob: ["a.md"]
  - glob: ["b.md"]
    foreign-regions:
      - start: "<!-- x -->"
        end: "<!-- x -->"
`, 5, 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseBytes([]byte(tt.src))
			line, col := issuePos(t, tt.src, err)
			assert.Equal(t, tt.line, line, "err: %v", err)
			assert.Equal(t, tt.col, col, "err: %v", err)
		})
	}
}

func TestDecoderIssuePositions(t *testing.T) {
	tests := []struct {
		name string
		src  string
		line int
		col  int
	}{
		{"rule scalar not bool", `rules:
  line-length: 42
`, 2, 16},
		{"rule sequence", `rules:
  line-length:
    - 1
`, 3, 5},
		{"kind rule scalar", `kinds:
  plan:
    rules:
      no-bare-urls: maybe
`, 4, 21},
		{"schema sequence", `kinds:
  plan:
    schema: [a]
`, 3, 13},
		{"schema bad name", `kinds:
  plan:
    schema: "bad name!"
`, 3, 13},
		{"schema empty name", `kinds:
  plan:
    schema: ""
`, 3, 13},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseBytes([]byte(tt.src))
			line, col := issuePos(t, tt.src, err)
			assert.Equal(t, tt.line, line, "err: %v", err)
			assert.Equal(t, tt.col, col, "err: %v", err)
		})
	}
}

func TestSidecarDecoderIssueNamesItsFile(t *testing.T) {
	for _, tc := range []struct{ dir, body string }{
		{"kinds", "rules:\n  line-length: 42\n"},
		{"conventions", "flavor: commonmark\nrules:\n  line-length: 42\n"},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			root := t.TempDir()
			cfgPath := filepath.Join(root, ".mdsmith.yml")
			require.NoError(t, os.WriteFile(cfgPath, []byte("{}\n"), 0o644))
			dir := filepath.Join(root, ".mdsmith", tc.dir)
			require.NoError(t, os.MkdirAll(dir, 0o755))
			side := filepath.Join(dir, "x.yml")
			require.NoError(t, os.WriteFile(side, []byte(tc.body), 0o644))

			_, err := Load(cfgPath)
			var iss *Issue
			require.True(t, errors.As(err, &iss), "err: %v", err)
			assert.Equal(t, side, iss.File)
			assert.Equal(t, strings.Count(tc.body, "\n"), iss.Line)
			assert.Equal(t, 16, iss.Column)
		})
	}
}

func TestAttachFileKeepsExisting(t *testing.T) {
	iss := &Issue{Message: "m", File: "first.yml"}
	err := attachFile("second.yml", fmt.Errorf("wrap: %w", iss))
	assert.Equal(t, "first.yml", iss.File)
	assert.EqualError(t, err, "wrap: m")
	plain := errors.New("plain")
	assert.Same(t, plain, attachFile("x.yml", plain))
}

func TestWordlistValidationPositions(t *testing.T) {
	tests := []struct {
		name string
		src  string
		line int
		col  int
	}{
		{"top-level unknown list", `rules:
  forbidden-text:
    lists:
      - ghost
`, 4, 9},
		{"kind lists not strings", `kinds:
  plan:
    rules:
      forbidden-text:
        lists: nope
`, 5, 9},
		{"override rule without lists support", `overrides:
  - glob: ["a.md"]
    rules:
      line-length:
        lists: [a]
`, 5, 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseBytes([]byte(tt.src))
			line, col := issuePos(t, tt.src, err)
			assert.Equal(t, tt.line, line, "err: %v", err)
			assert.Equal(t, tt.col, col, "err: %v", err)
		})
	}
}
