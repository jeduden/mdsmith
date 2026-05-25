package release

import (
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
