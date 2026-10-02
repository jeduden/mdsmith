package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSortRows_DescendingWithPathTieBreak(t *testing.T) {
	def, ok := LookupScope(ScopeFile, "bytes")
	require.True(t, ok, "bytes metric not found")

	rows := []Row{
		{Path: "b.md", Metrics: map[string]Value{"bytes": AvailableValue(10)}},
		{Path: "a.md", Metrics: map[string]Value{"bytes": AvailableValue(10)}},
		{Path: "c.md", Metrics: map[string]Value{"bytes": AvailableValue(3)}},
	}

	SortRows(rows, def, OrderDesc)

	want := []string{"a.md", "b.md", "c.md"}
	for i, path := range want {
		if rows[i].Path != path {
			t.Fatalf("row %d path = %q, want %q", i, rows[i].Path, path)
		}
	}
}

func TestSortRows_AvailableBeforeUnavailable(t *testing.T) {
	def, ok := LookupScope(ScopeFile, "conciseness")
	require.True(t, ok, "conciseness metric not found")

	rows := []Row{
		{Path: "a.md", Metrics: map[string]Value{"conciseness": UnavailableValue()}},
		{Path: "b.md", Metrics: map[string]Value{"conciseness": AvailableValue(40)}},
	}

	SortRows(rows, def, OrderAsc)
	if rows[0].Path != "b.md" {
		t.Fatalf("available row should sort first, got %q", rows[0].Path)
	}
}

func TestLimitRows(t *testing.T) {
	rows := []Row{
		{Path: "a.md"},
		{Path: "b.md"},
		{Path: "c.md"},
	}
	limited := LimitRows(rows, 2)
	require.Len(t, limited, 2, "len = %d, want 2", len(limited))
}

func TestFormatValue(t *testing.T) {
	intDef, ok := LookupScope(ScopeFile, "bytes")
	require.True(t, ok, "bytes metric not found")
	floatDef, ok := LookupScope(ScopeFile, "conciseness")
	require.True(t, ok, "conciseness metric not found")

	if got := FormatValue(intDef, AvailableValue(12.4)); got != "12" {
		t.Fatalf("int format = %q, want 12", got)
	}
	if got := FormatValue(floatDef, AvailableValue(12.44)); got != "12.4" {
		t.Fatalf("float format = %q, want 12.4", got)
	}
	if got := FormatValue(floatDef, UnavailableValue()); got != "-" {
		t.Fatalf("unavailable format = %q, want -", got)
	}
}

// SortRows must not go through reflect.Swapper (sort.Slice) or look the
// metric up in a map per comparison; see
// docs/development/high-performance-go.md, "Patterns to avoid".
func TestSortRows_AllocsBounded(t *testing.T) {
	skipAllocGate(t)
	def, ok := LookupScope(ScopeFile, "bytes")
	require.True(t, ok)
	const n = 500
	base := make([]Row, n)
	for i := range base {
		base[i] = Row{
			Path:    string(rune('a'+i%26)) + string(rune('a'+i/26%26)) + ".md",
			Metrics: map[string]Value{"bytes": AvailableValue(float64(i % 17))},
		}
	}
	rows := make([]Row, n)
	allocs := testing.AllocsPerRun(10, func() {
		copy(rows, base)
		SortRows(rows, def, OrderDesc)
	})
	require.LessOrEqual(t, allocs, 1.0)
}

// The comparator sees (unavailable, available) pairs in either argument
// order; both must sort available rows first.
func TestSortRows_UnavailableAfterAvailableEitherInputOrder(t *testing.T) {
	def, ok := LookupScope(ScopeFile, "bytes")
	require.True(t, ok)
	unavail := Row{Path: "u.md", Metrics: map[string]Value{"bytes": UnavailableValue()}}
	avail := Row{Path: "a.md", Metrics: map[string]Value{"bytes": AvailableValue(1)}}

	for _, rows := range [][]Row{{unavail, avail}, {avail, unavail}} {
		in := append([]Row(nil), rows...)
		SortRows(in, def, OrderAsc)
		require.Equal(t, "a.md", in[0].Path)
		require.Equal(t, "u.md", in[1].Path)
	}
}
