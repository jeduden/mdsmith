package corpus

import "testing"

func TestClassify_AssignsReferenceAndOther(t *testing.T) {
	t.Parallel()

	records := []Record{
		{RecordID: "a", Path: "docs/reference/cli.md", RawContent: "# CLI reference\n\nword word"},
		{RecordID: "b", Path: "guides/intro.md", RawContent: "# Getting Started\n\nword word"},
	}

	got := Classify(records)
	if got[0].Category != CategoryReference {
		t.Fatalf("record a category = %q, want %q", got[0].Category, CategoryReference)
	}
	if got[1].Category != CategoryOther {
		t.Fatalf("record b category = %q, want %q", got[1].Category, CategoryOther)
	}
}

func TestClassify_EmptyInput(t *testing.T) {
	t.Parallel()

	got := Classify(nil)
	if len(got) != 0 {
		t.Fatalf("len(Classify(nil)) = %d, want 0", len(got))
	}
}
