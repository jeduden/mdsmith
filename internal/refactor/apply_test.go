package refactor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mkEdit builds a single-line Edit, keeping the table-style test
// cases below readable.
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
	t.Run("two edits same line apply regardless of input order", func(t *testing.T) {
		// `[a](#x) [b](#y)` → rewrite both fragments. Passed out of
		// order (rightmost first) to confirm ApplyEdits sorts them.
		out, err := ApplyEdits([]byte("[a](#x) [b](#y)\n"), []Edit{
			mkEdit(0, 13, 14, "Y"),
			mkEdit(0, 5, 6, "X"),
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
	t.Run("overlapping edits rejected", func(t *testing.T) {
		_, err := ApplyEdits([]byte("abcdef\n"), []Edit{
			mkEdit(0, 0, 4, "x"),
			mkEdit(0, 2, 6, "y"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "overlapping")
	})
	t.Run("identical-range edits rejected", func(t *testing.T) {
		_, err := ApplyEdits([]byte("abcdef\n"), []Edit{
			mkEdit(0, 1, 3, "x"),
			mkEdit(0, 1, 3, "y"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "overlapping")
	})
	t.Run("adjacent non-overlapping edits both apply", func(t *testing.T) {
		out, err := ApplyEdits([]byte("abcdef\n"), []Edit{
			mkEdit(0, 0, 3, "X"),
			mkEdit(0, 3, 6, "Y"),
		})
		require.NoError(t, err)
		assert.Equal(t, "XY\n", string(out))
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
