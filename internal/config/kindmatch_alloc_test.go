package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestKindAssignmentEntryMatches_NoSelectorAlloc pins the allocation
// cost of the fast match-only path resolveEffectiveKinds uses.
// matchKindAssignmentEntry always built the provenance selector
// string via two strings.Join calls even when the caller discards it
// with `_` — resolveEffectiveKinds backs EffectiveSignature, which
// runs unconditionally on every file per
// docs/development/high-performance-go.md's "skip work you don't
// need". kindAssignmentEntryMatches does the same match without ever
// building that string.
func TestKindAssignmentEntryMatches_NoSelectorAlloc(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	entry := KindAssignmentEntry{
		Glob:  []string{"docs/**/*.md", "plan/*.md"},
		Kinds: []string{"doc"},
	}

	const runs = 200
	allocs := testing.AllocsPerRun(runs, func() {
		matched := kindAssignmentEntryMatches(entry, "docs/guide/index.md", nil)
		if !matched {
			t.Fatal("expected entry to match")
		}
	})
	t.Logf("kindAssignmentEntryMatches allocs/op = %.0f", allocs)
	require.LessOrEqualf(t, allocs, float64(0),
		"kindAssignmentEntryMatches allocs/op = %.0f, want 0 (no selector string built)", allocs)
}

func TestKindAssignmentEntryMatches_AgreesWithMatchKindAssignmentEntry(t *testing.T) {
	entry := KindAssignmentEntry{
		Glob:          []string{"docs/**/*.md"},
		FieldsPresent: []string{"status"},
		Kinds:         []string{"doc"},
	}

	cases := []struct {
		name     string
		path     string
		fmFields map[string]any
	}{
		{"glob and fields match", "docs/a.md", map[string]any{"status": "open"}},
		{"glob matches, fields missing", "docs/a.md", nil},
		{"fields match, glob misses", "plan/a.md", map[string]any{"status": "open"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantMatched, wantSelector := matchKindAssignmentEntry(entry, tc.path, tc.fmFields)
			gotMatched := kindAssignmentEntryMatches(entry, tc.path, tc.fmFields)
			require.Equal(t, wantMatched, gotMatched)
			if wantMatched {
				require.NotEmpty(t, wantSelector)
			}
		})
	}
}
