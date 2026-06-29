package punkt

import "testing"

func TestIsAbbrevToken(t *testing.T) {
	s := NewEnglish().Storage
	cases := []struct {
		tok  string
		want bool
	}{
		{"J.", true},        // single-letter initial
		{"e.g.", true},      // dotted abbreviation pattern
		{"i.e.", true},      // dotted abbreviation pattern
		{"U.S.A.", true},    // dotted abbreviation pattern
		{"Dr.", true},       // trained AbbrevTypes (dr)
		{"vs.", true},       // trained AbbrevTypes (vs)
		{"Mr.", true},       // trained AbbrevTypes (mr)
		{"e.g.,", true},     // trailing comma ignored
		{"Dr.;", true},      // trailing semicolon ignored
		{"cat.", false},     // ordinary word ending a sentence
		{"Go.", false},      // ordinary word ending a sentence
		{"plain", false},    // no trailing period
		{"e.g", false},      // no trailing period, not in AbbrevTypes
		{",", false},        // trims to empty
		{"", false},         // empty
		{"unknown.", false}, // period-final but untrained
	}
	for _, c := range cases {
		if got := s.IsAbbrevToken(c.tok); got != c.want {
			t.Errorf("IsAbbrevToken(%q) = %v, want %v", c.tok, got, c.want)
		}
	}
}

// TestIsAbbrevToken_SyntheticStorage drives the AbbrevTypes branch with
// a hermetic Storage so the trained-asset cases above are not the only
// coverage of the lowercase/drop-period lookup.
func TestIsAbbrevToken_SyntheticStorage(t *testing.T) {
	s := NewStorage()
	s.AbbrevTypes.Add("approx")
	if !s.IsAbbrevToken("approx.") {
		t.Errorf("IsAbbrevToken(approx.) should be true for a seeded type")
	}
	if !s.IsAbbrevToken("Approx.") {
		t.Errorf("IsAbbrevToken(Approx.) should lowercase before lookup")
	}
	if s.IsAbbrevToken("approxx.") {
		t.Errorf("IsAbbrevToken(approxx.) should be false; not a seeded type")
	}
}
