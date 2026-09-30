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
	t.Run("two edits on one line both apply", func(t *testing.T) {
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

// TestApplyEdits_SameOffsetAndOverlap pins the same-line contract. No
// byte of the original row may be claimed by two edits: a partial
// overlap, a containment, an insert strictly inside another edit's
// range, and two identical replacements are all errors naming the line
// and both ranges (in document order, whatever the input order). Edits
// may touch: an insert at a replacement's start or end and adjacent
// replacements all apply. Zero-width inserts at one offset all apply in
// input order — two identical inserts both land (`abcxxdef`), they are
// not merged — and before a replacement starting at that offset. These
// are LSP's TextEdit rules, so a Plan lands the same here as in an
// editor.
func TestApplyEdits_SameOffsetAndOverlap(t *testing.T) {
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
			name:  "distinct zero-width inserts at one offset land in input order",
			edits: []Edit{mkEdit(0, 3, 3, "x"), mkEdit(0, 3, 3, "y")},
			want:  "abcxydef\n",
		},
		{
			name:  "insert at a replacement's start lands before it",
			edits: []Edit{mkEdit(0, 3, 3, "x"), mkEdit(0, 3, 5, "Y")},
			want:  "abcxYf\n",
		},
		{
			name:  "insert at a replacement's start lands before it whatever the input order",
			edits: []Edit{mkEdit(0, 3, 5, "Y"), mkEdit(0, 3, 3, "x")},
			want:  "abcxYf\n",
		},
		{
			name:  "insert at a replacement's end lands after it",
			edits: []Edit{mkEdit(0, 5, 5, "x"), mkEdit(0, 3, 5, "Y")},
			want:  "abcYxf\n",
		},
		{
			name:  "adjacent replacements both apply",
			edits: []Edit{mkEdit(0, 1, 3, "X"), mkEdit(0, 3, 5, "Y")},
			want:  "aXYf\n",
		},
		{
			name:  "ties keep input order past the insertion-sort cutoff",
			edits: tiedInsertsAfterRowStart(),
			want:  "AabcBCDEFGHIJKLMdef\n",
		},
		{
			name:    "identical same-range replacements are rejected",
			edits:   []Edit{mkEdit(0, 1, 3, "X"), mkEdit(0, 1, 3, "X")},
			wantErr: "edits [1,3) and [1,3) on line 1 overlap",
		},
		{
			name:    "same-range replacements with different text are rejected",
			edits:   []Edit{mkEdit(0, 1, 3, "X"), mkEdit(0, 1, 3, "Y")},
			wantErr: "edits [1,3) and [1,3) on line 1 overlap",
		},
		{
			name:    "partial overlap is rejected",
			edits:   []Edit{mkEdit(0, 1, 4, "X"), mkEdit(0, 2, 5, "Y")},
			wantErr: "edits [1,4) and [2,5) on line 1 overlap",
		},
		{
			name:    "partial overlap names the ranges in document order",
			edits:   []Edit{mkEdit(0, 2, 5, "Y"), mkEdit(0, 1, 4, "X")},
			wantErr: "edits [1,4) and [2,5) on line 1 overlap",
		},
		{
			name:    "containment is rejected",
			edits:   []Edit{mkEdit(0, 1, 6, "X"), mkEdit(0, 2, 3, "")},
			wantErr: "edits [1,6) and [2,3) on line 1 overlap",
		},
		{
			name:    "insert strictly inside a replacement is rejected",
			edits:   []Edit{mkEdit(0, 2, 2, "x"), mkEdit(0, 1, 4, "Y")},
			wantErr: "edits [1,4) and [2,2) on line 1 overlap",
		},
		{
			name:    "overlap is found past a non-overlapping edit",
			edits:   []Edit{mkEdit(0, 0, 1, "A"), mkEdit(0, 2, 5, "B"), mkEdit(0, 4, 6, "C")},
			wantErr: "edits [2,5) and [4,6) on line 1 overlap",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := ApplyEdits([]byte("abcdef\n"), tt.edits)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Nil(t, out)
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
// in order, so only a stable sort lands "B".."M" in input order.
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
