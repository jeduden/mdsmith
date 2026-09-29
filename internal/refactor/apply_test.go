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
	t.Run("identical duplicate edits collapse into one", func(t *testing.T) {
		// A caller-side index occasionally reports the same rewrite
		// twice (e.g. an anchor edit and a path edit resolving to the
		// same range); that must apply once, not error.
		out, err := ApplyEdits([]byte("[a](old.md)\n"), []Edit{
			mkEdit(0, 4, 10, "new.md"),
			mkEdit(0, 4, 10, "new.md"),
		})
		require.NoError(t, err)
		assert.Equal(t, "[a](new.md)\n", string(out))
	})
	t.Run("adjacent non-overlapping edits both apply", func(t *testing.T) {
		out, err := ApplyEdits([]byte("abcdef\n"), []Edit{
			mkEdit(0, 0, 3, "X"),
			mkEdit(0, 3, 6, "Y"),
		})
		require.NoError(t, err)
		assert.Equal(t, "XY\n", string(out))
	})
	t.Run("zero-width insert at another edit's start applies regardless of input order", func(t *testing.T) {
		// insert "PRE-" at [3,3), replace [3,6) with "REST" — both share
		// Start.Character 3, so the End tie-break (not input order) must
		// decide which comes first.
		insert := mkEdit(0, 3, 3, "PRE-")
		replace := mkEdit(0, 3, 6, "REST")
		out1, err := ApplyEdits([]byte("abcdef\n"), []Edit{insert, replace})
		require.NoError(t, err)
		out2, err := ApplyEdits([]byte("abcdef\n"), []Edit{replace, insert})
		require.NoError(t, err)
		assert.Equal(t, "abcPRE-REST\n", string(out1))
		assert.Equal(t, string(out1), string(out2))
	})
	t.Run("character past end of line clamps to EOL", func(t *testing.T) {
		// mdtext.UTF16ToByteOffset clamps an out-of-range Character to
		// len(row); ApplyEdits relies on that clamp instead of its own
		// bounds check.
		out, err := ApplyEdits([]byte("abc\n"), []Edit{mkEdit(0, 1, 99, "X")})
		require.NoError(t, err)
		assert.Equal(t, "aX\n", string(out))
	})
}

func TestApplyEdits_Errors(t *testing.T) {
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
	t.Run("same-range edits with different NewText rejected", func(t *testing.T) {
		_, err := ApplyEdits([]byte("abcdef\n"), []Edit{
			mkEdit(0, 1, 3, "x"),
			mkEdit(0, 1, 3, "y"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "overlapping")
	})
	t.Run("error is deterministic across multiple bad lines", func(t *testing.T) {
		// Both line 0 and line 1 have a reversed Start/End pair; the
		// reported error must always name the earliest bad line, not
		// whichever one a randomized map iteration visits first.
		for i := 0; i < 5; i++ {
			_, err := ApplyEdits([]byte("a\nb\nc\n"), []Edit{
				mkEdit(1, 1, 0, "y"),
				mkEdit(0, 1, 0, "x"),
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "on line 1")
		}
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
