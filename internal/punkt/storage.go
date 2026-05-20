package punkt

import (
	"encoding/json"
	"strings"
)

// SetString is a string-keyed set matching upstream's JSON shape:
// values are int (always 1 for set membership) so existing training
// JSON loads without translation. Lookups go through Has, which
// returns true for any non-zero value.
type SetString map[string]int

// Add marks str as present in the set.
func (ss SetString) Add(str string) { ss[str] = 1 }

// Has reports whether str is present in the set.
func (ss SetString) Has(str string) bool { return ss[str] != 0 }

// Storage holds the trained Punkt model. The JSON-loaded fields
// (AbbrevTypes, Collocations, SentStarters, OrthoContext) mirror the
// upstream shape so existing training assets (data/english.json from
// neurosnap/sentences/data) deserialize unchanged.
//
// CollocationIndex is a derived structure built from Collocations at
// load time. Upstream looks up collocations via
// `strings.Join([]string{typ, nextTyp}, ",")` followed by a SetString
// hit; that join is one of the per-token allocation sources plan 193
// targets. The index keys the same data by the [2]string pair
// directly, so HasCollocation runs without joining.
type Storage struct {
	AbbrevTypes  SetString `json:"AbbrevTypes"`
	Collocations SetString `json:"Collocations"`
	SentStarters SetString `json:"SentStarters"`
	OrthoContext SetString `json:"OrthoContext"`

	// CollocationIndex is built by rebuildCollocationIndex from
	// Collocations. Direct lookups go through HasCollocation.
	CollocationIndex map[[2]string]struct{} `json:"-"`
}

// NewStorage returns an empty Storage with all maps initialized.
// Used in tests; LoadTraining is the production constructor.
func NewStorage() *Storage {
	return &Storage{
		AbbrevTypes:      SetString{},
		Collocations:     SetString{},
		SentStarters:     SetString{},
		OrthoContext:     SetString{},
		CollocationIndex: map[[2]string]struct{}{},
	}
}

// LoadTraining parses the JSON training data shipped with
// neurosnap/sentences and returns the corresponding Storage. The
// derived CollocationIndex is built before return, so HasCollocation
// is usable immediately. An empty/malformed input returns the
// json.Unmarshal error.
func LoadTraining(data []byte) (*Storage, error) {
	var s Storage
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	s.rebuildCollocationIndex()
	return &s, nil
}

// rebuildCollocationIndex regenerates CollocationIndex from
// Collocations. Called once from LoadTraining; callers who mutate
// Collocations at runtime (none currently) would have to call this
// again to keep the index in sync.
//
// Upstream stores each collocation as `typ + "," + nextTyp` in a
// SetString. The index keys the same pair as a [2]string so the
// runtime lookup avoids `strings.Join`. A malformed key (missing
// comma) is skipped — that key would never match a runtime join
// against a real (typ, nextTyp) pair anyway.
func (s *Storage) rebuildCollocationIndex() {
	s.CollocationIndex = make(map[[2]string]struct{}, len(s.Collocations))
	for k, v := range s.Collocations {
		if v == 0 {
			continue
		}
		i := strings.IndexByte(k, ',')
		if i < 0 {
			continue
		}
		s.CollocationIndex[[2]string{k[:i], k[i+1:]}] = struct{}{}
	}
}

// HasCollocation reports whether (typ, nextTyp) appears in the
// trained collocation map. The lookup goes through CollocationIndex
// to avoid the per-call `strings.Join` upstream does.
func (s *Storage) HasCollocation(typ, nextTyp string) bool {
	_, ok := s.CollocationIndex[[2]string{typ, nextTyp}]
	return ok
}

// IsAbbr reports whether any of tokens is a known abbreviation type.
// Mirrors upstream Storage.IsAbbr.
func (s *Storage) IsAbbr(tokens ...string) bool {
	for _, t := range tokens {
		if s.AbbrevTypes.Has(t) {
			return true
		}
	}
	return false
}

// addOrthoContext sets the ortho flag for typ. Used by training
// loaders; kept here only because tests in this package construct
// Storage values and seed OrthoContext directly.
func (s *Storage) addOrthoContext(typ string, flag int) {
	s.OrthoContext[typ] |= flag
}
