package release

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/rules"
)

// TestRenderCoverageMatrix_TablePerCategory verifies that
// RenderCoverageMatrix groups rules by category, emits one
// five-column table per category that has peer mappings, and
// renders each peer cell with the upstream default-on indicator.
func TestRenderCoverageMatrix_TablePerCategory(t *testing.T) {
	rs := []rules.RuleInfo{
		{
			ID: "MDS001", Name: "line-length", Status: "ready",
			Description: "Line exceeds maximum length.",
			Category:    "line",
			Markdownlint: []rules.RuleMapping{
				{ID: "MD013", Name: "line-length", Default: true},
			},
			Rumdl: []rules.RuleMapping{
				{ID: "MD013", Name: "line-length", Default: true},
			},
			Mado: []rules.RuleMapping{
				{ID: "MD013", Name: "line-length", Default: true},
			},
		},
		{
			ID: "MDS019", Name: "catalog", Status: "ready",
			Description: "Catalog directive.",
			Category:    "directive",
		},
	}
	out := RenderCoverageMatrix(rs)
	assert.Contains(t, out, "## Line length")
	assert.Contains(t, out,
		"[MDS001](../../../internal/rules/MDS001-line-length/README.md) line-length")
	assert.Contains(t, out, "MD013 ✅ line-length")
	assert.Contains(t, out, "## Generated sections (directives) (mdsmith-only)")
	assert.Contains(t, out,
		"[MDS019](../../../internal/rules/MDS019-catalog/README.md) catalog")
	assert.Contains(t, out, "| Catalog directive.")
	// Table rows are padded to the widest cell so output passes
	// MDS025 table-format without a downstream fix pass.
	assert.NotContains(t, out, " | —|", "cells should have a trailing space before the pipe")
	assert.False(t, strings.HasSuffix(out, "\n\n"),
		"file ends with a single trailing newline, not a blank line")
}

// TestRenderCoverageMatrix_PeerDefaults verifies that an entry
// marked default:false renders with the off-by-default marker,
// and that partial coverage suffixes with "(partial)".
func TestRenderCoverageMatrix_PeerDefaults(t *testing.T) {
	rs := []rules.RuleInfo{
		{
			ID: "MDS064", Name: "atx-heading-whitespace", Status: "ready",
			Description: "ATX whitespace.",
			Category:    "heading",
			Markdownlint: []rules.RuleMapping{
				{ID: "MD020", Name: "no-missing-space-closed-atx",
					Default: true, Partial: true},
			},
			Rumdl: []rules.RuleMapping{
				{ID: "MD020", Name: "no-space-closed-atx", Default: false},
			},
		},
	}
	out := RenderCoverageMatrix(rs)
	assert.Contains(t, out, "MD020 ✅ no-missing-space-closed-atx (partial)")
	assert.Contains(t, out, "MD020 ⚪ no-space-closed-atx")
}

// TestRenderCoverageMatrix_DeterministicAcrossRuns verifies
// that two renderings of the same input slice produce byte-
// identical output. Drift checking depends on this property.
func TestRenderCoverageMatrix_DeterministicAcrossRuns(t *testing.T) {
	rs := []rules.RuleInfo{
		{ID: "MDS003", Name: "heading-increment", Status: "ready", Category: "heading"},
		{ID: "MDS001", Name: "line-length", Status: "ready", Category: "line"},
	}
	a := RenderCoverageMatrix(rs)
	b := RenderCoverageMatrix(rs)
	assert.Equal(t, a, b)
}

// TestApplyCoverageMatrix_PropagatesListRulesError verifies the
// stub-listRules-fails branch in ApplyCoverageMatrix. The
// real embed.FS-backed listRules cannot fail in practice, so
// this is the only way to exercise the error-propagation path.
func TestApplyCoverageMatrix_PropagatesListRulesError(t *testing.T) {
	prev := listRules
	t.Cleanup(func() { listRules = prev })
	listRules = func() ([]rules.RuleInfo, error) {
		return nil, errStubListRules
	}
	_, err := ApplyCoverageMatrix(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading rule metadata")
}

// TestCheckCoverageMatrix_PropagatesListRulesError verifies
// the same listRules-fails branch in CheckCoverageMatrix.
func TestCheckCoverageMatrix_PropagatesListRulesError(t *testing.T) {
	prev := listRules
	t.Cleanup(func() { listRules = prev })
	listRules = func() ([]rules.RuleInfo, error) {
		return nil, errStubListRules
	}
	_, err := CheckCoverageMatrix(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading rule metadata")
}

// TestApplyCoverageMatrix_PropagatesReadError drives the
// non-NotExist ReadFile error: a directory at the target file
// path makes os.ReadFile fail with EISDIR, and Apply must
// surface the error rather than treating it as "file absent".
func TestApplyCoverageMatrix_PropagatesReadError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(root, CoverageMatrixFile), 0o755,
	))
	_, err := ApplyCoverageMatrix(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading existing matrix")
}

// The chmod-based tests that drive ApplyCoverageMatrix's
// MkdirAll and WriteFile error branches live in
// coverage_chmod_unix_test.go (build-tagged !windows)
// because os.Geteuid is not available on Windows.

// TestFormatCoverageDrift_HaveLongerThanWant drives the n =
// len(wLines) branch (when on-disk has more lines than the
// generator's output and every overlapping line matches).
func TestFormatCoverageDrift_HaveLongerThanWant(t *testing.T) {
	msg := formatCoverageDrift("a\nb\nc", "a\nb")
	assert.Contains(t, msg, "file has 3 lines, expected 2")
}

// errStubListRules is the sentinel returned by the listRules
// seam in the error-path tests above so the assertions can
// identify it without coupling to the wrapping message.
var errStubListRules = errors.New("stub listRules failure")

// TestRenderPeerTable_PrintsFiveColumnHeader exercises the
// five-column header + body shape directly so a refactor to
// the header row can be unit-caught without re-running the
// whole RenderCoverageMatrix pipeline.
func TestRenderPeerTable_PrintsFiveColumnHeader(t *testing.T) {
	var buf bytes.Buffer
	renderPeerTable(&buf, []rules.RuleInfo{
		{ID: "MDS001", Name: "line-length", Category: "line",
			Markdownlint: []rules.RuleMapping{{ID: "MD013",
				Name: "line-length", Default: true}}},
	})
	out := buf.String()
	assert.Contains(t, out, "| mdsmith")
	assert.Contains(t, out, "| markdownlint")
	assert.Contains(t, out, "| rumdl")
	assert.Contains(t, out, "| mado")
	assert.Contains(t, out, "| panache")
	assert.Contains(t, out, "MD013 ✅ line-length")
}

// TestRenderMdsmithOnlyTable_PrintsDescriptionAsSecondColumn
// asserts the two-column shape used for categories with no
// peer-linter mappings.
func TestRenderMdsmithOnlyTable_PrintsDescriptionAsSecondColumn(t *testing.T) {
	var buf bytes.Buffer
	renderMdsmithOnlyTable(&buf, []rules.RuleInfo{
		{ID: "MDS019", Name: "catalog",
			Description: "Catalog directive.", Category: "directive"},
	})
	out := buf.String()
	assert.Contains(t, out, "| mdsmith")
	assert.Contains(t, out, "| What it adds")
	assert.Contains(t, out, "| Catalog directive. |")
}

// TestWritePaddedTable_PadsEveryColumnToWidestCell verifies
// that every cell in a column ends padded to the same width,
// the property MDS025 (table-format) requires.
func TestWritePaddedTable_PadsEveryColumnToWidestCell(t *testing.T) {
	var buf bytes.Buffer
	writePaddedTable(&buf,
		[]string{"a", "bb"},
		[][]string{{"xxx", "y"}, {"z", "wwww"}},
	)
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.Len(t, lines, 4) // header + separator + 2 body rows
	// Each row has equal length once padded, since columns share
	// max widths across rows.
	for _, l := range lines[1:] {
		assert.Equal(t, len(lines[0]), len(l),
			"row %q should match header width %d", l, len(lines[0]))
	}
}

// TestRenderMdsmithCell_LinksReadmeAndAppendsNotReady marks
// experimental (`status: not-ready`) rules so a reader sees
// the maturity flag without leaving the table.
func TestRenderMdsmithCell_LinksReadmeAndAppendsNotReady(t *testing.T) {
	ready := renderMdsmithCell(rules.RuleInfo{
		ID: "MDS001", Name: "line-length", Status: "ready",
	})
	assert.Equal(t,
		"[MDS001](../../../internal/rules/MDS001-line-length/README.md) line-length",
		ready)
	notReady := renderMdsmithCell(rules.RuleInfo{
		ID: "MDS029", Name: "conciseness-scoring", Status: "not-ready",
	})
	assert.Contains(t, notReady, "(not-ready)")
}

// TestRenderPeerCell_FormatsAllStates exercises every combination
// the renderer surfaces: no analog (long dash), default-on, off
// by default, partial coverage, and comma-joined multi-entry.
func TestRenderPeerCell_FormatsAllStates(t *testing.T) {
	assert.Equal(t, "—", renderPeerCell(nil))
	assert.Equal(t, "MD013 ✅ line-length",
		renderPeerCell([]rules.RuleMapping{{ID: "MD013",
			Name: "line-length", Default: true}}))
	assert.Equal(t, "MD020 ⚪ no-space-closed-atx",
		renderPeerCell([]rules.RuleMapping{{ID: "MD020",
			Name: "no-space-closed-atx", Default: false}}))
	assert.Equal(t, "MD056 ✅ table-column-count (partial)",
		renderPeerCell([]rules.RuleMapping{{ID: "MD056",
			Name: "table-column-count", Default: true, Partial: true}}))
	assert.Equal(t, "MD055 ✅ table-pipe-style, MD058 ✅ blanks-around-tables",
		renderPeerCell([]rules.RuleMapping{
			{ID: "MD055", Name: "table-pipe-style", Default: true},
			{ID: "MD058", Name: "blanks-around-tables", Default: true},
		}))
}

// TestRenderPeerCell_CollapsesWhenIDEqualsName verifies that when a
// peer's rule ID and name are identical — the obsidian-linter and
// panache convention — the cell renders the token once rather than
// duplicating it (e.g. "consecutive-blank-lines ⚪", not
// "consecutive-blank-lines ⚪ consecutive-blank-lines"). The
// "(partial)" suffix still appends after the collapsed form.
func TestRenderPeerCell_CollapsesWhenIDEqualsName(t *testing.T) {
	assert.Equal(t, "consecutive-blank-lines ⚪",
		renderPeerCell([]rules.RuleMapping{{ID: "consecutive-blank-lines",
			Name: "consecutive-blank-lines", Default: false}}))
	assert.Equal(t, "undefined-anchor ✅",
		renderPeerCell([]rules.RuleMapping{{ID: "undefined-anchor",
			Name: "undefined-anchor", Default: true}}))
	assert.Equal(t, "headings-start-line ⚪ (partial)",
		renderPeerCell([]rules.RuleMapping{{ID: "headings-start-line",
			Name: "headings-start-line", Default: false, Partial: true}}))
}

// TestGroupByCategory_BucketsByCategoryAndSortsByID groups rules
// under their `category:` key and sorts each bucket by ID so the
// rendered page has a stable per-row order.
func TestGroupByCategory_BucketsByCategoryAndSortsByID(t *testing.T) {
	rs := []rules.RuleInfo{
		{ID: "MDS010", Category: "code"},
		{ID: "MDS003", Category: "heading"},
		{ID: "MDS001", Category: "line"},
		{ID: "MDS002", Category: "heading"},
	}
	got := groupByCategory(rs)
	require.Len(t, got["heading"], 2)
	assert.Equal(t, "MDS002", got["heading"][0].ID)
	assert.Equal(t, "MDS003", got["heading"][1].ID)
	assert.Len(t, got["code"], 1)
	assert.Len(t, got["line"], 1)
}

// TestOrderedCategories_PrefersCanonicalOrderThenAlphabetic
// returns canonical categories first, then any unknown category
// alphabetically at the tail.
func TestOrderedCategories_PrefersCanonicalOrderThenAlphabetic(t *testing.T) {
	grouped := map[string][]rules.RuleInfo{
		"experimental": nil,
		"prose":        nil,
		"heading":      nil,
		"zeta":         nil,
	}
	got := orderedCategories(grouped)
	// heading comes before prose in categoryOrder; experimental
	// and zeta are unknown and sort alphabetically at the end.
	assert.Equal(t,
		[]string{"heading", "prose", "experimental", "zeta"}, got)
}

// TestUnknownCategoryTitle_EmptyAndArbitrary covers the two
// branches: an empty category renders the loud "Uncategorized"
// label; any other unknown category title-cases its first byte.
func TestUnknownCategoryTitle_EmptyAndArbitrary(t *testing.T) {
	assert.Equal(t,
		"Uncategorized (category missing from rule README front matter)",
		unknownCategoryTitle(""))
	assert.Equal(t, "Experimental", unknownCategoryTitle("experimental"))
}

// TestCategoryIsMdsmithOnly_TrueOnlyWhenAllRulesHaveNoPeer
// returns true only when every rule in the slice has zero
// peer-linter mappings across all four tools.
func TestCategoryIsMdsmithOnly_TrueOnlyWhenAllRulesHaveNoPeer(t *testing.T) {
	assert.True(t, categoryIsMdsmithOnly([]rules.RuleInfo{
		{ID: "MDS019"}, {ID: "MDS021"},
	}))
	assert.False(t, categoryIsMdsmithOnly([]rules.RuleInfo{
		{ID: "MDS019"},
		{ID: "MDS003", Markdownlint: []rules.RuleMapping{
			{ID: "MD001"}}},
	}))
	assert.False(t, categoryIsMdsmithOnly([]rules.RuleInfo{
		{ID: "MDS027", Panache: []rules.RuleMapping{
			{ID: "undefined-anchor"}}},
	}))
}

// TestApplyCoverageMatrix_WritesWhenMissing verifies that a
// fresh run writes the generated file and returns changed=true.
func TestApplyCoverageMatrix_WritesWhenMissing(t *testing.T) {
	root := t.TempDir()
	changed, err := ApplyCoverageMatrix(root)
	require.NoError(t, err)
	assert.True(t, changed)
	path := filepath.Join(root, CoverageMatrixFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(data), "---\n"))
}

// TestApplyCoverageMatrix_IdempotentAfterFirstWrite verifies
// that a re-run on an already-generated file is a no-op.
func TestApplyCoverageMatrix_IdempotentAfterFirstWrite(t *testing.T) {
	root := t.TempDir()
	_, err := ApplyCoverageMatrix(root)
	require.NoError(t, err)
	changed, err := ApplyCoverageMatrix(root)
	require.NoError(t, err)
	assert.False(t, changed)
}

// TestRenderCoverageMatrix_UnknownCategoryFallback verifies
// that a rule whose `category:` value is missing from the
// canonical categoryTitle map still renders — the section
// title falls back to a title-cased form, and orderedCategories
// places the bucket at the end. Drives the "extras" branch in
// orderedCategories plus the title-fallback branch in
// RenderCoverageMatrix.
func TestRenderCoverageMatrix_UnknownCategoryFallback(t *testing.T) {
	rs := []rules.RuleInfo{
		{
			ID: "MDS900", Name: "experimental", Status: "ready",
			Description: "Experimental.",
			Category:    "experimental",
		},
	}
	out := RenderCoverageMatrix(rs)
	assert.Contains(t, out, "## Experimental")
}

// TestCheckCoverageMatrix_ReturnsEmptyWhenInSync verifies the
// happy path: when on-disk matches the generator, the check
// reports no drift (empty message, no error).
func TestCheckCoverageMatrix_ReturnsEmptyWhenInSync(t *testing.T) {
	root := t.TempDir()
	_, err := ApplyCoverageMatrix(root)
	require.NoError(t, err)
	msg, err := CheckCoverageMatrix(root)
	require.NoError(t, err)
	assert.Empty(t, msg)
}

// TestCheckCoverageMatrix_PropagatesReadError verifies that a
// non-NotExist read failure (here: a directory where a file is
// expected) surfaces as an error rather than a drift message.
func TestCheckCoverageMatrix_PropagatesReadError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(root, CoverageMatrixFile),
		0o755,
	))
	msg, err := CheckCoverageMatrix(root)
	require.Error(t, err)
	assert.Empty(t, msg)
}

// TestFormatCoverageDrift_FileLengthsDiffer drives the fallback
// branch in formatCoverageDrift: when every overlapping line
// matches but the files have different total lengths, the
// formatter reports the length mismatch rather than a per-line
// diff.
func TestFormatCoverageDrift_FileLengthsDiffer(t *testing.T) {
	// Two strings without a trailing newline; every overlapping
	// line matches and want is exactly one line longer.
	msg := formatCoverageDrift("a\nb", "a\nb\nc")
	assert.Contains(t, msg, "file has 2 lines, expected 3")
	assert.Contains(t, msg,
		"run `mdsmith-release sync-coverage-matrix` to regenerate")
}

// TestCheckCoverageMatrix_DetectsDrift verifies that a
// manually edited coverage file surfaces a drift message with
// the offending line number.
func TestCheckCoverageMatrix_DetectsDrift(t *testing.T) {
	root := t.TempDir()
	_, err := ApplyCoverageMatrix(root)
	require.NoError(t, err)
	path := filepath.Join(root, CoverageMatrixFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	tampered := strings.Replace(string(data),
		"# Peer-linter coverage matrix",
		"# Hand-edited title",
		1)
	require.NoError(t, os.WriteFile(path, []byte(tampered), 0o644))
	msg, err := CheckCoverageMatrix(root)
	require.NoError(t, err)
	assert.Contains(t, msg, "drift at line")
	assert.Contains(t, msg, "Hand-edited title")
}
