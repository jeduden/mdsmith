package lsp

import (
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// shuffledCopy returns a copy of in, shuffled with a fixed seed so a
// failure reproduces.
func shuffledCopy[T any](in []T, seed int64) []T {
	out := append([]T(nil), in...)
	rand.New(rand.NewSource(seed)).Shuffle(len(out), func(i, j int) {
		out[i], out[j] = out[j], out[i]
	})
	return out
}

// sortOrderSeeds are the shuffles each order test sorts. Every test
// below uses more than 12 elements: pdqsort falls back to insertion
// sort (which is stable) at 12 or fewer, so a smaller input would not
// expose a comparator that leaves distinct values tied.
var sortOrderSeeds = []int64{1, 2, 3, 4, 5}

// TestSortLocations_DeterministicAcrossInputOrders pins that
// sortLocations is a total order over location values. Its
// callers collect locations by ranging over index maps, so the input
// order varies from call to call. Twenty references on one line of one
// file differ only by start column; every shuffle must come back in
// column order.
func TestSortLocations_DeterministicAcrossInputOrders(t *testing.T) {
	want := make([]location, 20)
	for i := range want {
		want[i] = location{
			URI: "file:///a.md",
			Range: Range{
				Start: Position{Line: 3, Character: i},
				End:   Position{Line: 3, Character: i + 1},
			},
		}
	}
	for _, seed := range sortOrderSeeds {
		got := shuffledCopy(want, seed)
		sortLocations(got)
		require.Equal(t, want, got, "seed %d", seed)
	}
}

// TestSortSymbolInformation_DeterministicAcrossInputOrders pins the
// workspace/symbol response order when one file repeats a heading
// ("## Example" under several sections is common). Index.SearchSymbols
// ranges over a map, so the hits arrive in a different order on each
// request; the same-named symbols must still come back in line order.
func TestSortSymbolInformation_DeterministicAcrossInputOrders(t *testing.T) {
	want := make([]symbolInformation, 20)
	for i := range want {
		want[i] = symbolInformation{
			Name: "Example",
			Kind: symbolKindString,
			Location: location{
				URI:   "file:///a.md",
				Range: Range{Start: Position{Line: i * 10}, End: Position{Line: i * 10}},
			},
			ContainerName: "a.md",
		}
	}
	for _, seed := range sortOrderSeeds {
		got := shuffledCopy(want, seed)
		sortSymbolInformation(got)
		require.Equal(t, want, got, "seed %d", seed)
	}
}

// TestSortItems_DeterministicAcrossInputOrders pins sortItems's order:
// by SortText, falling back to Label when SortText is empty, then by
// Label. anchorItems prefixes same-file anchors with "a" and cross-file
// anchors with "b", so both groups appear here next to items that
// carry no SortText.
func TestSortItems_DeterministicAcrossInputOrders(t *testing.T) {
	want := make([]completionItem, 0, 21)
	for i := range 7 {
		label := "h" + strconv.Itoa(i)
		want = append(want, completionItem{Label: label, SortText: "a" + label})
	}
	for i := range 7 {
		label := "h" + strconv.Itoa(i)
		want = append(want, completionItem{Label: label, SortText: "b" + label})
	}
	for i := range 7 {
		want = append(want, completionItem{Label: "c" + strconv.Itoa(i)})
	}
	for _, seed := range sortOrderSeeds {
		got := shuffledCopy(want, seed)
		sortItems(got)
		require.Equal(t, want, got, "seed %d", seed)
	}
}

// TestCompareLocations pins each key of compareLocations in order: URI,
// start line, start column, end line, end column. Each case differs
// from the base location in one key only, and a key must decide the
// result even when a later key points the other way.
func TestCompareLocations(t *testing.T) {
	loc := func(uri string, sl, sc, el, ec int) location {
		return location{URI: uri, Range: Range{
			Start: Position{Line: sl, Character: sc},
			End:   Position{Line: el, Character: ec},
		}}
	}
	base := loc("file:///b.md", 5, 5, 5, 5)
	cases := []struct {
		name  string
		other location
		want  int
	}{
		{"equal", loc("file:///b.md", 5, 5, 5, 5), 0},
		{"URI first", loc("file:///a.md", 9, 9, 9, 9), 1},
		{"URI after", loc("file:///c.md", 0, 0, 0, 0), -1},
		{"start line first", loc("file:///b.md", 4, 9, 9, 9), 1},
		{"start line after", loc("file:///b.md", 6, 0, 0, 0), -1},
		{"start column first", loc("file:///b.md", 5, 4, 9, 9), 1},
		{"start column after", loc("file:///b.md", 5, 6, 0, 0), -1},
		{"end line first", loc("file:///b.md", 5, 5, 4, 9), 1},
		{"end line after", loc("file:///b.md", 5, 5, 6, 0), -1},
		{"end column first", loc("file:///b.md", 5, 5, 5, 4), 1},
		{"end column after", loc("file:///b.md", 5, 5, 5, 6), -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, compareLocations(base, tc.other))
			require.Equal(t, -tc.want, compareLocations(tc.other, base))
		})
	}
}

// TestSortSymbolInformation_KindBreaksFullTie pins the last key: two
// symbols with the same name, file and location but different kinds
// (a front-matter title and a heading can share their text) sort by
// kind, not by the order they arrived in.
func TestSortSymbolInformation_KindBreaksFullTie(t *testing.T) {
	at := location{URI: "file:///a.md"}
	got := []symbolInformation{
		{Name: "Intro", Kind: symbolKindString, Location: at, ContainerName: "a.md"},
		{Name: "Intro", Kind: symbolKindProperty, Location: at, ContainerName: "a.md"},
	}
	sortSymbolInformation(got)
	require.Equal(t, symbolKindProperty, got[0].Kind)
	require.Equal(t, symbolKindString, got[1].Kind)
}
