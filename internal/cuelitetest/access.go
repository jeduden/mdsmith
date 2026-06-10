package cuelitetest

// access.go — differential harness for the surface-A/B accessor methods:
// LookupPath, Fields, Exists, and String.
//
// Surfaces A (schema) and B (query) read a compiled value back: the query
// arm walks Fields() to collect leaf paths, then LookupPath().Exists()
// confirms each leaf is present in the data before unifying. The
// validate-only arms in the main harness do not cover those reads, so this
// file adds an accessor-comparing arm that runs one data document plus one
// lookup path through both the cuelite façade and a direct cuelang.org/go
// oracle, asserting they agree on existence, on the looked-up string, and
// on the set of top-level field selectors.
//
// The oracle arm reconstructs the SAME contract cuelite implements:
// LookupPath built from raw string segments via cue.Str (so a dotted or
// hyphenated data key is looked up verbatim), Exists() on the result, and
// Fields() over cue.Selector.Unquoted() labels. The two arms therefore
// compare the same string-label read model rather than the oracle reaching
// selectors the façade cannot represent.

import (
	"slices"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	cuelitepkg "github.com/jeduden/mdsmith/cue/cuelite"
)

// AccessCase is one differential-test input: a JSON data document, the
// raw path segments to look up within it, and the case Name for failure
// messages. Segments are RAW (not a parsed expression) because both arms
// build the path with the verbatim-segment constructor (MakePath /
// cue.Str), mirroring how query derives a path from a data key.
type AccessCase struct {
	Name     string
	Data     string
	Segments []string
}

// AccessOutcome is the result of running an AccessCase through one arm.
// Exists reports whether the looked-up path resolved; Str holds the
// looked-up leaf's concrete string when it is one (empty otherwise);
// HasStr distinguishes an empty-string leaf from a non-string leaf;
// Fields holds the document's top-level selectors, sorted. Comparing all
// four makes the arms agree on existence, on the decoded leaf, and on the
// enumerated field set — the reads query and schema depend on.
type AccessOutcome struct {
	Exists bool
	Str    string
	HasStr bool
	Fields []string
}

// Equal reports whether two AccessOutcomes agree on every field. A nil
// Fields equals an empty Fields, consistent with how an empty struct
// reports its members.
func (o AccessOutcome) Equal(other AccessOutcome) bool {
	return o.Exists == other.Exists &&
		o.Str == other.Str &&
		o.HasStr == other.HasStr &&
		slices.Equal(o.Fields, other.Fields)
}

// AccessPath is an accessor strategy: it runs an AccessCase and reports
// the AccessOutcome. The in-house and oracle arms are both AccessPaths.
type AccessPath func(c AccessCase) AccessOutcome

// CueLiteAccess runs an AccessCase through the cue/cuelite façade — the
// in-house arm. A data document that fails to compile yields the zero
// AccessOutcome (no existence, no fields), which the oracle arm matches
// by the same compile guard.
func CueLiteAccess(c AccessCase) AccessOutcome {
	data, err := cuelitepkg.CompileJSON([]byte(c.Data))
	if err != nil {
		return AccessOutcome{}
	}
	var out AccessOutcome
	for _, f := range data.Fields() {
		out.Fields = append(out.Fields, f.Selector)
	}
	slices.Sort(out.Fields)
	leaf, ok := data.LookupPath(cuelitepkg.MakePath(c.Segments...))
	out.Exists = ok
	if ok {
		if s, serr := leaf.String(); serr == nil {
			out.Str = s
			out.HasStr = true
		}
	}
	return out
}

// OracleAccess runs an AccessCase through cuelang.org/go directly — the
// oracle the in-house arm is measured against. It mirrors CueLiteAccess
// step for step: strict-JSON lift (via oracleData, so a duplicate key or
// non-JSON document is rejected identically), Fields() over unquoted
// selectors, and a verbatim-segment LookupPath whose Exists() and String()
// it reads back.
func OracleAccess(c AccessCase) AccessOutcome {
	ctx := cuecontext.New()
	data, err := oracleData(ctx, []byte(c.Data))
	if err != nil {
		return AccessOutcome{}
	}
	var out AccessOutcome
	if iter, ferr := data.Fields(); ferr == nil {
		for iter.Next() {
			out.Fields = append(out.Fields, iter.Selector().Unquoted())
		}
	}
	slices.Sort(out.Fields)
	sels := make([]cue.Selector, len(c.Segments))
	for i, seg := range c.Segments {
		sels[i] = cue.Str(seg)
	}
	leaf := data.LookupPath(cue.MakePath(sels...))
	out.Exists = leaf.Exists()
	if out.Exists {
		if s, serr := leaf.String(); serr == nil {
			out.Str = s
			out.HasStr = true
		}
	}
	return out
}

// CompareAccess runs one AccessCase through both arms and reports a
// failure on t when they disagree. It returns true when they agree.
func CompareAccess(t testing.TB, inHouse, oracle AccessPath, c AccessCase) bool {
	t.Helper()
	got := inHouse(c)
	want := oracle(c)
	if got.Equal(want) {
		return true
	}
	t.Errorf("access case %q: in-house %+v disagrees with oracle %+v", c.Name, got, want)
	return false
}

// RunAccess compares every AccessCase through the in-house and oracle
// arms, reporting each disagreement on t. It is the entry point the
// surface-A/B differential accessor test calls over its corpus.
func RunAccess(t testing.TB, cases []AccessCase) {
	t.Helper()
	for _, c := range cases {
		CompareAccess(t, CueLiteAccess, OracleAccess, c)
	}
}

// accessCorpus returns the representative accessor cases, one row per
// behaviour class the query and schema reads exercise: a present leaf, a
// missing leaf, a nested leaf, a key needing quotes (dotted, hyphenated),
// a non-string leaf, an empty-string leaf, an empty path (the receiver),
// a scalar document with no fields, and an array document.
func accessCorpus() []AccessCase {
	return []AccessCase{
		{"present string leaf", `{"status": "done"}`, []string{"status"}},
		{"missing leaf", `{"status": "done"}`, []string{"absent"}},
		{"nested leaf", `{"meta": {"status": "x"}}`, []string{"meta", "status"}},
		{"nested missing leaf", `{"meta": {"status": "x"}}`, []string{"meta", "absent"}},
		{"hyphenated key", `{"my-key": "v"}`, []string{"my-key"}},
		{"dotted key", `{"a.b": "v"}`, []string{"a.b"}},
		{"non-string leaf", `{"n": 42}`, []string{"n"}},
		{"empty-string leaf", `{"s": ""}`, []string{"s"}},
		{"empty path returns the document", `{"a": 1}`, nil},
		{"scalar document has no fields", `42`, []string{"x"}},
		{"array document", `[1, 2]`, []string{"0"}},
		{"multi-field document", `{"a": 1, "b": "two", "c": true}`, []string{"b"}},
	}
}
