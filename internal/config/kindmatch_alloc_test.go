package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResolveEffectiveKinds_AllocBudget pins resolveEffectiveKinds's
// per-call allocation count on a file that matches a kind-assignment
// entry. resolveEffectiveKinds only needs matchKindAssignmentEntry's
// bool verdict, but matchKindAssignmentEntry unconditionally also
// builds the provenance selector string (formatSelector) and throws
// it away — see docs/development/high-performance-go.md "Skip work
// you don't need". resolveEffectiveKinds runs on every workspace file
// via config.EffectiveSignature, so those wasted allocs pay for
// themselves once per file per matching kind-assignment entry, every
// check/fix run.
func TestResolveEffectiveKinds_AllocBudget(t *testing.T) {
	cfg := &Config{
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"plan/*.md"}, Kinds: []string{"plan"}},
		},
	}
	allocs := testing.AllocsPerRun(200, func() {
		_ = resolveEffectiveKinds(cfg, "plan/1.md", nil, nil)
	})
	if allocs > 1 {
		t.Fatalf("resolveEffectiveKinds allocs per call: want <= 1, got %v", allocs)
	}
}

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
