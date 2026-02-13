package paragraphstructure

import (
	"strings"
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
)

func TestCheck_TooManySentences(t *testing.T) {
	// 8 sentences, default max is 6.
	src := []byte("One. Two. Three. Four. Five. Six. Seven. Eight.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %v", len(diags), diags)
	}
	d := diags[0]
	if d.RuleID != "TM024" {
		t.Errorf("expected rule ID TM024, got %s", d.RuleID)
	}
	if !strings.Contains(d.Message, "too many sentences") {
		t.Errorf("unexpected message: %s", d.Message)
	}
	if !strings.Contains(d.Message, "8 > 6") {
		t.Errorf("expected count in message, got: %s", d.Message)
	}
}

func TestCheck_UnderSentenceLimit(t *testing.T) {
	src := []byte("One. Two. Three.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %v", len(diags), diags)
	}
}

func TestCheck_SentenceTooLong(t *testing.T) {
	// Build a sentence with 45 words.
	words := make([]string, 45)
	for i := range words {
		words[i] = "word"
	}
	src := []byte(strings.Join(words, " ") + ".\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %v", len(diags), diags)
	}
	if !strings.Contains(diags[0].Message, "sentence too long") {
		t.Errorf("unexpected message: %s", diags[0].Message)
	}
	if !strings.Contains(diags[0].Message, "45 > 40") {
		t.Errorf("expected count in message, got: %s", diags[0].Message)
	}
}

func TestCheck_BothLimitsExceeded(t *testing.T) {
	// 8 sentences (exceeds max 6) and one sentence with 45 words (exceeds max 40).
	words := make([]string, 45)
	for i := range words {
		words[i] = "word"
	}
	longSent := strings.Join(words, " ") + "."
	src := []byte(longSent + " Two. Three. Four. Five. Six. Seven. Eight.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d: %v", len(diags), diags)
	}
	hasSentences := false
	hasWords := false
	for _, d := range diags {
		if strings.Contains(d.Message, "too many sentences") {
			hasSentences = true
		}
		if strings.Contains(d.Message, "sentence too long") {
			hasWords = true
		}
	}
	if !hasSentences {
		t.Error("missing 'too many sentences' diagnostic")
	}
	if !hasWords {
		t.Error("missing 'sentence too long' diagnostic")
	}
}

func TestCheck_CustomSettings(t *testing.T) {
	src := []byte("One. Two. Three. Four.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{MaxSentences: 3, MaxWords: 40}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %v", len(diags), diags)
	}
	if !strings.Contains(diags[0].Message, "4 > 3") {
		t.Errorf("expected custom limit in message, got: %s", diags[0].Message)
	}
}

func TestCheck_ShortParagraph(t *testing.T) {
	src := []byte("Short.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %v", len(diags), diags)
	}
}

func TestCheck_DiagnosticFields(t *testing.T) {
	src := []byte("One. Two. Three. Four. Five. Six. Seven. Eight.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	d := diags[0]
	if d.Line != 1 {
		t.Errorf("expected line 1, got %d", d.Line)
	}
	if d.Column != 1 {
		t.Errorf("expected column 1, got %d", d.Column)
	}
	if d.RuleName != "paragraph-structure" {
		t.Errorf("expected rule name paragraph-structure, got %s", d.RuleName)
	}
	if d.Severity != lint.Warning {
		t.Errorf("expected severity warning, got %s", d.Severity)
	}
}

func TestApplySettings_Valid(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{
		"max-sentences": 10,
		"max-words":     50,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.MaxSentences != 10 {
		t.Errorf("expected MaxSentences=10, got %d", r.MaxSentences)
	}
	if r.MaxWords != 50 {
		t.Errorf("expected MaxWords=50, got %d", r.MaxWords)
	}
}

func TestApplySettings_InvalidType(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{"max-sentences": "not-a-number"})
	if err == nil {
		t.Fatal("expected error for non-int max-sentences")
	}
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{MaxSentences: 6, MaxWords: 40}
	err := r.ApplySettings(map[string]any{"unknown": 1})
	if err == nil {
		t.Fatal("expected error for unknown setting")
	}
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["max-sentences"] != 6 {
		t.Errorf("expected max-sentences=6, got %v", ds["max-sentences"])
	}
	if ds["max-words"] != 40 {
		t.Errorf("expected max-words=40, got %v", ds["max-words"])
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "TM024" {
		t.Errorf("expected TM024, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "paragraph-structure" {
		t.Errorf("expected paragraph-structure, got %s", r.Name())
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() != "meta" {
		t.Errorf("expected meta, got %s", r.Category())
	}
}
