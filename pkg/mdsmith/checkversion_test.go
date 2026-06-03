package mdsmith

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// hasEngineRule reports whether any engine diagnostic carries ruleID.
func hasEngineRule(diags []lint.Diagnostic, ruleID string) bool {
	for _, d := range diags {
		if d.RuleID == ruleID {
			return true
		}
	}
	return false
}

// TestCheckVersionReflectsNewText is the LSP-facing contract: a
// version-aware Check parses (and caches by version), and a later Check
// at a new version on edited bytes reflects the post-edit text — the
// per-keystroke path plan 216's parse cache backs.
func TestCheckVersionReflectsNewText(t *testing.T) {
	s := newTestSession(t, "", nil)

	clean := []byte("# Hi\n\nclean line\n")
	if res := s.CheckVersion("a.md", clean, 1); hasEngineRule(res.Diagnostics, "MDS006") {
		t.Fatalf("CheckVersion v1: clean source should have no MDS006, got %+v", res.Diagnostics)
	}

	dirty := []byte("# Hi\n\ndirty line   \n")
	res := s.CheckVersion("a.md", dirty, 2)
	if len(res.Errors) != 0 {
		t.Fatalf("CheckVersion v2: unexpected errors %v", res.Errors)
	}
	if !hasEngineRule(res.Diagnostics, "MDS006") {
		t.Fatalf("CheckVersion v2: expected MDS006 for trailing space, got %+v", res.Diagnostics)
	}
}

// TestCheckVersionReusesParseAtSameVersion verifies the version-keyed
// parse cache: a second CheckVersion at the same (uri, version) reuses
// the parsed file rather than re-parsing.
func TestCheckVersionReusesParseAtSameVersion(t *testing.T) {
	s := newTestSession(t, "", nil)
	src := []byte("# Hi\n\nsome body line here\n")

	s.CheckVersion("a.md", src, 1)
	hitsBefore := s.parseCacheHits()
	s.CheckVersion("a.md", src, 1)
	if s.parseCacheHits() <= hitsBefore {
		t.Fatalf("expected a parse-cache hit on the second CheckVersion at the same version")
	}
}

// TestCheckVersionCrossFileSeesOverlay verifies the version path reads
// cross-file content through the session workspace: a catalog over a
// MemWorkspace file projects that file's summary. This is the seam the
// LSP buffer overlay rides on (footgun 3).
func TestCheckVersionCrossFileSeesOverlay(t *testing.T) {
	files := map[string][]byte{
		"docs/one.md": []byte("---\nsummary: First\n---\n# One\n\nBody paragraph.\n"),
	}
	s := newTestSession(t, "", files)
	// An index whose catalog body is empty and out of date. Check returns
	// MDS019, proving the cross-file read ran.
	index := []byte("# Index\n\n<?catalog\nglob:\n  - \"docs/*.md\"\n" +
		"row: \"- [{summary}](docs/{filename})\"\n?>\n<?/catalog?>\n")

	res := s.CheckVersion("index.md", index, 1)
	if !hasEngineRule(res.Diagnostics, "MDS019") {
		t.Fatalf("CheckVersion: expected MDS019 (stale catalog) proving cross-file read, got %+v", res.Diagnostics)
	}
}
