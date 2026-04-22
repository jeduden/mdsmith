package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metricspkg "github.com/jeduden/mdsmith/internal/metrics"
)

// --- containsMetric ---

func TestContainsMetric_FoundFirst(t *testing.T) {
	defs := []metricspkg.Definition{{ID: "bytes"}, {ID: "lines"}}
	assert.True(t, containsMetric(defs, "bytes"))
}

func TestContainsMetric_FoundLast(t *testing.T) {
	defs := []metricspkg.Definition{{ID: "bytes"}, {ID: "lines"}}
	assert.True(t, containsMetric(defs, "lines"))
}

func TestContainsMetric_NotFound(t *testing.T) {
	defs := []metricspkg.Definition{{ID: "bytes"}}
	assert.False(t, containsMetric(defs, "missing"))
}

func TestContainsMetric_EmptySlice(t *testing.T) {
	assert.False(t, containsMetric(nil, "bytes"))
}

// --- parseMetricsRankOptions ---

func TestParseMetricsRankOptions_Defaults(t *testing.T) {
	opts, files, err := parseMetricsRankOptions(nil)
	require.NoError(t, err)
	assert.Equal(t, "text", opts.format)
	assert.Equal(t, 0, opts.top)
	assert.Equal(t, []string{"."}, files)
}

func TestParseMetricsRankOptions_ExplicitFiles(t *testing.T) {
	_, files, err := parseMetricsRankOptions([]string{"a.md", "b.md"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a.md", "b.md"}, files)
}

func TestParseMetricsRankOptions_TopFlag(t *testing.T) {
	opts, _, err := parseMetricsRankOptions([]string{"--top", "5"})
	require.NoError(t, err)
	assert.Equal(t, 5, opts.top)
}

func TestParseMetricsRankOptions_NegativeTop_Error(t *testing.T) {
	_, _, err := parseMetricsRankOptions([]string{"--top", "-1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--top must be >= 0")
}

func TestParseMetricsRankOptions_JSONFormat(t *testing.T) {
	opts, _, err := parseMetricsRankOptions([]string{"-f", "json"})
	require.NoError(t, err)
	assert.Equal(t, "json", opts.format)
}

func TestParseMetricsRankOptions_MetricsFlag(t *testing.T) {
	opts, _, err := parseMetricsRankOptions([]string{"--metrics", "bytes,lines"})
	require.NoError(t, err)
	assert.Equal(t, "bytes,lines", opts.metricsRaw)
}

func TestParseMetricsRankOptions_ByFlag(t *testing.T) {
	opts, _, err := parseMetricsRankOptions([]string{"--by", "bytes"})
	require.NoError(t, err)
	assert.Equal(t, "bytes", opts.byRaw)
}

func TestParseMetricsRankOptions_OrderFlag(t *testing.T) {
	opts, _, err := parseMetricsRankOptions([]string{"--order", "asc"})
	require.NoError(t, err)
	assert.Equal(t, "asc", opts.orderRaw)
}

func TestParseMetricsRankOptions_MaxInputSize(t *testing.T) {
	opts, _, err := parseMetricsRankOptions([]string{"--max-input-size", "1MB"})
	require.NoError(t, err)
	assert.Equal(t, "1MB", opts.maxInputSize)
}

func TestParseMetricsRankOptions_NoGitignore(t *testing.T) {
	opts, _, err := parseMetricsRankOptions([]string{"--no-gitignore"})
	require.NoError(t, err)
	assert.True(t, opts.noGitignore)
}

// --- resolveRankSelection ---

func TestResolveRankSelection_Defaults_ReturnsDefsAndByDef(t *testing.T) {
	opts := metricsRankOptions{}
	defs, byDef, _, err := resolveRankSelection(opts)
	require.NoError(t, err)
	assert.NotEmpty(t, defs)
	assert.NotEmpty(t, byDef.ID)
}

func TestResolveRankSelection_ExplicitByMetric(t *testing.T) {
	opts := metricsRankOptions{byRaw: "bytes"}
	_, byDef, _, err := resolveRankSelection(opts)
	require.NoError(t, err)
	assert.Equal(t, "bytes", byDef.Name)
}

func TestResolveRankSelection_UnknownMetric_Error(t *testing.T) {
	opts := metricsRankOptions{metricsRaw: "no-such-metric"}
	_, _, _, err := resolveRankSelection(opts)
	assert.Error(t, err)
}

func TestResolveRankSelection_ByNotInExplicitMetrics_Error(t *testing.T) {
	opts := metricsRankOptions{metricsRaw: "bytes", byRaw: "lines"}
	_, _, _, err := resolveRankSelection(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--by metric")
}

func TestResolveRankSelection_ExplicitAscOrder(t *testing.T) {
	opts := metricsRankOptions{orderRaw: "asc"}
	_, _, order, err := resolveRankSelection(opts)
	require.NoError(t, err)
	assert.Equal(t, metricspkg.Order("asc"), order)
}

func TestResolveRankSelection_ExplicitDescOrder(t *testing.T) {
	opts := metricsRankOptions{orderRaw: "desc"}
	_, _, order, err := resolveRankSelection(opts)
	require.NoError(t, err)
	assert.Equal(t, metricspkg.Order("desc"), order)
}

func TestResolveRankSelection_InvalidOrder_Error(t *testing.T) {
	opts := metricsRankOptions{orderRaw: "sideways"}
	_, _, _, err := resolveRankSelection(opts)
	assert.Error(t, err)
}

func TestResolveRankSelection_ByNotDefaultButInMetrics_OK(t *testing.T) {
	// When --by is included in --metrics, no error even if not default.
	opts := metricsRankOptions{metricsRaw: "bytes,lines", byRaw: "lines"}
	_, byDef, _, err := resolveRankSelection(opts)
	require.NoError(t, err)
	assert.Equal(t, "lines", byDef.Name)
}

func TestResolveRankSelection_ByNotInDefaultsGetsAppended(t *testing.T) {
	// When no explicit --metrics and --by is non-default,
	// the by metric should be appended to the defaults.
	opts := metricsRankOptions{byRaw: "lines"}
	defs, byDef, _, err := resolveRankSelection(opts)
	require.NoError(t, err)
	assert.Equal(t, "lines", byDef.Name)
	assert.True(t, containsMetric(defs, byDef.ID))
}

// --- writeRankOutput ---

func TestWriteRankOutput_UnknownFormat_Error(t *testing.T) {
	err := writeRankOutput("xml", nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown format")
}

func TestWriteRankOutput_TextFormat_NoError(t *testing.T) {
	captureStdout(func() {
		err := writeRankOutput("text", nil, nil)
		assert.NoError(t, err)
	})
}

func TestWriteRankOutput_JSONFormat_NoError(t *testing.T) {
	captureStdout(func() {
		err := writeRankOutput("json", nil, nil)
		assert.NoError(t, err)
	})
}

// --- writeMetricsListText ---

func TestWriteMetricsListText_PrintsHeaderAndRows(t *testing.T) {
	defs := []metricspkg.Definition{
		{ID: "m1", Name: "Metric One", Scope: metricspkg.ScopeFile, DefaultOrder: "desc", Description: "a test metric"},
	}
	out := captureStdout(func() {
		err := writeMetricsListText(defs)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "ID")
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "m1")
	assert.Contains(t, out, "Metric One")
	assert.Contains(t, out, "a test metric")
}

func TestWriteMetricsListText_EmptyDefs_HeaderOnly(t *testing.T) {
	out := captureStdout(func() {
		err := writeMetricsListText(nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "ID")
}

// --- writeMetricsListJSON ---

func TestWriteMetricsListJSON_ValidJSONArray(t *testing.T) {
	defs := []metricspkg.Definition{
		{ID: "m1", Name: "Metric One", Description: "desc"},
	}
	out := captureStdout(func() {
		err := writeMetricsListJSON(defs)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, `"id"`)
	assert.Contains(t, out, `"m1"`)
	assert.Contains(t, out, `"name"`)
	assert.Contains(t, out, `"Metric One"`)
}

func TestWriteMetricsListJSON_EmptyDefs_EmptyArray(t *testing.T) {
	out := captureStdout(func() {
		err := writeMetricsListJSON(nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "[]")
}

// --- writeMetricsRankText ---

func TestWriteMetricsRankText_PrintsHeaderAndRows(t *testing.T) {
	defs := []metricspkg.Definition{{ID: "bytes", Name: "bytes"}}
	rows := []metricspkg.Row{
		{Path: "a.md", Metrics: map[string]metricspkg.Value{"bytes": metricspkg.AvailableValue(100)}},
	}
	out := captureStdout(func() {
		err := writeMetricsRankText(rows, defs)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "BYTES")
	assert.Contains(t, out, "PATH")
	assert.Contains(t, out, "a.md")
}

func TestWriteMetricsRankText_Empty_HeaderOnly(t *testing.T) {
	defs := []metricspkg.Definition{{ID: "bytes", Name: "bytes"}}
	out := captureStdout(func() {
		err := writeMetricsRankText(nil, defs)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "BYTES")
}

// --- writeMetricsRankJSON ---

func TestWriteMetricsRankJSON_ValidJSONArray(t *testing.T) {
	defs := []metricspkg.Definition{{ID: "bytes", Name: "bytes"}}
	rows := []metricspkg.Row{
		{Path: "a.md", Metrics: map[string]metricspkg.Value{"bytes": metricspkg.AvailableValue(100)}},
	}
	out := captureStdout(func() {
		err := writeMetricsRankJSON(rows, defs)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, `"path"`)
	assert.Contains(t, out, `"a.md"`)
}

func TestWriteMetricsRankJSON_Empty_EmptyArray(t *testing.T) {
	out := captureStdout(func() {
		err := writeMetricsRankJSON(nil, nil)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "[]")
}

// --- runMetrics dispatch ---

func TestRunMetrics_NoArgs_PrintsUsageExitsZero(t *testing.T) {
	got := captureStderr(func() {
		code := runMetrics(nil)
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, got, "metrics")
}

func TestRunMetrics_UnknownSubcommand_ExitsTwo(t *testing.T) {
	got := captureStderr(func() {
		code := runMetrics([]string{"unknown"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "unknown command")
}

func TestRunMetrics_ListSubcommand_ExitsZero(t *testing.T) {
	captureStdout(func() {
		code := runMetrics([]string{"list"})
		assert.Equal(t, 0, code)
	})
}

// --- runHelpMetrics ---

func TestRunHelpMetrics_NoArgs_ListsMetrics(t *testing.T) {
	out := captureStdout(func() {
		code := runHelpMetrics(nil)
		assert.Equal(t, 0, code)
	})
	assert.NotEmpty(t, out)
}

func TestRunHelpMetrics_KnownMetric_ShowsDoc(t *testing.T) {
	out := captureStdout(func() {
		captureStderr(func() {
			code := runHelpMetrics([]string{"bytes"})
			assert.Equal(t, 0, code)
		})
	})
	assert.NotEmpty(t, out)
}

// --- listAllMetrics / showMetric ---

func TestListAllMetrics_PrintsRows(t *testing.T) {
	out := captureStdout(func() {
		code := listAllMetrics()
		assert.Equal(t, 0, code)
	})
	assert.NotEmpty(t, out)
}

func TestShowMetric_KnownMetric_PrintsContent(t *testing.T) {
	out := captureStdout(func() {
		captureStderr(func() {
			code := showMetric("bytes")
			assert.Equal(t, 0, code)
		})
	})
	assert.NotEmpty(t, out)
}

func TestShowMetric_UnknownMetric_ExitsTwo(t *testing.T) {
	captureStdout(func() {
		captureStderr(func() {
			code := showMetric("no-such-metric")
			assert.Equal(t, 2, code)
		})
	})
}

// --- runMetricsList ---

func TestRunMetricsList_DefaultText_ExitsZero(t *testing.T) {
	captureStdout(func() {
		code := runMetricsList(nil)
		assert.Equal(t, 0, code)
	})
}

func TestRunMetricsList_JSONFormat_ExitsZero(t *testing.T) {
	captureStdout(func() {
		code := runMetricsList([]string{"-f", "json"})
		assert.Equal(t, 0, code)
	})
}

func TestRunMetricsList_UnknownFormat_ExitsTwo(t *testing.T) {
	captureStdout(func() {
		captureStderr(func() {
			code := runMetricsList([]string{"-f", "xml"})
			assert.Equal(t, 2, code)
		})
	})
}

func TestRunMetricsList_FileArgs_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		code := runMetricsList([]string{"file.md"})
		assert.Equal(t, 2, code)
	})
}

func TestRunMetricsList_InvalidScope_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		code := runMetricsList([]string{"--scope", "bogus"})
		assert.Equal(t, 2, code)
	})
}

// --- writeMetricsListText / JSON with multiple defs ---

func TestWriteMetricsListText_MultipleRows(t *testing.T) {
	defs := []metricspkg.Definition{
		{ID: "m1", Name: "Alpha", Scope: metricspkg.ScopeFile, DefaultOrder: "asc"},
		{ID: "m2", Name: "Beta", Scope: metricspkg.ScopeFile, DefaultOrder: "desc"},
	}
	out := captureStdout(func() {
		err := writeMetricsListText(defs)
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "m1")
	assert.Contains(t, out, "m2")
	assert.Contains(t, out, "Alpha")
	assert.Contains(t, out, "Beta")
}

func TestWriteMetricsRankText_MultipleMetrics(t *testing.T) {
	defs := []metricspkg.Definition{
		{ID: "bytes", Name: "bytes"},
		{ID: "lines", Name: "lines"},
	}
	rows := []metricspkg.Row{
		{Path: "a.md", Metrics: map[string]metricspkg.Value{
			"bytes": metricspkg.AvailableValue(100),
			"lines": metricspkg.AvailableValue(5),
		}},
		{Path: "b.md", Metrics: map[string]metricspkg.Value{
			"bytes": metricspkg.AvailableValue(200),
			"lines": metricspkg.AvailableValue(10),
		}},
	}
	out := captureStdout(func() {
		err := writeMetricsRankText(rows, defs)
		assert.NoError(t, err)
	})
	assert.True(t, strings.Contains(out, "a.md"))
	assert.True(t, strings.Contains(out, "b.md"))
}
