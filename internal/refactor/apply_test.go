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
	t.Run("offset out of range", func(t *testing.T) {
		// Start past End after mapping → the s>en guard fires.
		_, err := ApplyEdits([]byte("abcd\n"), []Edit{mkEdit(0, 3, 1, "x")})
		require.Error(t, err)
	})
}

// TestApplyEdits_SameOffsetAndOverlapCharacterization pins what
// ApplyEdits does today with edits that share a start offset or
// overlap. It adds no checks: every edit is spliced in turn, rightmost
// start first (ties keep input order), each against the row the
// previous splice left behind. These cases document that contract so a
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

func TestSplitKeepCRAndJoinLF(t *testing.T) {
	src := []byte("a\r\nb\nc")
	segs := splitKeepCR(src)
	assert.Equal(t, [][]byte{[]byte("a\r"), []byte("b"), []byte("c")}, segs)
	assert.Equal(t, src, joinLF(segs))
	// Trailing newline yields a trailing empty segment that round-trips.
	assert.Equal(t, []byte("x\n"), joinLF(splitKeepCR([]byte("x\n"))))
}
