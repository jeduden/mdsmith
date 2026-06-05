package lsp

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	"github.com/stretchr/testify/assert"
)

// stubRule is a minimal rule.Rule that is not a QuickFixTitler.
type stubRule struct{ name string }

func (s stubRule) ID() string                         { return "MDS999" }
func (s stubRule) Name() string                       { return s.name }
func (s stubRule) Category() string                   { return "test" }
func (s stubRule) Check(*lint.File) []lint.Diagnostic { return nil }

// stubTitledRule adds a custom quick-fix label.
type stubTitledRule struct {
	stubRule
	title string
}

func (s stubTitledRule) FixTitle() string { return s.title }

// TestQuickFixTitle covers the three quickFixTitle branches: a rule that
// supplies its own label, a matching rule that is not a QuickFixTitler
// (break → generic fallback), and a name absent from the set (loop
// completes → generic fallback).
func TestQuickFixTitle(t *testing.T) {
	rules := []rule.Rule{
		stubTitledRule{stubRule{name: "fancy"}, "Do the fancy thing"},
		stubRule{name: "plain"},
	}

	assert.Equal(t, "Do the fancy thing", quickFixTitle(rules, "fancy"))
	assert.Equal(t, "Fix all plain with mdsmith", quickFixTitle(rules, "plain"))
	assert.Equal(t, "Fix all missing with mdsmith", quickFixTitle(rules, "missing"))
}
