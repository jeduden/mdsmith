package include

import (
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateHeadingOffset covers the five documented cases:
// missing param, invalid integer, out-of-range, valid value, and
// conflict with heading-level.
func TestValidateHeadingOffset(t *testing.T) {
	t.Run("missing param returns nil", func(t *testing.T) {
		diags := validateHeadingOffset("doc.md", 1, map[string]string{})
		assert.Nil(t, diags)
	})

	t.Run("invalid integer returns diagnostic", func(t *testing.T) {
		params := map[string]string{"heading-offset": "abc"}
		diags := validateHeadingOffset("doc.md", 1, params)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "heading-offset")
		assert.Contains(t, diags[0].Message, "integer between -6 and 6")
	})

	t.Run("out of range +7 returns diagnostic", func(t *testing.T) {
		params := map[string]string{"heading-offset": "7"}
		diags := validateHeadingOffset("doc.md", 1, params)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "heading-offset")
	})

	t.Run("out of range -7 returns diagnostic", func(t *testing.T) {
		params := map[string]string{"heading-offset": "-7"}
		diags := validateHeadingOffset("doc.md", 1, params)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "heading-offset")
	})

	t.Run("valid value returns nil", func(t *testing.T) {
		for _, v := range []string{"-6", "-1", "0", "1", "6"} {
			params := map[string]string{"heading-offset": v}
			diags := validateHeadingOffset("doc.md", 1, params)
			assert.Nil(t, diags, "expected no diagnostic for heading-offset=%q", v)
		}
	})

	t.Run("conflict with heading-level returns diagnostic", func(t *testing.T) {
		params := map[string]string{
			"heading-offset": "2",
			"heading-level":  "absolute",
		}
		diags := validateHeadingOffset("doc.md", 1, params)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "heading-offset")
		assert.Contains(t, diags[0].Message, "heading-level")
	})
}

// TestValidateHeadingLevel covers three documented cases:
// missing param, invalid string, valid numeric/named.
func TestValidateHeadingLevel(t *testing.T) {
	t.Run("missing param returns nil", func(t *testing.T) {
		diags := validateHeadingLevel("doc.md", 1, map[string]string{})
		assert.Nil(t, diags)
	})

	t.Run("invalid string returns diagnostic", func(t *testing.T) {
		params := map[string]string{"heading-level": "top"}
		diags := validateHeadingLevel("doc.md", 1, params)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "heading-level")
	})

	t.Run("out of range integer returns diagnostic", func(t *testing.T) {
		for _, v := range []string{"0", "7", "-1"} {
			params := map[string]string{"heading-level": v}
			diags := validateHeadingLevel("doc.md", 1, params)
			require.Len(t, diags, 1, "expected diagnostic for heading-level=%q", v)
			assert.Contains(t, diags[0].Message, "heading-level")
		}
	})

	t.Run("valid named absolute returns nil", func(t *testing.T) {
		params := map[string]string{"heading-level": "absolute"}
		diags := validateHeadingLevel("doc.md", 1, params)
		assert.Nil(t, diags)
	})

	t.Run("valid numeric 1..6 returns nil", func(t *testing.T) {
		for _, v := range []string{"1", "2", "3", "4", "5", "6"} {
			params := map[string]string{"heading-level": v}
			diags := validateHeadingLevel("doc.md", 1, params)
			assert.Nil(t, diags, "expected no diagnostic for heading-level=%q", v)
		}
	})
}

// TestValidateExtractParam covers the absent-param, empty-value, and
// valid-value branches of validateExtractParam.
func TestValidateExtractParam(t *testing.T) {
	t.Run("no extract param returns nil", func(t *testing.T) {
		diags := validateExtractParam("doc.md", 1, map[string]string{})
		assert.Nil(t, diags)
	})

	t.Run("empty extract value returns diagnostic", func(t *testing.T) {
		params := map[string]string{"extract": "   "}
		diags := validateExtractParam("doc.md", 1, params)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "extract")
		assert.Contains(t, diags[0].Message, "empty")
	})

	t.Run("valid extract with only file param returns nil", func(t *testing.T) {
		params := map[string]string{
			"extract": "tagline",
			"file":    "other.md",
		}
		diags := validateExtractParam("doc.md", 1, params)
		assert.Nil(t, diags)
	})
}

// TestValidateExtractParam_Conflicts covers the four incompatible-parameter
// branches: strip-frontmatter, heading-level, heading-offset, wrap, source-dir.
func TestValidateExtractParam_Conflicts(t *testing.T) {
	t.Run("extract conflicts with strip-frontmatter", func(t *testing.T) {
		params := map[string]string{
			"extract":           "tagline",
			"strip-frontmatter": "true",
		}
		diags := validateExtractParam("doc.md", 1, params)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "extract")
		assert.Contains(t, diags[0].Message, "strip-frontmatter")
	})

	t.Run("extract conflicts with heading-level", func(t *testing.T) {
		params := map[string]string{
			"extract":       "tagline",
			"heading-level": "absolute",
		}
		diags := validateExtractParam("doc.md", 1, params)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "extract")
		assert.Contains(t, diags[0].Message, "heading-level")
	})

	t.Run("extract conflicts with heading-offset", func(t *testing.T) {
		params := map[string]string{
			"extract":        "tagline",
			"heading-offset": "1",
		}
		diags := validateExtractParam("doc.md", 1, params)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "extract")
		assert.Contains(t, diags[0].Message, "heading-offset")
	})

	t.Run("extract conflicts with wrap", func(t *testing.T) {
		params := map[string]string{
			"extract": "tagline",
			"wrap":    "go",
		}
		diags := validateExtractParam("doc.md", 1, params)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "extract")
		assert.Contains(t, diags[0].Message, "wrap")
	})

	t.Run("extract conflicts with source-dir", func(t *testing.T) {
		params := map[string]string{
			"extract":    "tagline",
			"source-dir": "docs/",
		}
		diags := validateExtractParam("doc.md", 1, params)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "extract")
		assert.Contains(t, diags[0].Message, "source-dir")
	})
}

// TestFindParentHeadingLevel covers two documented cases:
// no heading above the marker (returns 0), and a heading found before the marker.
func TestFindParentHeadingLevel(t *testing.T) {
	t.Run("no heading above marker returns 0", func(t *testing.T) {
		// Marker is on line 1, before any headings.
		src := "Some plain text.\n\n# Heading after\n"
		f, err := lint.NewFile("doc.md", []byte(src))
		require.NoError(t, err)

		// Line 1 is before the heading on line 3.
		level := findParentHeadingLevel(f, 1)
		assert.Equal(t, 0, level)
	})

	t.Run("single heading before marker returns its level", func(t *testing.T) {
		// H2 on line 1, marker on line 5.
		src := "## Section\n\nSome text.\n\nContent here.\n"
		f, err := lint.NewFile("doc.md", []byte(src))
		require.NoError(t, err)

		level := findParentHeadingLevel(f, 5)
		assert.Equal(t, 2, level)
	})

	t.Run("multiple headings before marker returns last one's level", func(t *testing.T) {
		// H1 on line 1, H3 on line 5, marker on line 9.
		src := "# Top\n\nText.\n\n### Deep\n\nMore text.\n\nContent.\n"
		f, err := lint.NewFile("doc.md", []byte(src))
		require.NoError(t, err)

		level := findParentHeadingLevel(f, 9)
		assert.Equal(t, 3, level)
	})

	t.Run("heading exactly on marker line is not counted", func(t *testing.T) {
		// H2 starts on the same line as the marker.
		src := "## Heading\n\n"
		f, err := lint.NewFile("doc.md", []byte(src))
		require.NoError(t, err)

		// Marker is on line 1 — the heading itself.
		level := findParentHeadingLevel(f, 1)
		assert.Equal(t, 0, level)
	})

	t.Run("heading found at depth with filesystem", func(t *testing.T) {
		// Simulate a real include scenario: a file with a heading,
		// tested with an optional FS attached.
		fsys := fstest.MapFS{
			"data.md": {Data: []byte("# Data heading\n")},
		}
		src := "# Parent\n\nSome text.\n\nInclude here.\n"
		f, err := lint.NewFile("doc.md", []byte(src))
		require.NoError(t, err)
		f.FS = fsys
		f.RootFS = fsys

		// The heading "# Parent" is on line 1; marker at line 5.
		level := findParentHeadingLevel(f, 5)
		assert.Equal(t, 1, level)
	})
}
