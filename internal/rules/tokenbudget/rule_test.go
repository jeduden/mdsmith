package tokenbudget

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

func mustFile(t *testing.T, path, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile(path, []byte(src))
	require.NoError(t, err)
	return f
}

func TestCheck_HeuristicBudgetExceeded(t *testing.T) {
	f := mustFile(t, "test.md", "one two three four five six")
	r := &Rule{Max: 3, Mode: "heuristic", TokensPerWord: 1.0}
	diags := r.Check(f)

	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	d := diags[0]
	if d.RuleID != "MDS028" {
		t.Errorf("expected rule ID MDS028, got %s", d.RuleID)
	}
	if d.RuleName != "token-budget" {
		t.Errorf("expected rule name token-budget, got %s", d.RuleName)
	}
	if d.Severity != lint.Warning {
		t.Errorf("expected severity warning, got %s", d.Severity)
	}
	if d.Line != 1 || d.Column != 1 {
		t.Errorf("expected location 1:1, got %d:%d", d.Line, d.Column)
	}
	require.Contains(t, d.Message, "token budget exceeded (6 > 3, mode=heuristic:tokens-per-word=1.00)",
		"message missing base info, got: %s", d.Message)
	require.Contains(t, d.Message, "~3 words over budget",
		"message should include words-over-budget estimate, got: %s", d.Message)
}

func TestCheck_HeuristicAtBudget_NoDiagnostic(t *testing.T) {
	f := mustFile(t, "test.md", "one two three four")
	r := &Rule{Max: 4, Mode: "heuristic", TokensPerWord: 1.0}
	if diags := r.Check(f); len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_TokenizerBudgetExceeded(t *testing.T) {
	f := mustFile(t, "test.md", "Alpha beta, gamma!")
	r := &Rule{
		Max:       1,
		Mode:      "tokenizer",
		Tokenizer: "builtin",
		Encoding:  "cl100k_base",
	}
	if diags := r.Check(f); len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	} else if !strings.Contains(diags[0].Message, "mode=tokenizer:builtin/cl100k_base") {
		t.Fatalf("expected tokenizer mode in diagnostic, got %q", diags[0].Message)
	}
}

func TestCheck_PerGlobBudget_LastMatchWins(t *testing.T) {
	f := mustFile(t, "docs/guide.md", "one two three four five six")
	r := &Rule{Max: 100, Mode: "heuristic", TokensPerWord: 1.0}
	if err := r.ApplySettings(map[string]any{
		"budgets": []any{
			map[string]any{"glob": "docs/*.md", "max": 10},
			map[string]any{"glob": "docs/guide.md", "max": 5},
		},
	}); err != nil {
		t.Fatalf("ApplySettings returned error: %v", err)
	}

	if diags := r.Check(f); len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	} else if !strings.Contains(diags[0].Message, "(6 > 5") {
		t.Fatalf("expected budget 5 from last matching glob, got %q", diags[0].Message)
	}
}

func TestCheck_PerGlobBudget_NoMatchUsesDefault(t *testing.T) {
	f := mustFile(t, "README.md", "one two three")
	r := &Rule{Max: 3, Mode: "heuristic", TokensPerWord: 1.0}
	if err := r.ApplySettings(map[string]any{
		"budgets": []any{map[string]any{"glob": "docs/*.md", "max": 1}},
	}); err != nil {
		t.Fatalf("ApplySettings returned error: %v", err)
	}
	if diags := r.Check(f); len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestApplySettings_Valid(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"max":             2048,
		"mode":            "tokenizer",
		"tokens-per-word": 0.9,
		"tokenizer":       "builtin",
		"encoding":        "gpt2",
		"budgets": []any{
			map[string]any{"glob": "README.md", "max": 1024},
		},
	})
	require.NoError(t, err, "unexpected error: %v", err)
	if r.Max != 2048 {
		t.Errorf("expected Max=2048, got %d", r.Max)
	}
	if r.Mode != "tokenizer" {
		t.Errorf("expected Mode=tokenizer, got %s", r.Mode)
	}
	if r.TokensPerWord != 0.9 {
		t.Errorf("expected TokensPerWord=0.9, got %v", r.TokensPerWord)
	}
	if r.Tokenizer != "builtin" {
		t.Errorf("expected Tokenizer=builtin, got %s", r.Tokenizer)
	}
	if r.Encoding != "gpt2" {
		t.Errorf("expected Encoding=gpt2, got %s", r.Encoding)
	}
	require.Len(t, r.Budgets, 1, "expected 1 budget override, got %d", len(r.Budgets))
}

func TestApplySettings_InvalidType(t *testing.T) {
	r := &Rule{}
	if err := r.ApplySettings(map[string]any{"max": "many"}); err == nil {
		t.Fatal("expected error for non-int max")
	}
	if err := r.ApplySettings(map[string]any{"tokens-per-word": "high"}); err == nil {
		t.Fatal("expected error for non-number tokens-per-word")
	}
	if err := r.ApplySettings(map[string]any{"mode": 123}); err == nil {
		t.Fatal("expected error for non-string mode")
	}
}

func TestApplySettings_InvalidValues(t *testing.T) {
	r := &Rule{}
	if err := r.ApplySettings(map[string]any{"max": 0}); err == nil {
		t.Fatal("expected error for non-positive max")
	}
	if err := r.ApplySettings(map[string]any{"tokens-per-word": -1.0}); err == nil {
		t.Fatal("expected error for non-positive tokens-per-word")
	}
	if err := r.ApplySettings(map[string]any{"mode": "unknown"}); err == nil {
		t.Fatal("expected error for invalid mode")
	}
	if err := r.ApplySettings(map[string]any{"tokenizer": "other"}); err == nil {
		t.Fatal("expected error for invalid tokenizer")
	}
	if err := r.ApplySettings(map[string]any{"encoding": "other"}); err == nil {
		t.Fatal("expected error for invalid encoding")
	}
}

func TestApplySettings_InvalidBudgets(t *testing.T) {
	r := &Rule{}
	cases := []map[string]any{
		{"budgets": "bad"},
		{"budgets": []any{"bad"}},
		{"budgets": []any{map[string]any{"glob": 42, "max": 1}}},
		{"budgets": []any{map[string]any{"glob": "", "max": 1}}},
		{"budgets": []any{map[string]any{"glob": "[invalid", "max": 1}}},
		{"budgets": []any{map[string]any{"glob": "*.md", "max": "x"}}},
		{"budgets": []any{map[string]any{"glob": "*.md", "max": 0}}},
		{"budgets": []any{map[string]any{"glob": "*.md", "max": 1, "other": true}}},
	}

	for i, settings := range cases {
		if err := r.ApplySettings(settings); err == nil {
			t.Fatalf("case %d: expected error, got nil", i)
		}
	}
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	if err := r.ApplySettings(map[string]any{"unknown": true}); err == nil {
		t.Fatal("expected error for unknown setting")
	}
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["max"] != defaultMax {
		t.Errorf("expected max=%d, got %v", defaultMax, ds["max"])
	}
	if ds["mode"] != defaultMode {
		t.Errorf("expected mode=%q, got %v", defaultMode, ds["mode"])
	}
	if ds["tokens-per-word"] != defaultTokensPerWord {
		t.Errorf("expected tokens-per-word=%v, got %v", defaultTokensPerWord, ds["tokens-per-word"])
	}
	if ds["tokenizer"] != defaultTokenizer {
		t.Errorf("expected tokenizer=%q, got %v", defaultTokenizer, ds["tokenizer"])
	}
	if ds["encoding"] != defaultEncoding {
		t.Errorf("expected encoding=%q, got %v", defaultEncoding, ds["encoding"])
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS028" {
		t.Errorf("expected MDS028, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "token-budget" {
		t.Errorf("expected token-budget, got %s", r.Name())
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() != "meta" {
		t.Errorf("expected meta, got %s", r.Category())
	}
}
