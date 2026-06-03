package mdsmith

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/config"
)

// TestResolveSessionMaxBytesFallsBackOnBadSize pins the lenient branch
// in resolveSessionMaxBytes: an unparseable max-input-size does not wedge
// the session — it falls back to the default cap, mirroring the LSP's
// "a bad config does not stop linting" contract. The CLI surfaces the
// parse error separately at flag time.
func TestResolveSessionMaxBytesFallsBackOnBadSize(t *testing.T) {
	cfg := &config.Config{MaxInputSize: "not-a-size"}
	if got := resolveSessionMaxBytes(cfg); got != bytelimit.DefaultMaxInputBytes {
		t.Fatalf("resolveSessionMaxBytes(bad size) = %d, want default %d",
			got, bytelimit.DefaultMaxInputBytes)
	}
}

// TestSessionFixSurfacesPostFixCheckError covers Fix's re-lint error
// branch: fix expands an <?include?> whose body pulls in a neighbour that
// is itself within the cap, but the expanded document crosses
// max-input-size. fix.Source caps only its input, so it succeeds and
// returns the large bytes; the post-fix Check (which caps the buffer it
// lints) then reports "file too large", and Fix returns the fixed bytes
// alongside that error rather than swallowing it.
func TestSessionFixSurfacesPostFixCheckError(t *testing.T) {
	// Cap chosen so: the neighbour is readable (its size <= cap, or the
	// include rule's own byte-limited read would reject it and splice
	// nothing); the tiny host source is <= cap (so fix.Source accepts
	// it); but host + spliced neighbour exceeds cap (so the post-fix
	// Check, which caps the buffer it lints, reports "file too large").
	const cap = 300
	// A neighbour just under the cap. "filler word " is 12 bytes; 20 of
	// them plus the heading and trailing newline stay under 300 yet are
	// large enough that splicing them into the host crosses 300.
	neighbour := "# Included\n\n" + strings.Repeat("filler word ", 20) + "\n"
	if len(neighbour) > cap {
		t.Fatalf("precondition: neighbour (%d bytes) must be <= cap %d so the include can read it", len(neighbour), cap)
	}
	files := map[string][]byte{
		"included.md": []byte(neighbour),
	}
	s := newTestSession(t, "max-input-size: 300\n", files)

	host := []byte("# Host\n\n<?include\nfile: included.md\n?>\n<?/include?>\n")
	if len(host) > cap {
		t.Fatalf("precondition: host source (%d bytes) must be <= cap so fix.Source accepts it", len(host))
	}
	if len(host)+len(neighbour) <= cap {
		t.Fatalf("precondition: host (%d) + neighbour (%d) must exceed cap %d so the expanded doc trips Check",
			len(host), len(neighbour), cap)
	}

	res, err := s.Fix("host.md", host)
	if err == nil {
		t.Fatalf("Fix: expected a post-fix 'file too large' error from the expanded include, got nil (source:\n%s)",
			res.Source)
	}
	if !strings.Contains(err.Error(), "file too large") {
		t.Fatalf("Fix: error = %v, want it to mention 'file too large'", err)
	}
	// Fix still returns the rewritten (expanded) bytes alongside the error.
	if !strings.Contains(res.Source, "filler word") {
		t.Fatalf("Fix: expected the expanded include body in the returned Source, got:\n%s", res.Source)
	}
}

// TestSessionFixRuleSurfacesError covers FixRule's error branch: a source
// larger than max-input-size makes fix.SourceWithRules return the on-disk
// "file too large" shape, and FixRule propagates it (empty FixResult)
// rather than returning a half-fixed document.
func TestSessionFixRuleSurfacesError(t *testing.T) {
	s := newTestSession(t, "max-input-size: 32\n", nil)
	// Over the 32-byte cap, with a trailing-space violation the named
	// rule would otherwise fix.
	src := []byte("# Title\n\nthis line is over the cap   \n")
	if len(src) <= 32 {
		t.Fatalf("precondition: source (%d bytes) must exceed the cap", len(src))
	}

	res, err := s.FixRule("a.md", src, []string{"no-trailing-spaces"})
	if err == nil {
		t.Fatalf("FixRule: expected a 'file too large' error, got nil (source: %q)", res.Source)
	}
	if !strings.Contains(err.Error(), "file too large") {
		t.Fatalf("FixRule: error = %v, want it to mention 'file too large'", err)
	}
	if res.Source != "" {
		t.Fatalf("FixRule: expected empty FixResult on error, got source %q", res.Source)
	}
}

// TestSessionInvalidateWikilinksReresolves drives the LSP "watched file
// created" scenario through Session.InvalidateWikilinks: a [[Target]]
// wikilink is broken while Target.md is absent, then the file appears and
// InvalidateWikilinks drops the cached wikilink index so the next Check
// re-resolves the link and the broken-wikilink diagnostic clears.
func TestSessionInvalidateWikilinksReresolves(t *testing.T) {
	files := map[string][]byte{
		"doc.md": []byte("# Doc\n\nSee [[Target]] now.\n"),
	}
	s := newTestSession(t,
		"rules:\n  cross-file-reference-integrity:\n    wikilinks: true\n", files)
	src := files["doc.md"]

	hasBrokenWikilink := func(diags []Diagnostic) bool {
		for _, d := range diags {
			if d.RuleID == "MDS027" && strings.Contains(d.Message, "Target") {
				return true
			}
		}
		return false
	}

	diags, err := s.Check("doc.md", src)
	if err != nil {
		t.Fatalf("Check (missing target): %v", err)
	}
	if !hasBrokenWikilink(diags) {
		t.Fatalf("Check: expected a broken-wikilink diagnostic for [[Target]] before the file exists, got %+v", diags)
	}

	// The watched file is created. Without invalidation the cached
	// wikilink index still lacks Target, so the link stays "broken".
	s.Invalidate("Target.md", []byte("# Target\n"))
	s.InvalidateWikilinks()

	diags2, err := s.Check("doc.md", src)
	if err != nil {
		t.Fatalf("Check (target created): %v", err)
	}
	if hasBrokenWikilink(diags2) {
		t.Fatalf("Check: [[Target]] should resolve after Target.md is created and the index is invalidated, got %+v",
			diags2)
	}
}

// TestSessionAbsPathJoinsRelativeUnderRoot covers absPath's join branch:
// with a rooted OSWorkspace, a workspace-relative uri is resolved against
// the root so the cross-file read cache keys it the same absolute path
// the catalog/include rules compute.
func TestSessionAbsPathJoinsRelativeUnderRoot(t *testing.T) {
	root := t.TempDir()
	s, err := NewSession(SessionOptions{
		Workspace: OSWorkspace{Root: root},
		Config:    ConfigYAML(""),
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()

	got := s.absPath("docs/a.md")
	want := filepath.Join(root, "docs", "a.md")
	if got != want {
		t.Fatalf("absPath(docs/a.md) = %q, want %q", got, want)
	}

	// An absolute uri passes through unchanged even with a root set.
	abs := filepath.Join(root, "abs.md")
	if got := s.absPath(abs); got != abs {
		t.Fatalf("absPath(absolute) = %q, want passthrough %q", got, abs)
	}
}
