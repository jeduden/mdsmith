package tablereadability

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

func newFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err, "NewFile: %v", err)
	return f
}

func TestID(t *testing.T) {
	r := &Rule{}
	if got := r.ID(); got != "MDS026" {
		t.Fatalf("ID = %q, want %q", got, "MDS026")
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if got := r.Name(); got != "table-readability" {
		t.Fatalf("Name = %q, want %q", got, "table-readability")
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if got := r.Category(); got != "table" {
		t.Fatalf("Category = %q, want %q", got, "table")
	}
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()

	if got := ds["max-columns"]; got != 8 {
		t.Fatalf("max-columns = %v, want 8", got)
	}
	if got := ds["max-rows"]; got != 30 {
		t.Fatalf("max-rows = %v, want 30", got)
	}
	if got := ds["max-words-per-cell"]; got != 30 {
		t.Fatalf("max-words-per-cell = %v, want 30", got)
	}
	if got := ds["max-column-width-ratio"]; got != 60.0 {
		t.Fatalf("max-column-width-ratio = %v, want 60.0", got)
	}
}

func TestApplySettings_Valid(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"max-columns":            4,
		"max-rows":               12,
		"max-words-per-cell":     10,
		"max-column-width-ratio": 2.25,
	})
	require.NoError(t, err, "ApplySettings: %v", err)

	if r.MaxColumns != 4 {
		t.Fatalf("MaxColumns = %d, want 4", r.MaxColumns)
	}
	if r.MaxRows != 12 {
		t.Fatalf("MaxRows = %d, want 12", r.MaxRows)
	}
	if r.MaxWordsPerCell != 10 {
		t.Fatalf("MaxWordsPerCell = %d, want 10", r.MaxWordsPerCell)
	}
	if r.MaxColumnWidthRatio != 2.25 {
		t.Fatalf("MaxColumnWidthRatio = %v, want 2.25", r.MaxColumnWidthRatio)
	}
}

func TestApplySettings_InvalidType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-columns": "a"})
	require.Error(t, err, "expected error")
}

func TestApplySettings_UnknownSetting(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": 1})
	require.Error(t, err, "expected error")
}

func TestCheck_NoDiagnosticForReadableTable(t *testing.T) {
	r := &Rule{}
	src := "# Title\n\n| Metric | Value |\n|--------|-------|\n| Speed  | Fast  |\n| Cost   | Low   |\n"

	diags := r.Check(newFile(t, src))
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_TooManyColumns(t *testing.T) {
	r := &Rule{MaxColumns: 3, MaxRows: 20, MaxWordsPerCell: 50, MaxColumnWidthRatio: 10}
	src := "# Title\n\n| A | B | C | D |\n|---|---|---|---|\n| 1 | 2 | 3 | 4 |\n"

	diags := r.Check(newFile(t, src))
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Line != 3 {
		t.Fatalf("line = %d, want 3", diags[0].Line)
	}
	if diags[0].Message != "table has too many columns (4 > 3)" {
		t.Fatalf("message = %q", diags[0].Message)
	}
}

func TestCheck_TooManyRows(t *testing.T) {
	r := &Rule{MaxColumns: 6, MaxRows: 2, MaxWordsPerCell: 50, MaxColumnWidthRatio: 10}
	src := "# Title\n\n| A | B |\n|---|---|\n| 1 | 2 |\n| 3 | 4 |\n| 5 | 6 |\n"

	diags := r.Check(newFile(t, src))
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Line != 3 {
		t.Fatalf("line = %d, want 3", diags[0].Line)
	}
	if diags[0].Message != "table has too many rows (3 > 2)" {
		t.Fatalf("message = %q", diags[0].Message)
	}
}

func TestCheck_TooManyWordsPerCell(t *testing.T) {
	r := &Rule{MaxColumns: 6, MaxRows: 20, MaxWordsPerCell: 4, MaxColumnWidthRatio: 10}
	src := "# Title\n\n| Name | Notes |\n|------|-------|\n| ok   | short |\n| bad  | This cell has six words total |\n"

	diags := r.Check(newFile(t, src))
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Line != 6 {
		t.Fatalf("line = %d, want 6", diags[0].Line)
	}
	require.Contains(t, diags[0].Message, "table cell has too many words (6 > 4)",
		"message = %q", diags[0].Message)
	require.Contains(t, diags[0].Message, "column \"Notes\"",
		"message should include column header name, got: %q", diags[0].Message)
}

func TestCheck_HighWidthRatio(t *testing.T) {
	r := &Rule{MaxColumns: 6, MaxRows: 20, MaxWordsPerCell: 100, MaxColumnWidthRatio: 1.50}
	src := `# Title

| Key | Description |
|-----|-------------|
| a   | very very very very long explanation text |
| b   | short |
`

	diags := r.Check(newFile(t, src))
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Line != 3 {
		t.Fatalf("line = %d, want 3", diags[0].Line)
	}
	require.Contains(t, diags[0].Message, "table has high column width ratio", "message = %q", diags[0].Message)
}

func TestCheck_SkipsTablesInCodeBlock(t *testing.T) {
	r := &Rule{MaxColumns: 1, MaxRows: 1, MaxWordsPerCell: 1, MaxColumnWidthRatio: 1.01}
	src := "# Title\n\n```markdown\n| A | B | C |\n|---|---|---|\n| one two three four | x | y |\n```\n"

	diags := r.Check(newFile(t, src))
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestSplitRow_PreservesEscapedPipes(t *testing.T) {
	cells := splitRow(`| a \| b | c |`)
	require.Len(t, cells, 2, "expected 2 cells, got %d", len(cells))
	if cells[0] != `a \| b` {
		t.Fatalf("first cell = %q, want %q", cells[0], `a \| b`)
	}
	if cells[1] != "c" {
		t.Fatalf("second cell = %q, want %q", cells[1], "c")
	}
}
