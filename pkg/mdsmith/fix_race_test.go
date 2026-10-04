package mdsmith

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSessionFixConcurrentWithCheckIsRaceFree pins that a Fix (or
// FixRule, or FixPaths) on a session may run while a Check on the same
// session clones its rule set. Check clones the session's shared rules
// per worker; the fix paths must not run those shared instances
// themselves, since a stateful rule such as include writes its own
// fields during Fix while the clone reads them. Run with -race.
func TestSessionFixConcurrentWithCheckIsRaceFree(t *testing.T) {
	root := t.TempDir()
	src := "# T\n\n<?include\nfile: part.md\n?>\nhello\n<?/include?>\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "part.md"), []byte("hello\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "doc.md"), []byte(src), 0o600))
	s, err := NewSession(SessionOptions{Workspace: OSWorkspace{Root: root}, Config: ConfigYAML("")})
	require.NoError(t, err)
	defer s.Dispose()

	fixers := []func(){
		func() { _, _ = s.Fix("doc.md", []byte(src)) },
		func() { _, _ = s.FixRule("doc.md", []byte(src), []string{"include"}) },
		func() { s.FixPaths([]string{filepath.Join(root, "doc.md")}, BatchOptions{DryRun: true}) },
	}
	var wg sync.WaitGroup
	for range 5 {
		for _, fix := range fixers {
			wg.Add(2)
			go func() { defer wg.Done(); fix() }()
			go func() { defer wg.Done(); _, _ = s.Check("doc.md", []byte(src)) }()
		}
	}
	wg.Wait()
}

// FixRule runs Fix only on the named rules, so only those need a
// private copy: cloneNamed copies them and passes every other rule
// through as the shared instance, which the fix path only reads (its
// name, category and settings) and never runs.
func TestCloneNamedCopiesOnlyNamedRules(t *testing.T) {
	s, err := NewSession(SessionOptions{Workspace: NewMemWorkspace(nil), Config: ConfigYAML("")})
	require.NoError(t, err)
	defer s.Dispose()

	got := cloneNamed(s.rules, []string{"include", "line-length"})
	require.Len(t, got, len(s.rules))
	cloned := 0
	for i, rl := range got {
		switch rl.Name() {
		case "include", "line-length":
			assert.NotSame(t, s.rules[i], rl, rl.Name())
			assert.Equal(t, s.rules[i], rl, rl.Name())
			cloned++
		default:
			assert.Same(t, s.rules[i], rl, rl.Name())
		}
	}
	assert.Equal(t, 2, cloned)
}
