package mdsmith

import (
	"fmt"
	"strings"
	"testing"
)

// benchDoc is a representative single Markdown file: a heading and a
// handful of body paragraphs, sized so the parse cost dominates the
// per-Check work the cache is meant to skip.
func benchDoc() []byte {
	var b strings.Builder
	b.WriteString("# Benchmark document\n\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "Paragraph %d with enough words to parse as real prose content here.\n\n", i)
	}
	return []byte(b.String())
}

// BenchmarkSessionCheckCold measures a cold Check: every iteration
// presents fresh content, so the parse cache always misses and the
// source is parsed and linted each time.
func BenchmarkSessionCheckCold(b *testing.B) {
	s := mustBenchSession(b)
	base := benchDoc()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Mutate one byte per iteration so the content hash differs
		// and the cache misses — the cold path.
		src := append([]byte(nil), base...)
		src = append(src, byte('a'+(i%26)))
		if _, err := s.Check("doc.md", src); err != nil {
			b.Fatalf("Check: %v", err)
		}
	}
}

// BenchmarkSessionCheckSteady measures the steady-state Check: the same
// (uri, source) every iteration, so after the first parse the cache
// serves every later call without re-parsing.
func BenchmarkSessionCheckSteady(b *testing.B) {
	s := mustBenchSession(b)
	src := benchDoc()
	if _, err := s.Check("doc.md", src); err != nil {
		b.Fatalf("warm-up Check: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Check("doc.md", src); err != nil {
			b.Fatalf("Check: %v", err)
		}
	}
}

// TestSessionSteadyUnderHalfCold asserts the parse-cache contract from
// the plan: repeated Check on the same source-hash reuses the parsed
// AST so steady-state is well under half the cold-start time. Run as a
// test (not just a benchmark) so it gates CI.
func TestSessionSteadyUnderHalfCold(t *testing.T) {
	cold := testing.Benchmark(BenchmarkSessionCheckCold)
	steady := testing.Benchmark(BenchmarkSessionCheckSteady)

	coldNs := cold.NsPerOp()
	steadyNs := steady.NsPerOp()
	t.Logf("cold=%dns/op steady=%dns/op ratio=%.3f", coldNs, steadyNs, float64(steadyNs)/float64(coldNs))

	if steadyNs*2 >= coldNs {
		t.Fatalf("steady-state Check not under half cold-start: cold=%dns steady=%dns", coldNs, steadyNs)
	}
}

// TestSessionCheckNoPerFileGlob asserts the MemWorkspace's linear Glob
// is never called during cross-file Check: the engine globs through
// the FS view, not Workspace.Glob, so the hot loop stays off the
// linear key scan. The plan calls for a bench fixture asserting no
// per-file Glob under MemWorkspace.
func TestSessionCheckNoPerFileGlob(t *testing.T) {
	files := map[string][]byte{
		"docs/one.md": []byte("---\nsummary: One\n---\n# One\n\nBody paragraph one here.\n"),
		"docs/two.md": []byte("---\nsummary: Two\n---\n# Two\n\nBody paragraph two here.\n"),
	}
	ws := NewMemWorkspace(files)
	s, err := NewSession(SessionOptions{Workspace: ws, Config: ConfigYAML("")})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()

	host := []byte("# Index\n\n<?catalog\nglob:\n  - \"docs/*.md\"\n" +
		"row: \"- [{summary}](docs/{filename})\"\n?>\n<?/catalog?>\n")

	for i := 0; i < 50; i++ {
		// Vary content so every Check is a cache miss and actually
		// runs the engine (and its cross-file catalog resolution).
		src := append([]byte(nil), host...)
		src = append(src, byte('a'+(i%26)), '\n')
		if _, err := s.Check("index.md", src); err != nil {
			t.Fatalf("Check %d: %v", i, err)
		}
	}

	if n := ws.GlobCalls(); n != 0 {
		t.Fatalf("MemWorkspace.Glob called %d times during cross-file "+
			"Check; the engine must glob through the FS view, not "+
			"Workspace.Glob", n)
	}
}

// mustBenchSession builds a default session over an empty MemWorkspace.
func mustBenchSession(tb testing.TB) *Session {
	tb.Helper()
	s, err := NewSession(SessionOptions{
		Workspace: NewMemWorkspace(nil),
		Config:    ConfigYAML(""),
	})
	if err != nil {
		tb.Fatalf("NewSession: %v", err)
	}
	tb.Cleanup(s.Dispose)
	return s
}
