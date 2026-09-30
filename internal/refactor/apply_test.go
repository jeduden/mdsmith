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
		require.EqualError(t, err, "edit on line 1 ends on another line; multi-line edits are not supported")
	})
	t.Run("line past the end of the file", func(t *testing.T) {
		_, err := ApplyEdits([]byte("a\n"), []Edit{mkEdit(9, 0, 0, "")})
		require.EqualError(t, err, "edit on line 10 is past the end of the file")
	})
	t.Run("leaves src and the caller's edits unchanged", func(t *testing.T) {
		src := []byte("abcdef\n")
		edits := []Edit{mkEdit(0, 4, 5, "E"), mkEdit(0, 1, 2, "B")}
		out, err := ApplyEdits(src, edits)
		require.NoError(t, err)
		assert.Equal(t, "aBcdEf\n", string(out))
		assert.Equal(t, "abcdef\n", string(src))
		assert.Equal(t, []Edit{mkEdit(0, 4, 5, "E"), mkEdit(0, 1, 2, "B")}, edits)
	})
	t.Run("negative line", func(t *testing.T) {
		_, err := ApplyEdits([]byte("a\n"), []Edit{mkEdit(-1, 0, 0, "")})
		require.EqualError(t, err, "edit line index -1 is negative")
	})
}

// TestApplyEdits_TouchingAndSameOffsetEdits pins the edits that may
// share a line. Edits may touch: an insert at a replacement's start or
// end and adjacent replacements all apply. Zero-width inserts at one
// offset all apply in input order — two identical inserts both land
// (`abcxxdef`), they are not merged — and before a replacement
// starting at that offset. These are LSP's TextEdit rules, so a Plan
// lands the same here as in an editor.
func TestApplyEdits_TouchingAndSameOffsetEdits(t *testing.T) {
	tests := []struct {
		name  string
		edits []Edit
		want  string
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := ApplyEdits([]byte("abcdef\n"), tt.edits)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(out))
		})
	}
}

// TestApplyEdits_RejectsOverlap pins that no byte of the original row
// may be claimed by two edits: a partial overlap, a containment, an
// insert strictly inside another edit's range, and two identical
// replacements are all errors naming the line and both ranges (in
// document order, whatever the input order), with no output.
func TestApplyEdits_RejectsOverlap(t *testing.T) {
	tests := []struct {
		name    string
		edits   []Edit
		wantErr string
	}{
		{
			name:    "identical same-range replacements",
			edits:   []Edit{mkEdit(0, 1, 3, "X"), mkEdit(0, 1, 3, "X")},
			wantErr: "edits [1,3) and [1,3) on line 1 overlap",
		},
		{
			name:    "same-range replacements with different text",
			edits:   []Edit{mkEdit(0, 1, 3, "X"), mkEdit(0, 1, 3, "Y")},
			wantErr: "edits [1,3) and [1,3) on line 1 overlap",
		},
		{
			name:    "partial overlap",
			edits:   []Edit{mkEdit(0, 1, 4, "X"), mkEdit(0, 2, 5, "Y")},
			wantErr: "edits [1,4) and [2,5) on line 1 overlap",
		},
		{
			name:    "partial overlap given right edit first",
			edits:   []Edit{mkEdit(0, 2, 5, "Y"), mkEdit(0, 1, 4, "X")},
			wantErr: "edits [1,4) and [2,5) on line 1 overlap",
		},
		{
			name:    "containment",
			edits:   []Edit{mkEdit(0, 1, 6, "X"), mkEdit(0, 2, 3, "")},
			wantErr: "edits [1,6) and [2,3) on line 1 overlap",
		},
		{
			name:    "insert strictly inside a replacement",
			edits:   []Edit{mkEdit(0, 2, 2, "x"), mkEdit(0, 1, 4, "Y")},
			wantErr: "edits [1,4) and [2,2) on line 1 overlap",
		},
		{
			name:    "overlap past a non-overlapping edit",
			edits:   []Edit{mkEdit(0, 0, 1, "A"), mkEdit(0, 2, 5, "B"), mkEdit(0, 4, 6, "C")},
			wantErr: "edits [2,5) and [4,6) on line 1 overlap",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := ApplyEdits([]byte("abcdef\n"), tt.edits)
			require.EqualError(t, err, tt.wantErr)
			assert.Nil(t, out)
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

// TestApplyEdits_RejectsSurrogatePairSplit pins that a Character
// between the two UTF-16 units of a surrogate pair is an error. No
// byte offset addresses it, and rounding it to the next rune boundary
// would let two distinct Characters splice at one byte: on `😀x`,
// edits [0,1) and [1,2) both passed the range and overlap checks, then
// landed at byte 4.
func TestApplyEdits_RejectsSurrogatePairSplit(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		edits   []Edit
		wantErr string
	}{
		{
			name:    "two edits meeting inside the pair",
			src:     "😀x\n",
			edits:   []Edit{mkEdit(0, 0, 1, "A"), mkEdit(0, 1, 2, "B")},
			wantErr: "edit [0,1) on line 1 splits a surrogate pair at character 1",
		},
		{
			name:    "an insert inside the pair",
			src:     "a😀b\n",
			edits:   []Edit{mkEdit(0, 2, 2, "x")},
			wantErr: "edit [2,2) on line 1 splits a surrogate pair at character 2",
		},
		{
			name:    "a start inside the pair",
			src:     "a😀b\n",
			edits:   []Edit{mkEdit(0, 2, 4, "x")},
			wantErr: "edit [2,4) on line 1 splits a surrogate pair at character 2",
		},
		{
			name:    "an end inside the pair",
			src:     "a😀b\n",
			edits:   []Edit{mkEdit(0, 0, 2, "x")},
			wantErr: "edit [0,2) on line 1 splits a surrogate pair at character 2",
		},
		{
			name:    "an end inside a pair after an earlier edit",
			src:     "ab😀\r\n",
			edits:   []Edit{mkEdit(0, 0, 1, "A"), mkEdit(0, 1, 3, "B")},
			wantErr: "edit [1,3) on line 1 splits a surrogate pair at character 3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := ApplyEdits([]byte(tt.src), tt.edits)
			require.EqualError(t, err, tt.wantErr)
			assert.Nil(t, out)
		})
	}
	t.Run("edits at the pair's edges apply", func(t *testing.T) {
		out, err := ApplyEdits([]byte("a😀b\n"), []Edit{
			mkEdit(0, 1, 1, "<"), mkEdit(0, 1, 3, "X"), mkEdit(0, 3, 3, ">"),
		})
		require.NoError(t, err)
		assert.Equal(t, "a<X>b\n", string(out))
	})
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
// document order — whatever kind of error each line holds and in
// whatever order the edits arrive. Every case runs 100 times so a
// check that followed map iteration order would flake.
func TestApplyEdits_ReportsFirstBadLineDeterministically(t *testing.T) {
	src := []byte("l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\n")
	multiLine := func(line int) Edit {
		return Edit{Range: Range{Start: Position{Line: line}, End: Position{Line: line + 1}}}
	}
	tests := []struct {
		name    string
		edits   []Edit
		wantErr string
	}{
		{
			name:    "four inverted ranges given out of order",
			edits:   []Edit{mkEdit(7, 2, 1, "x"), mkEdit(1, 2, 1, "x"), mkEdit(5, 2, 1, "x"), mkEdit(3, 2, 1, "x")},
			wantErr: "edit [2,1) on line 2 ends before it starts",
		},
		{
			name:    "a multi-line edit given first on a later line",
			edits:   []Edit{multiLine(8), mkEdit(1, 0, 2, "x"), mkEdit(1, 1, 2, "y")},
			wantErr: "edits [0,2) and [1,2) on line 2 overlap",
		},
		{
			name:    "a multi-line edit given last on an earlier line",
			edits:   []Edit{mkEdit(5, 0, 9, "x"), mkEdit(3, 2, 1, "x"), multiLine(1)},
			wantErr: "edit on line 2 ends on another line; multi-line edits are not supported",
		},
		{
			name:    "a line past the end given first",
			edits:   []Edit{mkEdit(40, 0, 0, "x"), mkEdit(2, 0, 9, "x")},
			wantErr: "edit [0,9) on line 3 is outside the line (length 2)",
		},
		{
			name:    "a negative line sorts before every other line",
			edits:   []Edit{mkEdit(0, 2, 1, "x"), mkEdit(-3, 0, 0, "x")},
			wantErr: "edit line index -3 is negative",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range 100 {
				_, err := ApplyEdits(src, tt.edits)
				require.EqualErrorf(t, err, tt.wantErr, "run %d", i)
			}
		})
	}
}

func TestSpliceLine(t *testing.T) {
	t.Run("copies the bytes around and between edits", func(t *testing.T) {
		out, err := spliceLine([]byte("abcdef"), []Edit{mkEdit(4, 4, 5, "E"), mkEdit(4, 1, 2, "B")}, 4)
		require.NoError(t, err)
		assert.Equal(t, "aBcdEf", string(out))
	})
	t.Run("keeps a trailing CR outside the edited row", func(t *testing.T) {
		out, err := spliceLine([]byte("abc\r"), []Edit{mkEdit(0, 3, 3, "d")}, 0)
		require.NoError(t, err)
		assert.Equal(t, "abcd\r", string(out))
	})
	t.Run("an empty row accepts an insert", func(t *testing.T) {
		out, err := spliceLine(nil, []Edit{mkEdit(0, 0, 0, "x")}, 0)
		require.NoError(t, err)
		assert.Equal(t, "x", string(out))
	})
	t.Run("names the one-based line in an overlap", func(t *testing.T) {
		_, err := spliceLine([]byte("abcdef"), []Edit{mkEdit(4, 1, 3, "X"), mkEdit(4, 2, 4, "Y")}, 4)
		require.EqualError(t, err, "edits [1,3) and [2,4) on line 5 overlap")
	})
	t.Run("names the one-based line in a range error", func(t *testing.T) {
		_, err := spliceLine([]byte("ab"), []Edit{mkEdit(4, 0, 3, "X")}, 4)
		require.EqualError(t, err, "edit [0,3) on line 5 is outside the line (length 2)")
	})
}

func TestCheckEditRange(t *testing.T) {
	tests := []struct {
		name       string
		start, end int
		wantErr    string
	}{
		{"whole row", 0, 4, ""},
		{"insert at the row start", 0, 0, ""},
		{"insert at the row end", 4, 4, ""},
		{"start after end", 3, 2, "edit [3,2) on line 2 ends before it starts"},
		{"negative start", -1, 2, "edit [-1,2) on line 2 is outside the line (length 4)"},
		{"end past the row", 2, 5, "edit [2,5) on line 2 is outside the line (length 4)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkEditRange(mkEdit(1, tt.start, tt.end, "x"), 4, 1)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestCheckEditLine(t *testing.T) {
	multiLine := Edit{Range: Range{Start: Position{Line: 1}, End: Position{Line: 2}}}
	tests := []struct {
		name    string
		line    int
		es      []Edit
		wantErr string
	}{
		{"first line", 0, []Edit{mkEdit(0, 0, 1, "x")}, ""},
		{"last segment", 2, []Edit{mkEdit(2, 0, 0, "x")}, ""},
		{"negative line index", -1, []Edit{mkEdit(-1, 0, 0, "x")}, "edit line index -1 is negative"},
		{"one past the last segment", 3, []Edit{mkEdit(3, 0, 0, "x")}, "edit on line 4 is past the end of the file"},
		{
			"an edit ending on the next line",
			1, []Edit{mkEdit(1, 0, 1, "x"), multiLine},
			"edit on line 2 ends on another line; multi-line edits are not supported",
		},
		{
			"an edit ending on an earlier line",
			1, []Edit{{Range: Range{Start: Position{Line: 1}, End: Position{Line: 0}}}},
			"edit on line 2 ends on another line; multi-line edits are not supported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkEditLine(tt.line, 3, tt.es)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, tt.wantErr)
		})
	}
}

// TestUTF16Cursor_Seek pins that the cursor maps Characters to bytes
// moving forward only — each seek resumes where the last one stopped —
// and reports a Character inside a surrogate pair as unaddressable.
func TestUTF16Cursor_Seek(t *testing.T) {
	row := []byte("aé😀b") // bytes: a=1 é=2 😀=4 b=1; units: 1 1 2 1
	t.Run("maps each rune boundary to its byte offset", func(t *testing.T) {
		c := utf16Cursor{row: row}
		for _, tc := range []struct{ ch, b int }{{0, 0}, {1, 1}, {2, 3}, {4, 7}, {5, 8}} {
			b, ok := c.seek(tc.ch)
			require.Truef(t, ok, "character %d", tc.ch)
			assert.Equalf(t, tc.b, b, "character %d", tc.ch)
		}
	})
	t.Run("resumes from the previous seek", func(t *testing.T) {
		c := utf16Cursor{row: row}
		_, _ = c.seek(2)
		assert.Equal(t, utf16Cursor{row: row, b: 3, u: 2}, c)
		b, ok := c.seek(2)
		assert.True(t, ok)
		assert.Equal(t, 3, b)
	})
	t.Run("a Character inside a surrogate pair stops after the pair", func(t *testing.T) {
		c := utf16Cursor{row: row}
		b, ok := c.seek(3)
		assert.False(t, ok)
		assert.Equal(t, 7, b)
	})
	t.Run("an invalid byte counts one unit", func(t *testing.T) {
		c := utf16Cursor{row: []byte("\xffz")}
		b, ok := c.seek(1)
		assert.True(t, ok)
		assert.Equal(t, 1, b)
	})
}

func TestUTF16Cursor_EditBytes(t *testing.T) {
	row := []byte("a😀b")
	tests := []struct {
		name               string
		edit               Edit
		wantStart, wantEnd int
		wantErr            string
	}{
		{"a range around the pair", mkEdit(2, 1, 3, "x"), 1, 5, ""},
		{"an insert after the pair", mkEdit(2, 3, 3, "x"), 5, 5, ""},
		{
			"a start inside the pair", mkEdit(2, 2, 3, "x"), 0, 0,
			"edit [2,3) on line 3 splits a surrogate pair at character 2",
		},
		{
			"an end inside the pair", mkEdit(2, 1, 2, "x"), 0, 0,
			"edit [1,2) on line 3 splits a surrogate pair at character 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := utf16Cursor{row: row}
			start, end, err := c.editBytes(tt.edit, 2)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, [2]int{tt.wantStart, tt.wantEnd}, [2]int{start, end})
		})
	}
}
