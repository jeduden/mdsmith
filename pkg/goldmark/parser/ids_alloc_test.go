package parser

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
)

// TestIDsGenerateAllocBudget guards the "pre-size slices" rule in
// docs/development/high-performance-go.md: Generate knows the slug is no
// longer than its input, so a 60-byte heading must take one buffer, not
// one per append growth step (8, 16, 32, 64).
func TestIDsGenerateAllocBudget(t *testing.T) {
	heading := []byte("Installing the Command Line Tool on Linux and macOS today")
	got := testing.AllocsPerRun(100, func() {
		s := &ids{values: map[string]struct{}{}}
		_ = s.Generate(heading, ast.KindHeading)
	})
	// One slug buffer; the unsized form took four (8, 16, 32, 64 bytes).
	assert.LessOrEqual(t, got, 1.0, "Generate regrew its result buffer")
}
