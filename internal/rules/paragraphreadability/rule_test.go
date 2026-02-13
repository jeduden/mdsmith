package paragraphreadability

import (
	"strings"
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
)

// hardText returns a paragraph with long, complex words that yields
// a high ARI score (well above 14.0).
func hardText() string {
	return "The implementation of concurrent distributed systems " +
		"requires sophisticated understanding of fundamental " +
		"computational paradigms and synchronization mechanisms " +
		"that must guarantee linearizability across heterogeneous " +
		"processing environments and architectural configurations."
}

// easyText returns a simple paragraph that yields a low ARI score.
func easyText() string {
	return "The cat sat on the mat and the dog lay on the rug. " +
		"They were both very happy to be at home on a warm day."
}

func TestCheck_OverThreshold(t *testing.T) {
	src := []byte(hardText() + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{MaxGrade: 14.0, MinWords: 20, Grade: ARI}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	d := diags[0]
	if d.RuleID != "TM023" {
		t.Errorf("expected rule ID TM023, got %s", d.RuleID)
	}
	if d.RuleName != "paragraph-readability" {
		t.Errorf(
			"expected rule name paragraph-readability, got %s",
			d.RuleName,
		)
	}
	if d.Severity != lint.Warning {
		t.Errorf("expected severity warning, got %s", d.Severity)
	}
	if d.Column != 1 {
		t.Errorf("expected column 1, got %d", d.Column)
	}
	if !strings.Contains(d.Message, "> 14.0") {
		t.Errorf(
			"expected message to contain '> 14.0', got %q",
			d.Message,
		)
	}
}

func TestCheck_UnderThreshold(t *testing.T) {
	src := []byte(easyText() + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{MaxGrade: 14.0, MinWords: 20, Grade: ARI}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_ShortParagraphSkipped(t *testing.T) {
	// Fewer than 20 words: should be skipped regardless of ARI.
	src := []byte("One two three four five six seven eight.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	// Use a grade function that always returns a high score.
	alwaysHigh := func(_ string) float64 { return 99.0 }
	r := &Rule{MaxGrade: 14.0, MinWords: 20, Grade: alwaysHigh}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf(
			"expected 0 diagnostics for short paragraph, got %d",
			len(diags),
		)
	}
}

func TestCheck_InlineMarkupStripped(t *testing.T) {
	// The same hard text but with inline markup should still trigger.
	src := []byte(
		"The **implementation** of *concurrent* distributed " +
			"systems requires `sophisticated` understanding of " +
			"fundamental computational paradigms and " +
			"synchronization mechanisms that must guarantee " +
			"linearizability across heterogeneous processing " +
			"environments and architectural configurations.\n",
	)
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{MaxGrade: 14.0, MinWords: 20, Grade: ARI}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf(
			"expected 1 diagnostic for marked-up hard text, got %d",
			len(diags),
		)
	}
}

func TestCheck_DiagnosticLine(t *testing.T) {
	// The diagnostic should point to the correct line.
	src := []byte("# Heading\n\n" + hardText() + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{MaxGrade: 14.0, MinWords: 20, Grade: ARI}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Line != 3 {
		t.Errorf("expected line 3, got %d", diags[0].Line)
	}
}

func TestCheck_NilGradeUsesARI(t *testing.T) {
	src := []byte(hardText() + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{MaxGrade: 14.0, MinWords: 20, Grade: nil}
	diags := r.Check(f)
	// Hard text with ARI should trigger.
	if len(diags) != 1 {
		t.Fatalf(
			"expected 1 diagnostic with nil Grade, got %d",
			len(diags),
		)
	}
}

// --- Configurable tests ---

func TestApplySettings_ValidMaxGrade(t *testing.T) {
	r := &Rule{MaxGrade: 14.0, MinWords: 20}
	err := r.ApplySettings(map[string]any{"max-grade": 10.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.MaxGrade != 10.0 {
		t.Errorf("expected MaxGrade=10.0, got %f", r.MaxGrade)
	}
}

func TestApplySettings_ValidMinWords(t *testing.T) {
	r := &Rule{MaxGrade: 14.0, MinWords: 20}
	err := r.ApplySettings(map[string]any{"min-words": 30})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.MinWords != 30 {
		t.Errorf("expected MinWords=30, got %d", r.MinWords)
	}
}

func TestApplySettings_InvalidMaxGradeType(t *testing.T) {
	r := &Rule{MaxGrade: 14.0, MinWords: 20}
	err := r.ApplySettings(map[string]any{"max-grade": "high"})
	if err == nil {
		t.Fatal("expected error for non-number max-grade")
	}
}

func TestApplySettings_InvalidMinWordsType(t *testing.T) {
	r := &Rule{MaxGrade: 14.0, MinWords: 20}
	err := r.ApplySettings(map[string]any{"min-words": "many"})
	if err == nil {
		t.Fatal("expected error for non-int min-words")
	}
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{MaxGrade: 14.0, MinWords: 20}
	err := r.ApplySettings(map[string]any{"unknown": true})
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["max-grade"] != 14.0 {
		t.Errorf("expected max-grade=14.0, got %v", ds["max-grade"])
	}
	if ds["min-words"] != 20 {
		t.Errorf("expected min-words=20, got %v", ds["min-words"])
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "TM023" {
		t.Errorf("expected TM023, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "paragraph-readability" {
		t.Errorf(
			"expected paragraph-readability, got %s", r.Name(),
		)
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() != "meta" {
		t.Errorf("expected meta, got %s", r.Category())
	}
}
