package metrics

import "testing"

func TestSortRows_DescendingWithPathTieBreak(t *testing.T) {
	def, ok := LookupScope(ScopeFile, "bytes")
	if !ok {
		t.Fatal("bytes metric not found")
	}

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
	if !ok {
		t.Fatal("conciseness metric not found")
	}

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
	if len(limited) != 2 {
		t.Fatalf("len = %d, want 2", len(limited))
	}
}

func TestFormatValue(t *testing.T) {
	intDef, ok := LookupScope(ScopeFile, "bytes")
	if !ok {
		t.Fatal("bytes metric not found")
	}
	floatDef, ok := LookupScope(ScopeFile, "conciseness")
	if !ok {
		t.Fatal("conciseness metric not found")
	}

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
