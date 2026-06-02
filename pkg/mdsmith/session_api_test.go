package mdsmith

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckCarriesRelatedLocations verifies the public Diagnostic
// carries RelatedLocations for a schema (MDS020) violation and that it
// JSON-marshals with the related_locations field — matching the CLI
// --format json shape so a WASM/Session host (e.g. the Obsidian
// plugin) reads one schema (plan 230).
func TestCheckCarriesRelatedLocations(t *testing.T) {
	cfg := ConfigYAML("kinds:\n" +
		"  task:\n" +
		"    schema:\n" +
		"      sections:\n" +
		"        - heading: \"Goal\"\n" +
		"        - heading: \"Tasks\"\n" +
		"kind-assignment:\n" +
		"  - glob: [\"*.md\"]\n" +
		"    kinds: [task]\n")
	s, err := NewSession(SessionOptions{Workspace: NewMemWorkspace(nil), Config: cfg})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()
	// The document has Goal but is missing the required Tasks section.
	diags, err := s.Check("a.md", []byte("# Title\n\n## Goal\n\nbody\n"))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	var found *Diagnostic
	for i := range diags {
		if diags[i].Rule == "MDS020" {
			found = &diags[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no MDS020 diagnostic among %d diagnostics", len(diags))
	}
	if len(found.RelatedLocations) != 1 {
		t.Fatalf("RelatedLocations = %d, want 1", len(found.RelatedLocations))
	}
	b, err := json.Marshal(*found)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), "related_locations") {
		t.Errorf("public JSON missing related_locations: %s", b)
	}
}

// --- ConfigSource: ConfigPath and ConfigYAML ---

// TestConfigPathLoadsFromDisk exercises the ConfigPath source end to
// end: configPath() reports the path, loadConfig() reads the file, and
// NewSession builds a working session from it.
func TestConfigPathLoadsFromDisk(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte("rules:\n  line-length:\n    max: 50\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	src := ConfigPath(cfgPath)
	if got := src.configPath(); got != cfgPath {
		t.Fatalf("ConfigPath.configPath() = %q, want %q", got, cfgPath)
	}
	if _, err := src.loadConfig(); err != nil {
		t.Fatalf("ConfigPath.loadConfig: %v", err)
	}

	s, err := NewSession(SessionOptions{Workspace: NewMemWorkspace(nil), Config: src})
	if err != nil {
		t.Fatalf("NewSession(ConfigPath): %v", err)
	}
	defer s.Dispose()
	if _, err := s.Check("a.md", []byte("# Title\n\nBody paragraph.\n")); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestConfigPathLoadError(t *testing.T) {
	// A nonexistent config path fails NewSession via ConfigPath.loadConfig.
	_, err := NewSession(SessionOptions{
		Workspace: NewMemWorkspace(nil),
		Config:    ConfigPath(filepath.Join(t.TempDir(), "does-not-exist.yml")),
	})
	if err == nil {
		t.Fatal("NewSession(ConfigPath nonexistent): want error, got nil")
	}
}

func TestConfigYAMLConfigPathEmpty(t *testing.T) {
	if got := ConfigYAML("x: 1").configPath(); got != "" {
		t.Fatalf("ConfigYAML.configPath() = %q, want empty", got)
	}
}

// --- NewSession defaults and error path ---

// TestNewSessionDefaults verifies a zero SessionOptions defaults the
// workspace to OSWorkspace{} and the config to ConfigYAML(""), and the
// resulting session lints.
func TestNewSessionDefaults(t *testing.T) {
	s, err := NewSession(SessionOptions{})
	if err != nil {
		t.Fatalf("NewSession{}: %v", err)
	}
	defer s.Dispose()
	if _, err := s.Check("x.md", []byte("# Title\n\nBody paragraph.\n")); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestNewSessionConfigError(t *testing.T) {
	_, err := NewSession(SessionOptions{
		Workspace: NewMemWorkspace(nil),
		Config:    ConfigYAML("rules: [this is not a map"),
	})
	if err == nil {
		t.Fatal("NewSession with invalid YAML: want error, got nil")
	}
}

// --- rootDirOf ---

func TestRootDirOf(t *testing.T) {
	if got := rootDirOf(OSWorkspace{Root: "/proj"}); got != "/proj" {
		t.Fatalf("rootDirOf(OSWorkspace{Root}) = %q, want /proj", got)
	}
	if got := rootDirOf(OSWorkspace{}); got != "" {
		t.Fatalf("rootDirOf(OSWorkspace{}) = %q, want empty", got)
	}
	if got := rootDirOf(NewMemWorkspace(nil)); got != "" {
		t.Fatalf("rootDirOf(MemWorkspace) = %q, want empty", got)
	}
}

// --- Kinds / frontMatterFor ---

// TestSessionKindsReadsFrontMatterKinds covers the file-present path of
// frontMatterFor: a `kinds:` front-matter list is read through the
// workspace and surfaces in the resolution.
func TestSessionKindsReadsFrontMatterKinds(t *testing.T) {
	files := map[string][]byte{
		"doc.md": []byte("---\nkinds: [guide]\n---\n# Doc\n\nBody paragraph.\n"),
	}
	s := newTestSession(t, "kinds:\n  guide: {}\n", files)

	res, err := s.Kinds("doc.md")
	if err != nil {
		t.Fatalf("Kinds: %v", err)
	}
	found := false
	for _, k := range res.Kinds {
		if k.Name == "guide" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Kinds(doc.md) = %+v, want it to include 'guide'", res.Kinds)
	}
}

// TestSessionKindsReadsFieldsPresent drives the NeedsFieldsForFile
// branch of frontMatterFor: a kind-assignment with `fields-present:`
// makes the session parse front-matter fields, and the kind applies
// because the file carries the required field.
func TestSessionKindsReadsFieldsPresent(t *testing.T) {
	cfg := "kinds:\n  titled: {}\n" +
		"kind-assignment:\n  - glob: [\"*.md\"]\n    kinds: [titled]\n" +
		"    fields-present: [title]\n"
	files := map[string][]byte{
		"a.md": []byte("---\ntitle: Hello\n---\n# A\n\nBody paragraph.\n"),
	}
	s := newTestSession(t, cfg, files)

	res, err := s.Kinds("a.md")
	if err != nil {
		t.Fatalf("Kinds: %v", err)
	}
	found := false
	for _, k := range res.Kinds {
		if k.Name == "titled" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Kinds(a.md) = %+v, want 'titled' assigned via fields-present", res.Kinds)
	}
}

// TestSessionKindsFrontMatterError covers frontMatterFor's error path:
// malformed front-matter YAML fails to parse and Kinds surfaces it.
func TestSessionKindsFrontMatterError(t *testing.T) {
	files := map[string][]byte{
		"bad.md": []byte("---\nkinds: [unterminated\n---\n# Bad\n\nBody.\n"),
	}
	s := newTestSession(t, "", files)
	if _, err := s.Kinds("bad.md"); err == nil {
		t.Fatal("Kinds: expected error for malformed front matter, got nil")
	}
}

// --- Invalidate ---

// TestInvalidateRewritesDependentFile is the plan-215 acceptance test:
// after Invalidate(uri, content) rewrites one workspace file, a
// dependent file's next operation sees the new bytes. The dependent is
// an index whose catalog projects the rewritten file's summary.
func TestInvalidateRewritesDependentFile(t *testing.T) {
	files := map[string][]byte{
		"docs/one.md": []byte("---\nsummary: First\n---\n# One\n\nBody paragraph.\n"),
	}
	s := newTestSession(t, "", files)
	index := []byte("# Index\n\n<?catalog\nglob:\n  - \"docs/*.md\"\n" +
		"row: \"- [{summary}](docs/{filename})\"\n?>\n<?/catalog?>\n")

	res1, err := s.Fix("index.md", index)
	if err != nil {
		t.Fatalf("Fix 1: %v", err)
	}
	if !strings.Contains(res1.Source, "First") {
		t.Fatalf("Fix 1: catalog body missing original summary 'First':\n%s", res1.Source)
	}

	// Rewrite the dependency through Invalidate, then re-fix the index.
	s.Invalidate("docs/one.md", []byte("---\nsummary: Renamed\n---\n# One\n\nBody paragraph.\n"))
	res2, err := s.Fix("index.md", index)
	if err != nil {
		t.Fatalf("Fix 2: %v", err)
	}
	if !strings.Contains(res2.Source, "Renamed") {
		t.Fatalf("Fix 2: catalog did not pick up the rewritten dependency:\n%s", res2.Source)
	}
}

// TestInvalidateDropsParseCache covers the no-content Invalidate path:
// it drops the cached parse (and deletes the MemWorkspace entry), so the
// next Check re-parses.
func TestInvalidateDropsParseCache(t *testing.T) {
	s := newTestSession(t, "", nil)
	src := []byte("# T\n\nBody paragraph here.\n")

	if _, err := s.Check("a.md", src); err != nil {
		t.Fatalf("Check 1: %v", err)
	}
	first := s.parseCount()
	if _, err := s.Check("a.md", src); err != nil {
		t.Fatalf("Check 2: %v", err)
	}
	if s.parseCount() != first {
		t.Fatal("expected a cache hit on the second Check")
	}

	s.Invalidate("a.md")
	if _, err := s.Check("a.md", src); err != nil {
		t.Fatalf("Check 3: %v", err)
	}
	if s.parseCount() <= first {
		t.Fatal("expected a re-parse after Invalidate dropped the cache")
	}
}

// TestInvalidateOSWorkspaceDropsCacheOnly covers the non-MemWorkspace
// branch of Invalidate: an OSWorkspace ignores the content argument and
// only the parse cache is dropped.
func TestInvalidateOSWorkspaceDropsCacheOnly(t *testing.T) {
	s, err := NewSession(SessionOptions{Workspace: OSWorkspace{}, Config: ConfigYAML("")})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()
	src := []byte("# T\n\nBody paragraph here.\n")

	if _, err := s.Check("a.md", src); err != nil {
		t.Fatalf("Check 1: %v", err)
	}
	first := s.parseCount()
	s.Invalidate("a.md", []byte("ignored by OSWorkspace"))
	if _, err := s.Check("a.md", src); err != nil {
		t.Fatalf("Check 2: %v", err)
	}
	if s.parseCount() <= first {
		t.Fatal("expected a re-parse after Invalidate on an OSWorkspace session")
	}
}

// --- engine error paths ---

// TestCheckFixFileTooLarge covers the engine error surfaced through both
// Check and Fix (and firstError's non-empty branch): a source over the
// session's MaxInputBytes is refused. maxBytes is set white-box because
// it is not a public SessionOptions field today.
func TestCheckFixFileTooLarge(t *testing.T) {
	s := newTestSession(t, "", nil)
	s.maxBytes = 16
	big := []byte("# A heading that is definitely longer than sixteen bytes\n\nBody.\n")

	if _, err := s.Check("a.md", big); err == nil {
		t.Fatal("Check: expected a 'file too large' error, got nil")
	}
	if _, err := s.Fix("a.md", big); err == nil {
		t.Fatal("Fix: expected a 'file too large' error, got nil")
	}
}

// TestKindsFieldsParseError covers frontMatterFor's ParseFrontMatterFields
// error branch: a sequence front matter (no `kinds:` key, so the kinds
// parse short-circuits cleanly) is not a mapping, so the field-presence
// parse the kind-assignment requires fails.
func TestKindsFieldsParseError(t *testing.T) {
	cfg := "kinds:\n  titled: {}\n" +
		"kind-assignment:\n  - glob: [\"*.md\"]\n    kinds: [titled]\n" +
		"    fields-present: [title]\n"
	files := map[string][]byte{
		"seq.md": []byte("---\n- alpha\n- beta\n---\n# Seq\n\nBody.\n"),
	}
	s := newTestSession(t, cfg, files)
	if _, err := s.Kinds("seq.md"); err == nil {
		t.Fatal("Kinds: expected a ParseFrontMatterFields error for sequence front matter, got nil")
	}
}

// TestInvalidateClearsDependentCache verifies Invalidating one file
// drops the cached Check result of OTHER files too. A cross-file rule
// (here a catalog) means any file can depend on the changed one, and
// the session tracks no dependency graph, so a stale dependent must not
// be served. Observed via the parse counter: the index must re-parse
// after the file its catalog projects is invalidated.
func TestInvalidateClearsDependentCache(t *testing.T) {
	files := map[string][]byte{
		"docs/one.md": []byte("---\nsummary: First\n---\n# One\n\nBody paragraph.\n"),
	}
	s := newTestSession(t, "", files)
	index := []byte("# Index\n\n<?catalog\nglob:\n  - \"docs/*.md\"\n" +
		"row: \"- [{summary}](docs/{filename})\"\n?>\n<?/catalog?>\n")

	if _, err := s.Check("index.md", index); err != nil {
		t.Fatalf("Check 1: %v", err)
	}
	first := s.parseCount()
	if _, err := s.Check("index.md", index); err != nil {
		t.Fatalf("Check 2: %v", err)
	}
	if s.parseCount() != first {
		t.Fatal("expected a cache hit on the second Check of the index")
	}

	// Invalidate a DIFFERENT file the index's catalog depends on.
	s.Invalidate("docs/one.md", []byte("---\nsummary: Second\n---\n# One\n\nBody paragraph.\n"))

	if _, err := s.Check("index.md", index); err != nil {
		t.Fatalf("Check 3: %v", err)
	}
	if s.parseCount() <= first {
		t.Fatal("index Check served a stale cached result after its catalog dependency changed via Invalidate")
	}
}
