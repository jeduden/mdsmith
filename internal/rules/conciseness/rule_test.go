package conciseness

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
)

func newDefaultRule(t *testing.T) *Rule {
	t.Helper()
	r := &Rule{}
	if err := r.ApplySettings(r.DefaultSettings()); err != nil {
		t.Fatalf("apply default settings: %v", err)
	}
	return r
}

func TestCheck_VerboseParagraphFlagged(t *testing.T) {
	src := []byte("# Status Update\n\n" +
		"In order to make sure that we are all on the same page, " +
		"it is important to note that we might possibly make " +
		"changes in most cases if the timeline shifts again.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := newDefaultRule(t)
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	d := diags[0]
	if d.RuleID != "MDS026" {
		t.Errorf("expected rule ID MDS026, got %s", d.RuleID)
	}
	if d.RuleName != "conciseness" {
		t.Errorf("expected rule name conciseness, got %s", d.RuleName)
	}
	if d.Line != 3 {
		t.Errorf("expected line 3, got %d", d.Line)
	}
	if d.Column != 1 {
		t.Errorf("expected column 1, got %d", d.Column)
	}
	if !strings.Contains(d.Message, "paragraph conciseness score too low") {
		t.Errorf("unexpected message: %s", d.Message)
	}
	if !strings.Contains(d.Message, "< 55.0") {
		t.Errorf("expected threshold in message, got %s", d.Message)
	}
}

func TestCheck_TechnicalParagraphPasses(t *testing.T) {
	src := []byte("# Architecture\n\n" +
		"Shard leaders persist monotonic commit indices, reject " +
		"stale lease epochs, and replicate snapshots with bounded " +
		"retries across every region before applying writes.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := newDefaultRule(t)
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %v", len(diags), diags)
	}
}

func TestCheck_VerboseButReadableCanPassWithLowerMinScore(t *testing.T) {
	src := []byte("# Rollout\n\n" +
		"We should update the onboarding guide so new contributors " +
		"can quickly find build steps, understand the release " +
		"checklist, and avoid common pitfalls without asking in chat.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := newDefaultRule(t)
	r.MinScore = 45
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics with lower min-score, got %d", len(diags))
	}
}

func TestCheck_MinWordsSkipsParagraph(t *testing.T) {
	src := []byte("# Note\n\nBasically this change is very simple and maybe helps a bit.\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := newDefaultRule(t)
	r.MinWords = 30
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for short paragraph, got %d", len(diags))
	}
}

func TestCheck_TableSkipped(t *testing.T) {
	src := []byte("# Metrics\n\n" +
		"| Field | Value |\n" +
		"|-------|-------|\n" +
		"| in order to | maybe very |\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := newDefaultRule(t)
	r.MinWords = 1
	r.MinScore = 99
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for table content, got %d", len(diags))
	}
}

func TestScore_VerboseLowerThanTechnical(t *testing.T) {
	r := newDefaultRule(t)

	verbose := "In order to make sure that we are all on the same " +
		"page, it is important to note that we might possibly " +
		"adjust this later."
	technical := "The replication controller validates lease epochs, " +
		"updates shard manifests, and atomically publishes commit " +
		"indices to downstream readers."

	verboseScore := r.score(normalizeWords(verbose), mdtext.CountWords(verbose)).score
	technicalScore := r.score(normalizeWords(technical), mdtext.CountWords(technical)).score
	if verboseScore >= technicalScore {
		t.Fatalf("expected verbose score < technical score, got %.1f >= %.1f", verboseScore, technicalScore)
	}
}

func TestApplySettings_Valid(t *testing.T) {
	r := newDefaultRule(t)
	err := r.ApplySettings(map[string]any{
		"min-score":             60.0,
		"min-words":             25,
		"min-content-ratio":     0.5,
		"filler-weight":         1.4,
		"hedge-weight":          1.1,
		"verbose-phrase-weight": 5.0,
		"content-weight":        1.5,
		"filler-words":          []any{"really", "very"},
		"hedge-words":           []any{"maybe", "perhaps"},
		"verbose-phrases":       []any{"in order to", "it is important to note"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.MinScore != 60.0 {
		t.Errorf("expected MinScore=60.0, got %.1f", r.MinScore)
	}
	if r.MinWords != 25 {
		t.Errorf("expected MinWords=25, got %d", r.MinWords)
	}
	if r.MinContentRatio != 0.5 {
		t.Errorf("expected MinContentRatio=0.5, got %.2f", r.MinContentRatio)
	}
	if len(r.FillerWords) != 2 {
		t.Errorf("expected 2 filler words, got %d", len(r.FillerWords))
	}
}

func TestApplySettings_InvalidMinScoreType(t *testing.T) {
	r := newDefaultRule(t)
	if err := r.ApplySettings(map[string]any{"min-score": "high"}); err == nil {
		t.Fatal("expected error for non-number min-score")
	}
}

func TestApplySettings_InvalidMinContentRatioRange(t *testing.T) {
	r := newDefaultRule(t)
	if err := r.ApplySettings(map[string]any{"min-content-ratio": 1.2}); err == nil {
		t.Fatal("expected error for out-of-range min-content-ratio")
	}
}

func TestApplySettings_InvalidWordList(t *testing.T) {
	r := newDefaultRule(t)
	if err := r.ApplySettings(map[string]any{"filler-words": "really"}); err == nil {
		t.Fatal("expected error for non-list filler-words")
	}
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := newDefaultRule(t)
	if err := r.ApplySettings(map[string]any{"unknown": true}); err == nil {
		t.Fatal("expected error for unknown setting")
	}
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["min-score"] != defaultMinScore {
		t.Errorf("expected min-score=%v, got %v", defaultMinScore, ds["min-score"])
	}
	if ds["min-words"] != defaultMinWords {
		t.Errorf("expected min-words=%v, got %v", defaultMinWords, ds["min-words"])
	}
	if _, ok := ds["filler-words"].([]string); !ok {
		t.Errorf("expected filler-words []string, got %T", ds["filler-words"])
	}
}

func TestIDNameCategory(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS026" {
		t.Errorf("expected MDS026, got %s", r.ID())
	}
	if r.Name() != "conciseness" {
		t.Errorf("expected conciseness, got %s", r.Name())
	}
	if r.Category() != "meta" {
		t.Errorf("expected meta, got %s", r.Category())
	}
}

func TestCountPhraseMatches(t *testing.T) {
	words := normalizeWords("in order to proceed we need to test in order to ship")
	phrases := compilePhraseWords([]string{"in order to"})
	if got := countPhraseMatches(words, phrases); got != 2 {
		t.Fatalf("expected 2 phrase matches, got %d", got)
	}
}
