package refactor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mkEdit builds a single-line Edit, keeping the table-style
// test cases below readable.
func mkEdit(line, startCh, endCh int, text string) Edit {
	return Edit{
		Range: Range{
			Start: Position{Line: line, Character: startCh},
			End:   Position{Line: line, Character: endCh},
		},
		NewText: text,
	}
}

func TestApplyEdits(t *testing.T) {
	t.Run("single edit", func(t *testing.T) {
		out, err := ApplyEdits([]byte("# Setup\n"), []Edit{mkEdit(0, 2, 7, "Install")})
		require.NoError(t, err)
		assert.Equal(t, "# Install\n", string(out))
	})
	t.Run("two edits same line apply right-to-left", func(t *testing.T) {
		// `[a](#x) [b](#y)` → rewrite both fragments.
		out, err := ApplyEdits([]byte("[a](#x) [b](#y)\n"), []Edit{
			mkEdit(0, 5, 6, "X"),
			mkEdit(0, 13, 14, "Y"),
		})
		require.NoError(t, err)
		assert.Equal(t, "[a](#X) [b](#Y)\n", string(out))
	})
	t.Run("CRLF preserved", func(t *testing.T) {
		out, err := ApplyEdits([]byte("# Setup\r\n"), []Edit{mkEdit(0, 2, 7, "X")})
		require.NoError(t, err)
		assert.Equal(t, "# X\r\n", string(out))
	})
	t.Run("multi-line edit rejected", func(t *testing.T) {
		_, err := ApplyEdits([]byte("a\nb\n"), []Edit{{
			Range: Range{Start: Position{Line: 0}, End: Position{Line: 1}},
		}})
		require.Error(t, err)
	})
	t.Run("line out of range", func(t *testing.T) {
		_, err := ApplyEdits([]byte("a\n"), []Edit{mkEdit(9, 0, 0, "")})
		require.Error(t, err)
	})
}

// TestApplyEdits_SameOffsetAndOverlapCharacterization pins what
// ApplyEdits does today with edits that share a start offset or
// overlap. It adds no checks: every edit is spliced in turn, rightmost
// start first (ties keep input order). Each edit's range is mapped to
// bytes against the original row but spliced into the row the previous
// splice left behind, so a left edit that ends past the shrunk row
// errors rather than clamping. These cases document that contract so a
// refactor cannot change it silently — e.g. two identical zero-width
// inserts both land (`abcxxdef`), they are not merged into one.
func TestApplyEdits_SameOffsetAndOverlapCharacterization(t *testing.T) {
	tests := []struct {
		name    string
		edits   []Edit
		want    string
		wantErr string
	}{
		{
			name:  "identical zero-width inserts both apply",
			edits: []Edit{mkEdit(0, 3, 3, "x"), mkEdit(0, 3, 3, "x")},
			want:  "abcxxdef\n",
		},
		{
			name:  "distinct zero-width inserts at one offset: later edit lands leftmost",
			edits: []Edit{mkEdit(0, 3, 3, "x"), mkEdit(0, 3, 3, "y")},
			want:  "abcyxdef\n",
		},
		{
			name:  "identical same-range replacements both apply",
			edits: []Edit{mkEdit(0, 1, 3, "X"), mkEdit(0, 1, 3, "X")},
			want:  "aXef\n",
		},
		{
			name:  "partial overlap splices sequentially",
			edits: []Edit{mkEdit(0, 1, 4, "X"), mkEdit(0, 2, 5, "Y")},
			want:  "aX\n",
		},
		{
			name:  "ties keep input order past the insertion-sort cutoff",
			edits: tiedInsertsAfterRowStart(),
			want:  "AabcMLKJIHGFEDCBdef\n",
		},
		{
			name:    "overlap whose left edit ends past the shrunk row errors",
			edits:   []Edit{mkEdit(0, 1, 6, "X"), mkEdit(0, 2, 3, "")},
			wantErr: "edit offset [1,6) out of range on line 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := ApplyEdits([]byte("abcdef\n"), tt.edits)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(out))
		})
	}
}

// tiedInsertsAfterRowStart returns a zero-width insert of "A" at offset
// 0 followed by twelve tied zero-width inserts "B".."M" at offset 3.
// Thirteen edits is past the 12-element cutoff below which
// slices.SortFunc falls back to insertion sort and happens to keep ties
// in order, so only a stable sort splices "B".."M" in input order.
func tiedInsertsAfterRowStart() []Edit {
	edits := make([]Edit, 0, 13)
	edits = append(edits, mkEdit(0, 0, 0, "A"))
	for c := 'B'; c <= 'M'; c++ {
		edits = append(edits, mkEdit(0, 3, 3, string(c)))
	}
	return edits
}

// TestApplyEdits_UTF16Offsets pins how ApplyEdits maps an Edit's
// UTF-16 Characters to bytes in the row: é is two bytes but one UTF-16
// unit, and 😀 is four bytes but two units (a surrogate pair). A
// Character equal to the row's UTF-16 length addresses the row end.
func TestApplyEdits_UTF16Offsets(t *testing.T) {
	tests := []struct {
		name string
		src  string
		edit Edit
		want string
	}{
		{"two-byte rune before the edit", "# Café Setup\n", mkEdit(0, 7, 12, "Install"), "# Café Install\n"},
		{"surrogate pair before the edit", "a😀b\n", mkEdit(0, 3, 4, "Z"), "a😀Z\n"},
		{"insert at the row end", "abcd\n", mkEdit(0, 4, 4, "x"), "abcdx\n"},
		{"insert at the row end after a surrogate pair", "a😀b\n", mkEdit(0, 4, 4, "x"), "a😀bx\n"},
		{"replace up to the row end before a CR", "abcd\r\n", mkEdit(0, 2, 4, "x"), "abx\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := ApplyEdits([]byte(tt.src), []Edit{tt.edit})
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(out))
		})
	}
}

// TestApplyEdits_RejectsOutOfRangeCharacters pins that a Character
// outside [0, row length] is an error rather than being clamped to the
// nearer row end: a clamped edit silently rewrites bytes the producer
// never addressed. The row length is counted in UTF-16 units and
// excludes a CRLF file's trailing `\r`.
func TestApplyEdits_RejectsOutOfRangeCharacters(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		edit    Edit
		wantErr string
	}{
		{
			name:    "end past the row",
			src:     "abcd\n",
			edit:    mkEdit(0, 2, 99, "x"),
			wantErr: "edit [2,99) on line 1 is outside the line (length 4)",
		},
		{
			name:    "insert past the row",
			src:     "abcd\n",
			edit:    mkEdit(0, 5, 5, "x"),
			wantErr: "edit [5,5) on line 1 is outside the line (length 4)",
		},
		{
			name:    "negative start",
			src:     "abcd\n",
			edit:    mkEdit(0, -5, 1, "x"),
			wantErr: "edit [-5,1) on line 1 is outside the line (length 4)",
		},
		{
			name:    "length counts UTF-16 units, not bytes",
			src:     "café\n",
			edit:    mkEdit(0, 0, 5, "x"),
			wantErr: "edit [0,5) on line 1 is outside the line (length 4)",
		},
		{
			name:    "a surrogate pair counts two units",
			src:     "a😀b\n",
			edit:    mkEdit(0, 5, 5, "x"),
			wantErr: "edit [5,5) on line 1 is outside the line (length 4)",
		},
		{
			name:    "a CRLF row's trailing CR is not addressable",
			src:     "abcd\r\n",
			edit:    mkEdit(0, 4, 5, ""),
			wantErr: "edit [4,5) on line 1 is outside the line (length 4)",
		},
		{
			name:    "start after end",
			src:     "abcd\n",
			edit:    mkEdit(0, 3, 1, "x"),
			wantErr: "edit [3,1) on line 1 ends before it starts",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ApplyEdits([]byte(tt.src), []Edit{tt.edit})
			require.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestSplitKeepCRAndJoinLF(t *testing.T) {
	src := []byte("a\r\nb\nc")
	segs := splitKeepCR(src)
	assert.Equal(t, [][]byte{[]byte("a\r"), []byte("b"), []byte("c")}, segs)
	assert.Equal(t, src, joinLF(segs))
	// Trailing newline yields a trailing empty segment that round-trips.
	assert.Equal(t, []byte("x\n"), joinLF(splitKeepCR([]byte("x\n"))))
}

// TestApplyEdits_ReportsFirstBadLineDeterministically pins that a plan
// with several bad lines always names the same one — the first in
// document order — rather than whichever line map iteration reached
// first. Lines 2, 4, 6 and 8 each carry an invalid edit, given out of
// order; every run must report line 2.
func TestApplyEdits_ReportsFirstBadLineDeterministically(t *testing.T) {
	src := []byte("l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\n")
	for i := range 100 {
		edits := []Edit{
			mkEdit(7, 2, 1, "x"),
			mkEdit(1, 2, 1, "x"),
			mkEdit(5, 2, 1, "x"),
			mkEdit(3, 2, 1, "x"),
		}
		_, err := ApplyEdits(src, edits)
		require.Error(t, err)
		require.Containsf(t, err.Error(), "line 2", "run %d: %v", i, err)
	}
}
