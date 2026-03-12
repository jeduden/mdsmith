package corpus

import "testing"

func TestDedup_RemovesExactDuplicates(t *testing.T) {
	t.Parallel()

	records := []Record{
		{RecordID: "a", RawContent: "# Doc\n\nword word", ContentSHA256: sha256Hex("# Doc\n\nword word")},
		{RecordID: "b", RawContent: "# Doc\n\nword word", ContentSHA256: sha256Hex("# Doc\n\nword word")},
		{RecordID: "c", RawContent: "# Other\n\nword", ContentSHA256: sha256Hex("# Other\n\nword")},
	}

	got := Dedup(records)
	if len(got) != 2 {
		t.Fatalf("len(Dedup) = %d, want 2", len(got))
	}
	if got[0].RecordID != "a" || got[1].RecordID != "c" {
		t.Fatalf("unexpected kept IDs: %s, %s", got[0].RecordID, got[1].RecordID)
	}
}

func TestDedup_EmptyInput(t *testing.T) {
	t.Parallel()

	got := Dedup(nil)
	if len(got) != 0 {
		t.Fatalf("len(Dedup(nil)) = %d, want 0", len(got))
	}
}
