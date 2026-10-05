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
	if raceEnabled {
		t.Skip("allocation counts are not stable under -race")
	}
	heading := []byte("Installing the Command Line Tool on Linux and macOS today")
	got := testing.AllocsPerRun(100, func() {
		s := &ids{values: map[string]struct{}{}}
		_ = s.Generate(heading, ast.KindHeading)
	})
	// One slug buffer plus the map insert; the unsized form took four
	// (8, 16, 32, 64 bytes) before the insert.
	assert.LessOrEqual(t, got, 2.0, "Generate regrew its result buffer")
}

// TestIDsGenerateNonASCIIHeadingAllocBudget pins that a heading with no
// ASCII letters, whose slug falls back to "heading", does not allocate a
// pre-sized buffer it then discards.
func TestIDsGenerateNonASCIIHeadingAllocBudget(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not stable under -race")
	}
	heading := []byte("日本語のタイトルとても長い見出しです")
	got := testing.AllocsPerRun(100, func() {
		s := &ids{values: map[string]struct{}{}}
		_ = s.Generate(heading, ast.KindHeading)
	})
	// The "heading" fallback buffer plus the map insert; no discarded
	// pre-sized buffer.
	assert.LessOrEqual(t, got, 2.0)
}

// TestIDsGenerateLeadingPunctuation covers slugs whose first kept byte
// is a space, hyphen or underscore rather than an ASCII letter.
func TestIDsGenerateLeadingPunctuation(t *testing.T) {
	s := &ids{values: map[string]struct{}{}}
	assert.Equal(t, "-private", string(s.Generate([]byte("_private"), ast.KindHeading)))
	assert.Equal(t, "---flags", string(s.Generate([]byte("-- flags"), ast.KindHeading)))
	assert.Equal(t, "-guide", string(s.Generate([]byte("日本語 guide"), ast.KindHeading)))
}
