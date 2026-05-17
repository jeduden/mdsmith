package engine_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/engine"
	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Production rule set so the equivalence test exercises the
	// real stateful rules (include, catalog, …), not a stub subset.
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// statefulRule writes f.Path to a per-instance field during Check and
// reads it back into the diagnostic message. If a single instance is
// shared across goroutines running Check concurrently, the field is
// clobbered between the write and the read and the message no longer
// equals the file path (and `go test -race` flags the access). Correct
// per-worker rule isolation keeps every Check on a given instance
// sequential, so message == path always.
type statefulRule struct {
	tmp string
}

func (r *statefulRule) ID() string       { return "MDS900" }
func (r *statefulRule) Name() string     { return "stateful-mock" }
func (r *statefulRule) Category() string { return "test" }

func (r *statefulRule) Check(f *lint.File) []lint.Diagnostic {
	r.tmp = f.Path
	runtime.Gosched() // widen the race window if the instance is shared
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     1,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  r.tmp,
	}}
}

func writeCorpus(t *testing.T, n int) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, 0, n)
	for i := 0; i < n; i++ {
		p := filepath.Join(dir, fmt.Sprintf("doc%03d.md", i))
		body := fmt.Sprintf("# Document %d\n\nProse line one for document %d.\n\n"+
			"## Section\n\nSee [next](doc%03d.md) for more.\n\n"+
			"```go\nfunc f() int { return 0 }\n```\n", i, i, (i+1)%n)
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
		paths = append(paths, p)
	}
	return dir, paths
}

func TestResolveWorkers(t *testing.T) {
	gomax := runtime.GOMAXPROCS(0)
	cases := []struct {
		name        string
		concurrency int
		n           int
		want        int
	}{
		{"auto clamps to file count", 0, 3, min(gomax, 3)},
		{"auto uses gomaxprocs", 0, 10_000, gomax},
		{"explicit one is sequential", 1, 50, 1},
		{"explicit n honored", 4, 50, 4},
		{"explicit clamps to files", 8, 3, 3},
		{"negative means auto", -1, 10_000, gomax},
		{"zero files yields zero", 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, engine.ResolveWorkers(tc.concurrency, tc.n))
		})
	}
}

func TestRunner_ParallelEquivalenceFullRuleSet(t *testing.T) {
	dir, paths := writeCorpus(t, 40)
	cfg := config.Defaults()
	newRunner := func(c int) *engine.Runner {
		return &engine.Runner{
			Config:           cfg,
			Rules:            rule.All(),
			StripFrontMatter: true,
			RootDir:          dir,
			Concurrency:      c,
		}
	}

	seq := newRunner(1).Run(paths)
	for _, c := range []int{0, 2, 8, 16} {
		par := newRunner(c).Run(paths)
		require.Equal(t, seq.FilesChecked, par.FilesChecked,
			"FilesChecked mismatch at concurrency=%d", c)
		require.Equal(t, len(seq.Errors), len(par.Errors),
			"error count mismatch at concurrency=%d: %v vs %v", c, seq.Errors, par.Errors)
		require.Equal(t, len(seq.Diagnostics), len(par.Diagnostics),
			"diagnostic count mismatch at concurrency=%d", c)
		for i := range seq.Diagnostics {
			assert.Equal(t, seq.Diagnostics[i], par.Diagnostics[i],
				"diagnostic %d differs at concurrency=%d", i, c)
		}
	}
}

func TestRunner_ParallelStatefulRuleIsolated(t *testing.T) {
	_, paths := writeCorpus(t, 64)
	runner := &engine.Runner{
		Config: &config.Config{
			Rules: map[string]config.RuleCfg{"stateful-mock": {Enabled: true}},
		},
		Rules:       []rule.Rule{&statefulRule{}},
		Concurrency: 8,
	}
	res := runner.Run(paths)
	require.Empty(t, res.Errors)
	require.Len(t, res.Diagnostics, len(paths))
	// Each diagnostic's message must equal its own file path; a shared
	// instance under concurrent Check would cross-contaminate them.
	for _, d := range res.Diagnostics {
		assert.Equal(t, d.File, d.Message,
			"stateful rule leaked state across goroutines")
	}
}

// TestRunner_ParallelVerboseLogIsOrdered checks that a parallel run
// with an enabled logger emits each file's verbose block in input
// order with no cross-file interleaving — the buffered-then-merged
// path makes -v output deterministic despite the worker scheduling.
// Run under -race it also covers the enabled-logger concurrent path.
func TestRunner_ParallelVerboseLogIsOrdered(t *testing.T) {
	_, paths := writeCorpus(t, 24)
	var buf bytes.Buffer
	runner := &engine.Runner{
		Config:           config.Defaults(),
		Rules:            rule.All(),
		StripFrontMatter: true,
		RootDir:          filepath.Dir(paths[0]),
		Concurrency:      8,
		Logger:           &vlog.Logger{Enabled: true, W: &buf},
	}

	res := runner.Run(paths)
	require.Equal(t, len(paths), res.FilesChecked)

	// The sequence of "file:" lines must exactly match input order.
	var gotFiles []string
	for _, ln := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if rest, ok := strings.CutPrefix(ln, "file: "); ok {
			gotFiles = append(gotFiles, rest)
		}
	}
	require.Equal(t, paths, gotFiles,
		"verbose file: lines must appear in input order, no interleaving")

	// And every rule line must sit inside its own file's block: between
	// two consecutive file: markers there are only that file's lines.
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	curFileIdx := -1
	for _, ln := range lines {
		if rest, ok := strings.CutPrefix(ln, "file: "); ok {
			idx := indexOf(paths, rest)
			require.Equal(t, curFileIdx+1, idx, "file blocks out of order at %q", ln)
			curFileIdx = idx
			continue
		}
		assert.True(t, strings.HasPrefix(ln, "rule: "),
			"unexpected verbose line %q", ln)
	}
}

func indexOf(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}

// TestRunner_ParallelGitignoreCacheNoRace hammers the shared
// gitignore cache from many workers: a pre-fix lazy-init data race on
// Runner.gitignoreCache shows up here under `go test -race`.
func TestRunner_ParallelGitignoreCacheNoRace(t *testing.T) {
	dir, paths := writeCorpus(t, 64)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"),
		[]byte("ignored/\n"), 0o644))
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runner := &engine.Runner{
				Config:           config.Defaults(),
				Rules:            rule.All(),
				StripFrontMatter: true,
				RootDir:          dir,
				Concurrency:      8,
			}
			_ = runner.Run(paths)
		}()
	}
	wg.Wait()
}
