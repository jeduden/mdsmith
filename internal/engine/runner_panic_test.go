package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// panicRule is a test rule whose Check panics when a file's path
// contains a given substring (trigger), so the test can co-locate a
// hostile file next to normal files in the same run.
type panicRule struct {
	id      string
	name    string
	msg     string
	trigger string // path substring that causes the panic; empty = always
}

func (r *panicRule) ID() string       { return r.id }
func (r *panicRule) Name() string     { return r.name }
func (r *panicRule) Category() string { return "test" }
func (r *panicRule) Check(f *lint.File) []lint.Diagnostic {
	if r.trigger == "" || strings.Contains(f.Path, r.trigger) {
		panic(r.msg)
	}
	return nil
}

// TestWorkerPanicProducesInternalErrorDiagnostic verifies that a rule
// panic inside a parallel worker goroutine is caught by the deferred
// recover, converted into an InternalError diagnostic for the affected
// file, and does not prevent other files from being linted normally.
//
// Three files are used: the first and last are linted by a silent rule
// (no diagnostics); the middle file triggers the panicking rule. After
// the run:
//   - The panicking file's outcome must contain exactly one diagnostic
//     with RuleID "internal-panic" and Severity lint.Error.
//   - That diagnostic's message must contain the panic value and a
//     goroutine stack trace (the "goroutine" keyword emitted by
//     runtime/debug.Stack).
//   - The other two files must produce no diagnostics and no errors.
func TestWorkerPanicProducesInternalErrorDiagnostic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	writeFile := func(name, body string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
		return p
	}

	normalA := writeFile("normal_a.md", "# Normal A\n\nSome prose.\n")
	hostile := writeFile("hostile.md", "# Hostile\n\nAttacker-controlled content.\n")
	normalB := writeFile("normal_b.md", "# Normal B\n\nSome prose.\n")

	const panicMsg = "injected rule panic"

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"panic-rule":  {Enabled: true},
			"silent-rule": {Enabled: true},
		},
	}

	runner := &Runner{
		Config: cfg,
		Rules: []rule.Rule{
			// panic-rule only panics on files containing "hostile" in their path.
			&panicRule{id: "MDS998", name: "panic-rule", msg: panicMsg, trigger: "hostile"},
			&silentRule{id: "MDS997", name: "silent-rule"},
		},
		Concurrency:          3, // force parallel workers
		IntraFileConcurrency: 1, // keep rules serial so the panic stays on lintFile's goroutine
	}

	paths := []string{normalA, hostile, normalB}
	res := runner.Run(paths)

	// No run-level errors: the panic is converted to a diagnostic, not an error.
	assert.Empty(t, res.Errors, "run must produce no errors; panic must become a diagnostic")

	// Exactly one diagnostic: the InternalError for the hostile file.
	// The silent rule emits nothing; the panic rule is caught before it
	// can return a diagnostic, and the other two files proceed normally.
	require.Len(t, res.Diagnostics, 1,
		"expected exactly one InternalError diagnostic, got %d: %v",
		len(res.Diagnostics), formatDiags(res.Diagnostics))

	d := res.Diagnostics[0]
	assert.Equal(t, hostile, d.File,
		"the InternalError diagnostic must point at the panicking file")
	assert.Equal(t, "internal-panic", d.RuleID,
		"InternalError diagnostic must carry RuleID 'internal-panic'")
	assert.Equal(t, lint.Error, d.Severity,
		"InternalError diagnostic must have Error severity")
	assert.Contains(t, d.Message, panicMsg,
		"panic value must appear in the diagnostic message")
	assert.Contains(t, d.Message, "goroutine",
		"stack trace (runtime/debug.Stack) must appear in the diagnostic message")
}

// TestWorkerPanicSequentialPathAlsoCaught verifies that the same
// recover() logic applies on the sequential (workers==1) path:
// lintFile itself is wrapped so a panic in any call — serial or
// parallel — produces an InternalError diagnostic.
func TestWorkerPanicSequentialPathAlsoCaught(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	hostile := filepath.Join(dir, "hostile.md")
	require.NoError(t, os.WriteFile(hostile, []byte("# Hostile\n"), 0o644))

	const panicMsg = "sequential panic"

	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"panic-rule": {Enabled: true},
		},
	}

	runner := &Runner{
		Config:               cfg,
		Rules:                []rule.Rule{&panicRule{id: "MDS998", name: "panic-rule", msg: panicMsg, trigger: "hostile"}},
		Concurrency:          1, // force the sequential file-level branch
		IntraFileConcurrency: 1, // keep rules serial so the panic stays on lintFile's goroutine
	}

	res := runner.Run([]string{hostile})

	assert.Empty(t, res.Errors)
	require.Len(t, res.Diagnostics, 1)
	d := res.Diagnostics[0]
	assert.Equal(t, "internal-panic", d.RuleID)
	assert.Equal(t, lint.Error, d.Severity)
	assert.Contains(t, d.Message, panicMsg)
	assert.Contains(t, d.Message, "goroutine")
}

func formatDiags(diags []lint.Diagnostic) string {
	var sb strings.Builder
	for _, d := range diags {
		fmt.Fprintf(&sb, "[%s:%d %s %s] %s\n", d.File, d.Line, d.RuleID, d.Severity, d.Message)
	}
	return sb.String()
}
