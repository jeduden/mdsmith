package corpus

import "testing"

func TestJaccard(t *testing.T) {
	t.Parallel()

	a := map[string]struct{}{"a": {}, "b": {}, "c": {}}
	b := map[string]struct{}{"b": {}, "c": {}, "d": {}}
	got := jaccard(a, b)
	want := 0.5
	if got != want {
		t.Fatalf("jaccard = %f, want %f", got, want)
	}

	if jaccard(map[string]struct{}{}, b) != 0 {
		t.Fatal("expected zero jaccard for empty set")
	}
}

func TestDropNearDuplicates(t *testing.T) {
	t.Parallel()

	records := []collectedRecord{
		makeCollected("a", CategoryReference, false, map[string]struct{}{"alpha": {}, "beta": {}}),
		makeCollected("b", CategoryReference, false, map[string]struct{}{"alpha": {}, "beta": {}}),
		makeCollected("c", CategoryReference, false, map[string]struct{}{"gamma": {}, "delta": {}}),
	}

	kept, dropped := dropNearDuplicates(records, 1.0)
	if dropped != 1 {
		t.Fatalf("dropped = %d, want 1", dropped)
	}
	if len(kept) != 2 {
		t.Fatalf("kept = %d, want 2", len(kept))
	}
}

func TestCapReadmes(t *testing.T) {
	t.Parallel()

	records := make([]collectedRecord, 0)
	for i := 0; i < 8; i++ {
		records = append(records, makeCollected(idFor("n", i), CategoryReference, false, nil))
	}
	for i := 0; i < 6; i++ {
		records = append(records, makeCollected(idFor("r", i), CategoryProjectDocs, true, nil))
	}

	capped, dropped := capReadmes(records, 0.2)
	if dropped != 4 {
		t.Fatalf("dropped = %d, want 4", dropped)
	}
	if len(capped) != 10 {
		t.Fatalf("len = %d, want 10", len(capped))
	}

	readmes := 0
	for _, record := range capped {
		if record.IsReadme {
			readmes++
		}
	}
	if readmes != 2 {
		t.Fatalf("readmes = %d, want 2", readmes)
	}
}

func TestApplyBalance(t *testing.T) {
	t.Parallel()

	records := []collectedRecord{
		makeCollected("d1", CategoryDesignProposal, false, nil),
		makeCollected("d2", CategoryDesignProposal, false, nil),
		makeCollected("d3", CategoryDesignProposal, false, nil),
		makeCollected("d4", CategoryDesignProposal, false, nil),
		makeCollected("d5", CategoryDesignProposal, false, nil),
		makeCollected("r1", CategoryReference, false, nil),
		makeCollected("r2", CategoryReference, false, nil),
		makeCollected("r3", CategoryReference, false, nil),
	}
	ranges := map[Category]BalanceRange{
		CategoryDesignProposal: {Min: 0.3, Max: 0.5},
		CategoryReference:      {Min: 0.3, Max: 0.7},
	}

	balanced, dropped, violations := applyBalance(records, ranges)
	if dropped != 1 {
		t.Fatalf("dropped = %d, want 1", dropped)
	}
	if len(balanced) != 7 {
		t.Fatalf("balanced len = %d, want 7", len(balanced))
	}
	if len(violations) != 1 {
		t.Fatalf("violations = %v, want one violation", violations)
	}
}

func TestCheckBalanceViolations(t *testing.T) {
	t.Parallel()

	records := []collectedRecord{
		makeCollected("r1", CategoryReference, false, nil),
		makeCollected("r2", CategoryReference, false, nil),
	}
	violations := checkBalanceViolations(records, map[Category]BalanceRange{
		CategoryReference:    {Min: 0, Max: 0.4},
		CategoryHowTo:        {Min: 0.1, Max: 0.8},
		CategoryAgentControl: {Min: 0, Max: 1},
	})
	if len(violations) != 2 {
		t.Fatalf("violations len = %d, want 2", len(violations))
	}
}

func makeCollected(id string, category Category, isReadme bool, tokens map[string]struct{}) collectedRecord {
	if tokens == nil {
		tokens = map[string]struct{}{}
	}
	return collectedRecord{
		ManifestRecord: ManifestRecord{
			RecordID: id,
			Category: category,
			IsReadme: isReadme,
		},
		tokenSet: tokens,
	}
}

func idFor(prefix string, idx int) string {
	return prefix + string(rune('a'+idx))
}
