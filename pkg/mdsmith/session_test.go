package mdsmith

import (
	"sort"
	"strings"
	"testing"
)

// newTestSession builds a session over an in-memory workspace seeded
// with files and an inline config string.
func newTestSession(t *testing.T, configYAML string, files map[string][]byte) *Session {
	t.Helper()
	s, err := NewSession(SessionOptions{
		Workspace: NewMemWorkspace(files),
		Config:    ConfigYAML(configYAML),
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(s.Dispose)
	return s
}

func TestSessionCheckFindsDiagnostic(t *testing.T) {
	s := newTestSession(t, "rules:\n  no-trailing-spaces: true\n", nil)

	diags, err := s.Check("a.md", []byte("# Title\n\ntrailing   \n"))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(diags) == 0 {
		t.Fatal("Check: expected at least one diagnostic for trailing spaces")
	}
	found := false
	for _, d := range diags {
		if d.RuleID == "MDS006" || strings.Contains(d.Message, "trailing") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Check: no trailing-space diagnostic in %+v", diags)
	}
}

func TestSessionCheckClean(t *testing.T) {
	s := newTestSession(t, "", nil)

	diags, err := s.Check("a.md", []byte("# Title\n\nClean body paragraph.\n"))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("Check: expected no diagnostics, got %+v", diags)
	}
}

func TestSessionFixRewritesSource(t *testing.T) {
	s := newTestSession(t, "rules:\n  no-trailing-spaces: true\n", nil)

	res, err := s.Fix("a.md", []byte("# Title\n\ntrailing   \n"))
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if strings.Contains(res.Source, "trailing   ") {
		t.Fatalf("Fix: trailing spaces not removed: %q", res.Source)
	}
	if !res.Changed {
		t.Fatal("Fix: expected Changed=true")
	}
}

func TestSessionFixNoChange(t *testing.T) {
	s := newTestSession(t, "", nil)

	src := "# Title\n\nClean body paragraph.\n"
	res, err := s.Fix("a.md", []byte(src))
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if res.Source != src {
		t.Fatalf("Fix: source changed unexpectedly: %q", res.Source)
	}
	if res.Changed {
		t.Fatal("Fix: expected Changed=false")
	}
}

func TestSessionKindsResolvesKind(t *testing.T) {
	cfg := "kinds:\n  doc:\n    path-pattern: \"docs/**/*.md\"\n" +
		"kind-assignment:\n  - glob: [\"docs/**/*.md\"]\n    kinds: [doc]\n"
	s := newTestSession(t, cfg, nil)

	res, err := s.Kinds("docs/guide.md")
	if err != nil {
		t.Fatalf("Kinds: %v", err)
	}
	hasDoc := false
	for _, k := range res.Kinds {
		if k.Name == "doc" {
			hasDoc = true
		}
	}
	if !hasDoc {
		t.Fatalf("Kinds: expected kind 'doc' for docs/guide.md, got %+v", res.Kinds)
	}
}

func TestSessionCapabilities(t *testing.T) {
	s := newTestSession(t, "", nil)

	caps := s.Capabilities()
	sort.Strings(caps)
	want := []string{"check", "fix", "kinds"}
	if len(caps) != len(want) {
		t.Fatalf("Capabilities = %v, want %v", caps, want)
	}
	for i := range want {
		if caps[i] != want[i] {
			t.Fatalf("Capabilities = %v, want %v", caps, want)
		}
	}
}

// TestSessionCheckReusesParse verifies repeated Check on the same
// (uri, source) reuses the cached parse rather than re-parsing. The
// session counts parses; the second call must not increment it.
func TestSessionCheckReusesParse(t *testing.T) {
	s := newTestSession(t, "", nil)
	src := []byte("# Title\n\nBody paragraph here.\n")

	if _, err := s.Check("a.md", src); err != nil {
		t.Fatalf("Check 1: %v", err)
	}
	first := s.parseCount()
	if _, err := s.Check("a.md", src); err != nil {
		t.Fatalf("Check 2: %v", err)
	}
	second := s.parseCount()
	if second != first {
		t.Fatalf("Check parse cache miss: parses went %d -> %d on identical (uri, source)", first, second)
	}
	if first == 0 {
		t.Fatal("expected at least one parse on the first Check")
	}
}

// TestSessionCheckReparsesOnChange verifies a changed source re-parses.
func TestSessionCheckReparsesOnChange(t *testing.T) {
	s := newTestSession(t, "", nil)

	if _, err := s.Check("a.md", []byte("# One\n\nBody paragraph here.\n")); err != nil {
		t.Fatalf("Check 1: %v", err)
	}
	first := s.parseCount()
	if _, err := s.Check("a.md", []byte("# Two\n\nDifferent body text now.\n")); err != nil {
		t.Fatalf("Check 2: %v", err)
	}
	if s.parseCount() == first {
		t.Fatal("Check: expected a re-parse when source changed")
	}
}

// TestSessionCheckCrossFileMemWorkspace verifies a catalog directive in
// a host file resolves its glob and reads front matter through the
// in-memory workspace — proving the engine drives cross-file rules off
// MemWorkspace with no disk.
func TestSessionCheckCrossFileMemWorkspace(t *testing.T) {
	files := map[string][]byte{
		"docs/one.md": []byte("---\nsummary: First doc\n---\n# One\n\nBody paragraph one here.\n"),
		"docs/two.md": []byte("---\nsummary: Second doc\n---\n# Two\n\nBody paragraph two here.\n"),
	}
	// Host index with a catalog that is OUT OF DATE (empty body): MDS019
	// should fire because the workspace holds two matching docs.
	host := []byte("# Index\n\n<?catalog\nglob:\n  - \"docs/*.md\"\n" +
		"row: \"- [{summary}](docs/{filename})\"\n?>\n<?/catalog?>\n")
	s := newTestSession(t, "", files)

	diags, err := s.Check("index.md", host)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	found := false
	for _, d := range diags {
		if d.RuleID == "MDS019" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Check: expected MDS019 (catalog out of date) reading docs/*.md from MemWorkspace, got %+v", diags)
	}
}

// TestSessionCheckResultIsolatedFromCache verifies the slice Check
// returns does not alias the cached slice, so a caller mutating its
// result cannot poison a later Check on the same (uri, source).
func TestSessionCheckResultIsolatedFromCache(t *testing.T) {
	s := newTestSession(t, "rules:\n  no-trailing-spaces: true\n", nil)
	src := []byte("# Title\n\ntrailing   \n")

	first, err := s.Check("a.md", src)
	if err != nil {
		t.Fatalf("Check 1: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("Check 1: expected at least one diagnostic")
	}
	// Mutate the caller's copy: clobber a field and attempt an in-place
	// grow into any spare capacity of the backing array.
	first[0].Message = "POISONED"
	_ = append(first[:len(first):len(first)], Diagnostic{Message: "EXTRA"})

	second, err := s.Check("a.md", src)
	if err != nil {
		t.Fatalf("Check 2: %v", err)
	}
	if len(second) == 0 {
		t.Fatal("Check 2: expected the cached diagnostic to survive")
	}
	if second[0].Message == "POISONED" {
		t.Fatal("Check 2: cached diagnostic was mutated by the first caller (slice aliases the cache)")
	}
}

// TestSessionCheckAfterDisposeNoPanic verifies that a Check racing or
// following Dispose does not panic with a nil-map write. Dispose nils
// the cache; Check must not blindly assign into it.
func TestSessionCheckAfterDisposeNoPanic(t *testing.T) {
	s := newTestSession(t, "", nil)
	s.Dispose()

	// Must not panic ("assignment to entry in nil map"). The result is
	// best-effort after Dispose; we only require no crash.
	if _, err := s.Check("a.md", []byte("# Title\n\nBody paragraph here.\n")); err != nil {
		t.Fatalf("Check after Dispose: unexpected error: %v", err)
	}
}
