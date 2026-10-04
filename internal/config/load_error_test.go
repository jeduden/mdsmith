package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeCfg writes body to dir/name and returns its path.
func writeCfg(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

func requireLoadError(t *testing.T, err error) *LoadError {
	t.Helper()
	var le *LoadError
	require.True(t, errors.As(err, &le), "not a LoadError: %v", err)
	return le
}

func TestLoad_SemanticErrorIsPositioned(t *testing.T) {
	dir := t.TempDir()
	p := writeCfg(t, dir, ".mdsmith.yml", `rules:
  line-length: true
foreign-regions:
  - start: "<!-- a -->"
    end: ""
`)
	_, err := Load(p)
	le := requireLoadError(t, err)
	assert.Equal(t, p, le.File)
	assert.Equal(t, 5, le.Line)
	assert.Equal(t, 5, le.Column)
	assert.Equal(t, "foreign-regions[0]: end marker must not be empty", le.Message)
	// The full wrapped text stays the error string.
	assert.Equal(t, "validating config: foreign-regions[0]: end marker must not be empty", err.Error())

	d := le.Diagnostic()
	assert.Equal(t, lint.Diagnostic{
		File: p, Line: 5, Column: 5, RuleID: "config", RuleName: "config",
		Severity: lint.Error, Message: le.Message,
	}, d)
}

func TestPositionedDiagnostic(t *testing.T) {
	positioned := &LoadError{
		File: "c.yml", Message: "m", Severity: lint.Error, Err: errors.New("w: m"), Line: 2, Column: 3,
	}
	d, ok := PositionedDiagnostic(fmt.Errorf("loading config: %w", positioned))
	require.True(t, ok)
	assert.Equal(t, positioned.Diagnostic(), d)

	for name, err := range map[string]error{
		"not a LoadError": errors.New("plain"),
		"no position":     &LoadError{File: "c.yml", Message: "m", Err: errors.New("m")},
		"no file":         &LoadError{Message: "m", Err: errors.New("m"), Line: 2, Column: 3},
	} {
		_, ok := PositionedDiagnostic(err)
		assert.False(t, ok, name)
	}
}

func TestLoad_SyntaxErrorPointsAtLine(t *testing.T) {
	dir := t.TempDir()
	p := writeCfg(t, dir, ".mdsmith.yml", "rules: {}\nfiles: [a.md]\n\tbad: x\n")
	_, err := Load(p)
	le := requireLoadError(t, err)
	assert.Equal(t, 3, le.Line)
	assert.Equal(t, 1, le.Column)
	assert.True(t, le.Positioned())
}

func TestLoad_TypeErrorPointsAtLine(t *testing.T) {
	dir := t.TempDir()
	p := writeCfg(t, dir, ".mdsmith.yml", "files: [a.md]\nfollow-symlinks: [x]\n")
	_, err := Load(p)
	le := requireLoadError(t, err)
	assert.Equal(t, 2, le.Line)
}

func TestLoad_UnpositionedErrorKeepsFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yml"))
	le := requireLoadError(t, err)
	assert.False(t, le.Positioned())
	assert.Equal(t, 0, le.Line)
	assert.Contains(t, le.Message, "reading config file")
}

func TestLoad_FileKindIssuePointsIntoKindFile(t *testing.T) {
	dir := t.TempDir()
	p := writeCfg(t, dir, ".mdsmith.yml", "kinds:\n  other: {}\n")
	kf := writeCfg(t, dir, ".mdsmith/kinds/plan.yml", "rules: {}\npath-pattern: \"plan/[a\"\n")
	_, err := Load(p)
	le := requireLoadError(t, err)
	assert.Equal(t, kf, le.File)
	assert.Equal(t, 2, le.Line)
	assert.Equal(t, 1, le.Column)
}

// A YAML type or unknown-key error in a kind file is positioned at the
// line the parser reports in that file, not left unpositioned.
func TestLoad_FileKindYAMLErrorPointsIntoKindFile(t *testing.T) {
	dir := t.TempDir()
	p := writeCfg(t, dir, ".mdsmith.yml", "rules: {}\n")
	kf := writeCfg(t, dir, ".mdsmith/kinds/plan.yml", "rules: {}\npath-pattern: [x]\n")
	_, err := Load(p)
	le := requireLoadError(t, err)
	assert.Equal(t, kf, le.File)
	assert.Equal(t, 2, le.Line)
	assert.Equal(t, 1, le.Column)
	assert.Contains(t, err.Error(), "loading kind files: parsing "+kf+": yaml: unmarshal errors")
}

func TestLoad_FileConventionYAMLErrorPointsIntoConventionFile(t *testing.T) {
	dir := t.TempDir()
	p := writeCfg(t, dir, ".mdsmith.yml", "rules: {}\n")
	cf := writeCfg(t, dir, ".mdsmith/conventions/mine.yml", "flavor: commonmark\nnope: 1\n")
	_, err := Load(p)
	le := requireLoadError(t, err)
	assert.Equal(t, cf, le.File)
	assert.Equal(t, 2, le.Line)
	assert.Equal(t, 1, le.Column)
}

func TestLoad_FileConventionIssuePointsIntoConventionFile(t *testing.T) {
	dir := t.TempDir()
	p := writeCfg(t, dir, ".mdsmith.yml", "rules: {}\n")
	cf := writeCfg(t, dir, ".mdsmith/conventions/mine.yml", "rules: {}\nflavor: nope\n")
	_, err := Load(p)
	le := requireLoadError(t, err)
	assert.Equal(t, cf, le.File)
	assert.Equal(t, 2, le.Line)
}

func TestSidecarPosition(t *testing.T) {
	dir := t.TempDir()
	kf := writeCfg(t, dir, "plan.yml", "rules: {}\nextends: x\n")
	cases := []struct {
		name      string
		iss       *Issue
		line, col int
	}{
		{"pre-resolved", &Issue{File: kf, Line: 7, Column: 3}, 7, 3},
		{"entry itself", &Issue{File: kf, Path: KeyPath{"kinds", "plan"}}, 1, 1},
		{"body key", &Issue{File: kf, Path: KeyPath{"kinds", "plan", "extends"}}, 2, 1},
		{"absent key", &Issue{File: kf, Path: KeyPath{"kinds", "plan", "nope"}}, 1, 1},
		{"unreadable file", &Issue{File: filepath.Join(dir, "gone.yml"), Path: KeyPath{"kinds", "p", "x"}}, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			line, col := sidecarPosition(tc.iss)
			assert.Equal(t, tc.line, line)
			assert.Equal(t, tc.col, col)
		})
	}
}

func TestYAMLErrorIssue(t *testing.T) {
	plain := errors.New("yaml: no line here")
	assert.Same(t, plain, yamlErrorIssue(plain))
	iss := &Issue{Message: "x", Line: 4}
	assert.Same(t, iss, yamlErrorIssue(iss))
	got := yamlErrorIssue(errors.New("yaml: unmarshal errors:\n  line 12: cannot unmarshal"))
	var out *Issue
	require.True(t, errors.As(got, &out))
	assert.Equal(t, 12, out.Line)
	assert.Equal(t, 1, out.Column)
}

func TestPositionError_UnresolvedPathStaysUnpositioned(t *testing.T) {
	err := positionError(issueAt(KeyPath{"nope"}, "m"), "c.yml",
		yamlResolverFor([]byte("rules: {}\n")))
	le := requireLoadError(t, err)
	assert.False(t, le.Positioned())
	assert.Equal(t, "c.yml", le.File)
	assert.Equal(t, "m", le.Message)
}

func TestParseBytes_ErrorIsPositionedWithoutFile(t *testing.T) {
	_, err := ParseBytes([]byte("kinds:\n  plan:\n    extends: ghost\n"))
	le := requireLoadError(t, err)
	assert.Equal(t, "", le.File)
	assert.Equal(t, 3, le.Line)
	assert.Equal(t, 5, le.Column)
}

func TestLoadErrorNil(t *testing.T) {
	assert.NoError(t, positionError(nil, "x", nil))
}
