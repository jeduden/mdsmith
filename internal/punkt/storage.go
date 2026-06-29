package punkt

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
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
// Collocations is keyed by the upstream `typ + "," + nextTyp` string.
// The runtime path in tokenAnnotation reproduces the key into a
// pooled byte buffer and looks it up with
// `Collocations[string(buf)]` — relying on the compiler's
// `m[string(b)]` elision so the lookup itself does not allocate. An
// earlier draft of plan 193 carried a derived `map[[2]string]`
// index, but the elision path is allocation-equivalent and keeps
// Storage one map smaller.
type Storage struct {
	AbbrevTypes  SetString `json:"AbbrevTypes"`
	Collocations SetString `json:"Collocations"`
	SentStarters SetString `json:"SentStarters"`
	OrthoContext SetString `json:"OrthoContext"`
}

// NewStorage returns an empty Storage with all maps initialized.
// Used in tests; LoadTraining is the production constructor.
func NewStorage() *Storage {
	return &Storage{
		AbbrevTypes:  SetString{},
		Collocations: SetString{},
		SentStarters: SetString{},
		OrthoContext: SetString{},
	}
}

// LoadTraining parses the JSON training data shipped with
// neurosnap/sentences and returns the corresponding Storage.
// An empty/malformed input returns the json.Unmarshal error.
func LoadTraining(data []byte) (*Storage, error) {
	var s Storage
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
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

// abbrTokenCutset is the trailing clause punctuation IsAbbrevToken
// ignores, so "e.g.," and "cf.;" are judged like "e.g." and "cf.".
const abbrTokenCutset = ",;:"

// IsAbbrevToken reports whether tok — a single whitespace-delimited
// word such as "Dr.", "e.g.", or "J." — is an abbreviation under this
// trained model, without running the full sentence tokenizer. It
// recognises three shapes the pipeline's type-annotation pass treats as
// abbreviations: a single-letter initial ("J."), a dotted abbreviation
// pattern ("e.g.", "U.S.A."), and a member of the trained AbbrevTypes
// set ("dr", "vs", "no"). Trailing clause punctuation is ignored, and
// the AbbrevTypes lookup drops the final period and lowercases the word,
// matching how typeAnnotation keys the set.
//
// It exists for callers — line-length reflow is the first — that need
// the model's per-token abbreviation judgement to decide a wrap, not a
// sentence boundary.
func (s *Storage) IsAbbrevToken(tok string) bool {
	t := strings.TrimRight(tok, abbrTokenCutset)
	if t == "" {
		return false
	}
	if isInitial(t) || MatchAbbrPattern(t) {
		return true
	}
	if !HasPeriodFinal(t) {
		return false
	}
	_, sz := utf8.DecodeLastRuneInString(t)
	return s.IsAbbr(strings.ToLower(t[:len(t)-sz]))
}

// addOrthoContext sets the ortho flag for typ. Used by training
// loaders; kept here only because tests in this package construct
// Storage values and seed OrthoContext directly.
func (s *Storage) addOrthoContext(typ string, flag int) {
	s.OrthoContext[typ] |= flag
}
