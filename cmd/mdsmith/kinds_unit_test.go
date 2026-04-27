package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kindsTestConfig is the YAML used by tests below: two kinds, one with
// a required-structure schema, plus a kind-assignment rule so resolve
// and why have a non-trivial chain to render.
const kindsTestConfig = `rules:
  line-length:
    max: 80
kinds:
  alpha:
    rules:
      line-length:
        max: 30
      required-structure:
        schema: schemas/alpha.yml
  beta:
    rules:
      line-length:
        max: 40
kind-assignment:
  - files: ["alpha/*"]
    kinds: ["alpha"]
`

// chdirToConfig writes the given config body into a temp dir, changes
// the process working directory there, and returns the dir. The
// original CWD is restored on cleanup.
func chdirToConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte(body), 0o644))
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
	require.NoError(t, os.Chdir(dir))
	return dir
}

// alwaysFailingWriter returns an error from every Write.
type alwaysFailingWriter struct{}

func (alwaysFailingWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("disk full")
}

// failOnBareNewlineWriter succeeds for every Write whose payload is not
// exactly "\n", and fails for the bare-newline payload that
// fmt.Fprintln(w) emits when called with no arguments.
type failOnBareNewlineWriter struct{}

func (failOnBareNewlineWriter) Write(p []byte) (int, error) {
	if len(p) == 1 && p[0] == '\n' {
		return 0, errors.New("disk full")
	}
	return len(p), nil
}

// --- runKinds dispatch ---

func TestRunKinds_NoArgsPrintsUsage(t *testing.T) {
	got := captureStderr(func() {
		assert.Equal(t, 0, runKinds(nil))
	})
	assert.Contains(t, got, "Usage: mdsmith kinds")
}

func TestRunKinds_HelpFlagPrintsUsage(t *testing.T) {
	got := captureStderr(func() {
		assert.Equal(t, 0, runKinds([]string{"--help"}))
	})
	assert.Contains(t, got, "Usage: mdsmith kinds")
}

// --- runKindsList ---

func TestRunKindsList_StdoutWriteFailureExitsTwo(t *testing.T) {
	chdirToConfig(t, kindsTestConfig)

	got := captureStderr(func() {
		assert.Equal(t, 2, runKindsList(alwaysFailingWriter{}, nil))
	})
	assert.Contains(t, got, "disk full")
}

func TestRunKindsList_JSONStdoutWriteFailureExitsTwo(t *testing.T) {
	chdirToConfig(t, kindsTestConfig)

	got := captureStderr(func() {
		assert.Equal(t, 2, runKindsList(alwaysFailingWriter{}, []string{"--json"}))
	})
	assert.Contains(t, got, "disk full")
}

// TestRunKindsList_SeparatorWriteFailureExitsTwo covers the
// per-iteration separator newline write inside runKindsList: a writer
// that only fails on bare-newline payloads lets the first kind's body
// print successfully, then errors on the separator before the second
// kind starts.
func TestRunKindsList_SeparatorWriteFailureExitsTwo(t *testing.T) {
	chdirToConfig(t, kindsTestConfig)

	got := captureStderr(func() {
		assert.Equal(t, 2, runKindsList(failOnBareNewlineWriter{}, nil))
	})
	assert.Contains(t, got, "disk full")
}

// TestRunKindsList_NoKindsExitsZero exercises the empty-config branch.
func TestRunKindsList_NoKindsExitsZero(t *testing.T) {
	chdirToConfig(t, "rules: {}\n")
	var buf bytes.Buffer
	stderr := captureStderr(func() {
		assert.Equal(t, 0, runKindsList(&buf, nil))
	})
	assert.Contains(t, stderr, "no kinds declared")
}

// --- runKindsShow ---

func TestRunKindsShow_StdoutWriteFailureExitsTwo(t *testing.T) {
	chdirToConfig(t, kindsTestConfig)

	got := captureStderr(func() {
		assert.Equal(t, 2, runKindsShow(alwaysFailingWriter{}, []string{"alpha"}))
	})
	assert.Contains(t, got, "disk full")
}

// --- runKindsPath ---

func TestRunKindsPath_StdoutWriteFailureExitsTwo(t *testing.T) {
	chdirToConfig(t, kindsTestConfig)

	got := captureStderr(func() {
		assert.Equal(t, 2, runKindsPath(alwaysFailingWriter{}, []string{"alpha"}))
	})
	assert.Contains(t, got, "disk full")
}

// --- kindSchemaPath ---

func TestKindSchemaPath_NoRequiredStructure(t *testing.T) {
	got := captureStderr(func() {
		_, code := kindSchemaPath(config.KindBody{}, "k")
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "does not configure required-structure")
}

func TestKindSchemaPath_RequiredStructureDisabled(t *testing.T) {
	body := config.KindBody{
		Rules: map[string]config.RuleCfg{
			"required-structure": {Enabled: false},
		},
	}
	got := captureStderr(func() {
		_, code := kindSchemaPath(body, "k")
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "does not configure required-structure")
}

// TestKindSchemaPath_NoSchemaKey covers the explicit !hasSchema branch
// where required-structure is enabled but the schema key is absent
// entirely (distinct from being present with an empty string).
func TestKindSchemaPath_NoSchemaKey(t *testing.T) {
	body := config.KindBody{
		Rules: map[string]config.RuleCfg{
			"required-structure": {Enabled: true},
		},
	}
	got := captureStderr(func() {
		_, code := kindSchemaPath(body, "k")
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "no required-structure.schema set")
}

func TestKindSchemaPath_NonStringSchema(t *testing.T) {
	body := config.KindBody{
		Rules: map[string]config.RuleCfg{
			"required-structure": {
				Enabled:  true,
				Settings: map[string]any{"schema": 42},
			},
		},
	}
	got := captureStderr(func() {
		_, code := kindSchemaPath(body, "k")
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "must be a string")
	assert.Contains(t, got, "int")
}

func TestKindSchemaPath_EmptyStringSchema(t *testing.T) {
	body := config.KindBody{
		Rules: map[string]config.RuleCfg{
			"required-structure": {
				Enabled:  true,
				Settings: map[string]any{"schema": ""},
			},
		},
	}
	got := captureStderr(func() {
		_, code := kindSchemaPath(body, "k")
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "no required-structure.schema set")
}

func TestKindSchemaPath_ValidString(t *testing.T) {
	body := config.KindBody{
		Rules: map[string]config.RuleCfg{
			"required-structure": {
				Enabled:  true,
				Settings: map[string]any{"schema": "schemas/foo.yml"},
			},
		},
	}
	schema, code := kindSchemaPath(body, "k")
	assert.Equal(t, 0, code)
	assert.Equal(t, "schemas/foo.yml", schema)
}

// --- runKindsResolve / runKindsWhy ---

// kindsTestFile writes a markdown file with a kinds: front matter and
// returns its absolute path. The chdir target stays on the test's CWD.
func kindsTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestRunKindsResolve_StdoutWriteFailureExitsTwo(t *testing.T) {
	dir := chdirToConfig(t, kindsTestConfig)
	path := kindsTestFile(t, dir, "x.md", "# Hello\n")

	got := captureStderr(func() {
		assert.Equal(t, 2, runKindsResolve(alwaysFailingWriter{}, []string{path}))
	})
	assert.Contains(t, got, "disk full")
}

func TestRunKindsWhy_StdoutWriteFailureExitsTwo(t *testing.T) {
	dir := chdirToConfig(t, kindsTestConfig)
	path := kindsTestFile(t, dir, "x.md", "# Hello\n")

	got := captureStderr(func() {
		assert.Equal(t, 2, runKindsWhy(alwaysFailingWriter{}, []string{path, "line-length"}))
	})
	assert.Contains(t, got, "disk full")
}

// TestRunKindsResolve_FrontMatterDisabledSkipsParsing covers the new
// branch in resolveFileFromCLI: when `front-matter: false` is set the
// helper neither parses nor validates the file's front-matter `kinds:`
// entries, so a kind name that would otherwise be rejected by
// ValidateFrontMatterKinds does not break the command.
func TestRunKindsResolve_FrontMatterDisabledSkipsParsing(t *testing.T) {
	cfgBody := `front-matter: false
rules:
  line-length:
    max: 80
kinds:
  alpha:
    rules:
      line-length:
        max: 30
`
	dir := chdirToConfig(t, cfgBody)
	// Front-matter declares an undeclared kind; when front-matter is
	// disabled this must be ignored rather than rejected.
	path := kindsTestFile(t, dir, "x.md", "---\nkinds: [phantom]\n---\n# Hi\n")

	var buf bytes.Buffer
	stderr := captureStderr(func() {
		assert.Equal(t, 0, runKindsResolve(&buf, []string{path}))
	})
	assert.Empty(t, stderr)
	assert.Contains(t, buf.String(), "file: ")
	// No effective kinds when front matter is ignored and no
	// kind-assignment rule matched.
	assert.Contains(t, buf.String(), "(none)")
}

// TestRunKindsResolve_FrontMatterDisabledMissingFileReportsError
// confirms the helper still reports a clear error when the file is
// missing while front matter is disabled.
func TestRunKindsResolve_FrontMatterDisabledMissingFileReportsError(t *testing.T) {
	cfgBody := "front-matter: false\nrules: {}\n"
	dir := chdirToConfig(t, cfgBody)

	missing := filepath.Join(dir, "no-such.md")
	got := captureStderr(func() {
		var buf bytes.Buffer
		assert.Equal(t, 2, runKindsResolve(&buf, []string{missing}))
	})
	assert.Contains(t, got, "reading ")
	assert.Contains(t, strings.ToLower(got), "no such")
}

// TestRunKindsResolve_FrontMatterDisabledDirectoryReportsError confirms
// that passing a directory as the file argument is rejected with an
// error when front-matter is disabled (os.Stat would have accepted it).
func TestRunKindsResolve_FrontMatterDisabledDirectoryReportsError(t *testing.T) {
	cfgBody := "front-matter: false\nrules: {}\n"
	dir := chdirToConfig(t, cfgBody)

	got := captureStderr(func() {
		var buf bytes.Buffer
		assert.Equal(t, 2, runKindsResolve(&buf, []string{dir}))
	})
	assert.Contains(t, got, "reading ")
}

// Compile-time assertion that the failing writers implement io.Writer.
var (
	_ io.Writer = alwaysFailingWriter{}
	_ io.Writer = failOnBareNewlineWriter{}
)
