package concisenessscoring

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

func verboseParagraph() string {
	return "Basically, it seems that we are just trying to explain the " +
		"same idea in order to make it very clear, and it appears that " +
		"we are really saying very little new information overall."
}

func conciseParagraph() string {
	return "The release process validates changelog links, updates " +
		"version tags, and publishes checksums so reviewers can verify " +
		"artifacts before promoting a build."
}

func TestCheck_LowScore(t *testing.T) {
	src := []byte(verboseParagraph() + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{
		MinScore: 0.40,
		MinWords: 20,
	}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}

	d := diags[0]
	if d.RuleID != "MDS029" {
		t.Errorf("expected rule ID MDS029, got %s", d.RuleID)
	}
	if d.RuleName != "conciseness-scoring" {
		t.Errorf(
			"expected rule name conciseness-scoring, got %s",
			d.RuleName,
		)
	}
	if d.Severity != lint.Warning {
		t.Errorf("expected severity warning, got %s", d.Severity)
	}
	if !strings.Contains(d.Message, "conciseness score too low") {
		t.Errorf("unexpected message: %s", d.Message)
	}
	if !strings.Contains(d.Message, "target >=") {
		t.Errorf("expected target guidance in message, got: %s", d.Message)
	}
	if !strings.Contains(d.Message, "\"basically\"") {
		t.Errorf("expected example cue in message, got: %s", d.Message)
	}
}

func TestCheck_HighScore(t *testing.T) {
	src := []byte(conciseParagraph() + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{
		MinScore: 0.35,
		MinWords: 20,
	}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_ShortParagraphSkipped(t *testing.T) {
	src := []byte("This is short and intentionally skipped.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{
		MinScore: 0.95,
		MinWords: 20,
	}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_DiagnosticLine(t *testing.T) {
	src := []byte("# Heading\n\n" + verboseParagraph() + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{
		MinScore: 0.40,
		MinWords: 20,
	}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Line != 3 {
		t.Errorf("expected line 3, got %d", diags[0].Line)
	}
}

func TestCheck_TableSkipped(t *testing.T) {
	src := []byte(
		"| Setting | Value |\n" +
			"|---------|-------|\n" +
			"| alpha   | beta  |\n",
	)
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := &Rule{
		MinScore: 0.95,
		MinWords: 1,
	}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for table, got %d", len(diags))
	}
}

func TestApplySettings_Valid(t *testing.T) {
	r := &Rule{MinScore: defaultMinScore, MinWords: defaultMinWords}
	err := r.ApplySettings(map[string]any{
		"min-score":       0.5,
		"min-words":       30,
		"filler-words":    []any{"literally", "simply"},
		"hedge-phrases":   []any{"perhaps"},
		"verbose-phrases": []any{"with regard to"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.MinScore != 0.5 {
		t.Errorf("expected MinScore=0.5, got %.2f", r.MinScore)
	}
	if r.MinWords != 30 {
		t.Errorf("expected MinWords=30, got %d", r.MinWords)
	}
	if len(r.FillerWords) != 2 {
		t.Errorf("expected 2 filler words, got %d", len(r.FillerWords))
	}
}

func TestApplySettings_InvalidMinScoreType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-score": "high"})
	if err == nil {
		t.Fatal("expected error for non-number min-score")
	}
}

func TestApplySettings_InvalidMinScoreRange(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-score": 1.2})
	if err == nil {
		t.Fatal("expected error for out-of-range min-score")
	}
}

func TestApplySettings_InvalidListType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"filler-words": []any{"fine", 123},
	})
	if err == nil {
		t.Fatal("expected error for invalid filler-words")
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
	if ds["min-score"] != defaultMinScore {
		t.Errorf(
			"expected min-score=%.2f, got %v",
			defaultMinScore, ds["min-score"],
		)
	}
	if ds["min-words"] != defaultMinWords {
		t.Errorf(
			"expected min-words=%d, got %v",
			defaultMinWords, ds["min-words"],
		)
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS029" {
		t.Errorf("expected MDS029, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "conciseness-scoring" {
		t.Errorf("expected conciseness-scoring, got %s", r.Name())
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() != "meta" {
		t.Errorf("expected meta, got %s", r.Category())
	}
}
