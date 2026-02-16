package emptysectionbody

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

func TestCheck_EmptySectionAtEOF(t *testing.T) {
	src := []byte("# Doc\n\n## Empty\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}

	d := diags[0]
	if d.RuleID != "MDS030" {
		t.Errorf("expected MDS030, got %s", d.RuleID)
	}
	if d.Line != 3 {
		t.Errorf("expected line 3, got %d", d.Line)
	}
	if !strings.Contains(d.Message, "## Empty") {
		t.Errorf("expected heading text in diagnostic, got: %s", d.Message)
	}
}

func TestCheck_CommentOnlySection(t *testing.T) {
	src := []byte(
		"# Doc\n\n## Placeholder\n\n<!-- TODO: fill in later -->\n\n## Next\n\nBody.\n",
	)
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Line != 3 {
		t.Errorf("expected line 3, got %d", diags[0].Line)
	}
}

func TestCheck_AllowMarkerSkipsDiagnostic(t *testing.T) {
	src := []byte(
		"# Doc\n\n## Template Slot\n\n<!-- allow-empty-section -->\n",
	)
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_PrefixedMarkerDoesNotSkipByDefault(t *testing.T) {
	src := []byte(
		"# Doc\n\n## Template Slot\n\n<!-- mdsmith: allow-empty-section -->\n",
	)
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
}

func TestCheck_CustomAllowMarkerUsesExactString(t *testing.T) {
	src := []byte(
		"# Doc\n\n## Template Slot\n\n<!-- allow-empty-section -->\n",
	)
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{
		MinLevel:    defaultMinLevel,
		MaxLevel:    defaultMaxLevel,
		AllowMarker: "docs: intentionally-empty",
	}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
}

func TestCheck_ListContentIsMeaningful(t *testing.T) {
	src := []byte("# Doc\n\n## Steps\n\n- first\n- second\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_CodeContentIsMeaningful(t *testing.T) {
	src := []byte("# Doc\n\n## Example\n\n```go\nfmt.Println(\"hello\")\n```\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_TableContentIsMeaningful(t *testing.T) {
	src := []byte(
		"# Doc\n\n## Matrix\n\n| A | B |\n|---|---|\n| 1 | 2 |\n",
	)
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_NestedHeadingWithContentIsNotEmpty(t *testing.T) {
	src := []byte("# Doc\n\n## Parent\n\n### Child\n\nDetails.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_NestedHeadingWithoutContentReportsBothSections(t *testing.T) {
	src := []byte("# Doc\n\n## Parent\n\n### Child\n\n## Next\n\nBody.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{}
	diags := r.Check(f)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}
	if diags[0].Line != 3 {
		t.Errorf("expected first diagnostic at line 3, got %d", diags[0].Line)
	}
	if diags[1].Line != 5 {
		t.Errorf("expected second diagnostic at line 5, got %d", diags[1].Line)
	}
}

func TestCheck_MinLevelSkipsH2WhenSetTo3(t *testing.T) {
	src := []byte("# Doc\n\n## Parent\n\n### Child\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{MinLevel: 3, MaxLevel: 6, AllowMarker: defaultAllowMarker}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Line != 5 {
		t.Errorf("expected line 5, got %d", diags[0].Line)
	}
}

func TestApplySettings_Valid(t *testing.T) {
	r := &Rule{MinLevel: defaultMinLevel, MaxLevel: defaultMaxLevel, AllowMarker: defaultAllowMarker}
	err := r.ApplySettings(map[string]any{
		"min-level":    3,
		"max-level":    5,
		"allow-marker": "docs: intentionally-empty",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.MinLevel != 3 {
		t.Errorf("expected MinLevel=3, got %d", r.MinLevel)
	}
	if r.MaxLevel != 5 {
		t.Errorf("expected MaxLevel=5, got %d", r.MaxLevel)
	}
	if r.AllowMarker != "docs: intentionally-empty" {
		t.Errorf("unexpected allow marker: %s", r.AllowMarker)
	}
}

func TestApplySettings_InvalidType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-level": "two"})
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestApplySettings_AllowMarkerWhitespaceOnly(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"allow-marker": "   "})
	if err == nil {
		t.Fatal("expected error for whitespace-only allow-marker")
	}
}

func TestApplySettings_InvalidRange(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-level": 8})
	if err == nil {
		t.Fatal("expected error for invalid max-level")
	}
}

func TestApplySettings_MinGreaterThanMax(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"min-level": 5,
		"max-level": 3,
	})
	if err == nil {
		t.Fatal("expected error when min-level > max-level")
	}
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": true})
	if err == nil {
		t.Fatal("expected error for unknown setting")
	}
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["min-level"] != defaultMinLevel {
		t.Errorf("expected min-level=%d, got %v", defaultMinLevel, ds["min-level"])
	}
	if ds["max-level"] != defaultMaxLevel {
		t.Errorf("expected max-level=%d, got %v", defaultMaxLevel, ds["max-level"])
	}
	if ds["allow-marker"] != defaultAllowMarker {
		t.Errorf("expected allow-marker=%q, got %v", defaultAllowMarker, ds["allow-marker"])
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS030" {
		t.Errorf("expected MDS030, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "empty-section-body" {
		t.Errorf("expected empty-section-body, got %s", r.Name())
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() != "heading" {
		t.Errorf("expected heading, got %s", r.Category())
	}
}
