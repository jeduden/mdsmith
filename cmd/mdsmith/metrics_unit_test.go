package main

import (
	"bytes"
	"errors"
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
	err := writeRankOutput(&bytes.Buffer{}, "xml", nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown format")
}

func TestWriteRankOutput_TextFormat_NoError(t *testing.T) {
	assert.NoError(t, writeRankOutput(&bytes.Buffer{}, "text", nil, nil))
}

func TestWriteRankOutput_JSONFormat_NoError(t *testing.T) {
	assert.NoError(t, writeRankOutput(&bytes.Buffer{}, "json", nil, nil))
}

// --- writeMetricsListText ---

func TestWriteMetricsListText_PrintsHeaderAndRows(t *testing.T) {
	defs := []metricspkg.Definition{
		{ID: "m1", Name: "Metric One", Scope: metricspkg.ScopeFile, DefaultOrder: "desc", Description: "a test metric"},
	}
	var buf bytes.Buffer
	require.NoError(t, writeMetricsListText(&buf, defs))
	out := buf.String()
	assert.Contains(t, out, "ID")
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "m1")
	assert.Contains(t, out, "Metric One")
	assert.Contains(t, out, "a test metric")
}

func TestWriteMetricsListText_EmptyDefs_HeaderOnly(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeMetricsListText(&buf, nil))
	assert.Contains(t, buf.String(), "ID")
}

func TestWriteMetricsListText_WriteError(t *testing.T) {
	err := writeMetricsListText(&errWriter{err: errors.New("disk full")},
		[]metricspkg.Definition{{ID: "m1", Name: "M"}})
	require.Error(t, err)
}

// --- writeMetricsListJSON ---

func TestWriteMetricsListJSON_ValidJSONArray(t *testing.T) {
	defs := []metricspkg.Definition{
		{ID: "m1", Name: "Metric One", Description: "desc"},
	}
	var buf bytes.Buffer
	require.NoError(t, writeMetricsListJSON(&buf, defs))
	out := buf.String()
	assert.Contains(t, out, `"id"`)
	assert.Contains(t, out, `"m1"`)
	assert.Contains(t, out, `"name"`)
	assert.Contains(t, out, `"Metric One"`)
}

func TestWriteMetricsListJSON_EmptyDefs_EmptyArray(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeMetricsListJSON(&buf, nil))
	assert.Contains(t, buf.String(), "[]")
}

// --- writeMetricsRankText ---

func TestWriteMetricsRankText_PrintsHeaderAndRows(t *testing.T) {
	defs := []metricspkg.Definition{{ID: "bytes", Name: "bytes"}}
	rows := []metricspkg.Row{
		{Path: "a.md", Metrics: map[string]metricspkg.Value{"bytes": metricspkg.AvailableValue(100)}},
	}
	var buf bytes.Buffer
	require.NoError(t, writeMetricsRankText(&buf, rows, defs))
	out := buf.String()
	assert.Contains(t, out, "BYTES")
	assert.Contains(t, out, "PATH")
	assert.Contains(t, out, "a.md")
}

func TestWriteMetricsRankText_Empty_HeaderOnly(t *testing.T) {
	defs := []metricspkg.Definition{{ID: "bytes", Name: "bytes"}}
	var buf bytes.Buffer
	require.NoError(t, writeMetricsRankText(&buf, nil, defs))
	assert.Contains(t, buf.String(), "BYTES")
}

func TestWriteMetricsRankText_WriteError(t *testing.T) {
	defs := []metricspkg.Definition{{ID: "bytes", Name: "bytes"}}
	rows := []metricspkg.Row{
		{Path: "a.md", Metrics: map[string]metricspkg.Value{"bytes": metricspkg.AvailableValue(100)}},
	}
	err := writeMetricsRankText(&errWriter{err: errors.New("disk full")}, rows, defs)
	require.Error(t, err)
}

// --- writeMetricsRankJSON ---

func TestWriteMetricsRankJSON_ValidJSONArray(t *testing.T) {
	defs := []metricspkg.Definition{{ID: "bytes", Name: "bytes"}}
	rows := []metricspkg.Row{
		{Path: "a.md", Metrics: map[string]metricspkg.Value{"bytes": metricspkg.AvailableValue(100)}},
	}
	var buf bytes.Buffer
	require.NoError(t, writeMetricsRankJSON(&buf, rows, defs))
	out := buf.String()
	assert.Contains(t, out, `"path"`)
	assert.Contains(t, out, `"a.md"`)
}

func TestWriteMetricsRankJSON_Empty_EmptyArray(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeMetricsRankJSON(&buf, nil, nil))
	assert.Contains(t, buf.String(), "[]")
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
	var buf bytes.Buffer
	require.NoError(t, writeMetricsListText(&buf, defs))
	out := buf.String()
	assert.Contains(t, out, "m1")
	assert.Contains(t, out, "m2")
	assert.Contains(t, out, "Alpha")
	assert.Contains(t, out, "Beta")
}

// --- yaml format for list ---

func TestRunMetricsList_YAMLFormat_ExitsZero(t *testing.T) {
	captureStdout(func() {
		code := runMetricsList([]string{"-f", "yaml"})
		assert.Equal(t, 0, code)
	})
}

func TestWriteMetricsListYAML_ValidYAML(t *testing.T) {
	defs := []metricspkg.Definition{
		{ID: "m1", Name: "Metric One", Description: "desc"},
	}
	var buf bytes.Buffer
	require.NoError(t, writeMetricsListYAML(&buf, defs))
	out := buf.String()
	assert.Contains(t, out, "id:")
	assert.Contains(t, out, "m1")
	assert.Contains(t, out, "name:")
	assert.Contains(t, out, "Metric One")
}

// --- yaml format for rank ---

func TestWriteRankOutput_YAMLFormat_NoError(t *testing.T) {
	assert.NoError(t, writeRankOutput(&bytes.Buffer{}, "yaml", nil, nil))
}

func TestWriteMetricsRankYAML_ValidYAML(t *testing.T) {
	defs := []metricspkg.Definition{{ID: "bytes", Name: "bytes"}}
	rows := []metricspkg.Row{
		{Path: "a.md", Metrics: map[string]metricspkg.Value{"bytes": metricspkg.AvailableValue(100)}},
	}
	var buf bytes.Buffer
	require.NoError(t, writeMetricsRankYAML(&buf, rows, defs))
	out := buf.String()
	assert.Contains(t, out, "path:")
	assert.Contains(t, out, "a.md")
}

func TestWriteRankOutput_UnknownFormat_MentionsYAML(t *testing.T) {
	err := writeRankOutput(&bytes.Buffer{}, "xml", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "yaml")
}

// --- metrics get ---

func TestRunMetrics_GetSubcommand_NoArgs_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		code := runMetrics([]string{"get"})
		assert.Equal(t, 2, code)
	})
}

func TestRunMetrics_GetSubcommand_TooManyArgs_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		code := runMetrics([]string{"get", "a.md", "b.md"})
		assert.Equal(t, 2, code)
	})
}

func TestParseMetricsGetOptions_Defaults(t *testing.T) {
	opts, file, err := parseMetricsGetOptions([]string{"file.md"})
	require.NoError(t, err)
	assert.Equal(t, "text", opts.format)
	assert.Equal(t, "file.md", file)
}

func TestParseMetricsGetOptions_JSONFormat(t *testing.T) {
	opts, _, err := parseMetricsGetOptions([]string{"-f", "json", "file.md"})
	require.NoError(t, err)
	assert.Equal(t, "json", opts.format)
}

func TestParseMetricsGetOptions_YAMLFormat(t *testing.T) {
	opts, _, err := parseMetricsGetOptions([]string{"-f", "yaml", "file.md"})
	require.NoError(t, err)
	assert.Equal(t, "yaml", opts.format)
}

func TestParseMetricsGetOptions_NoFile_Error(t *testing.T) {
	_, _, err := parseMetricsGetOptions([]string{})
	assert.Error(t, err)
}

func TestParseMetricsGetOptions_TooManyFiles_Error(t *testing.T) {
	_, _, err := parseMetricsGetOptions([]string{"a.md", "b.md"})
	assert.Error(t, err)
}

func TestWriteGetOutput_TextFormat(t *testing.T) {
	defs := []metricspkg.Definition{
		{ID: "MET001", Name: "bytes", Kind: metricspkg.KindInteger},
	}
	row := metricspkg.Row{
		Path:    "test.md",
		Metrics: map[string]metricspkg.Value{"bytes": metricspkg.AvailableValue(100)},
	}
	var buf bytes.Buffer
	require.NoError(t, writeGetOutput(&buf, "text", row, defs))
	out := buf.String()
	assert.Contains(t, out, "bytes")
	assert.Contains(t, out, "100")
}

func TestWriteGetOutput_JSONFormat(t *testing.T) {
	defs := []metricspkg.Definition{
		{ID: "MET001", Name: "bytes", Kind: metricspkg.KindInteger},
	}
	row := metricspkg.Row{
		Path:    "test.md",
		Metrics: map[string]metricspkg.Value{"bytes": metricspkg.AvailableValue(100)},
	}
	var buf bytes.Buffer
	require.NoError(t, writeGetOutput(&buf, "json", row, defs))
	out := buf.String()
	assert.Contains(t, out, `"bytes"`)
	assert.Contains(t, out, "100")
}

func TestWriteGetOutput_YAMLFormat(t *testing.T) {
	defs := []metricspkg.Definition{
		{ID: "MET001", Name: "bytes", Kind: metricspkg.KindInteger},
	}
	row := metricspkg.Row{
		Path:    "test.md",
		Metrics: map[string]metricspkg.Value{"bytes": metricspkg.AvailableValue(100)},
	}
	var buf bytes.Buffer
	require.NoError(t, writeGetOutput(&buf, "yaml", row, defs))
	out := buf.String()
	assert.Contains(t, out, "bytes:")
	assert.Contains(t, out, "100")
}

func TestWriteGetOutput_UnknownFormat_Error(t *testing.T) {
	err := writeGetOutput(&bytes.Buffer{}, "xml", metricspkg.Row{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "text, json, yaml")
}

func TestWriteMetricsGetText_PrintsAlignedTable(t *testing.T) {
	defs := []metricspkg.Definition{
		{ID: "MET001", Name: "bytes", Kind: metricspkg.KindInteger},
		{ID: "MET003", Name: "words", Kind: metricspkg.KindInteger},
	}
	row := metricspkg.Row{
		Path: "test.md",
		Metrics: map[string]metricspkg.Value{
			"bytes": metricspkg.AvailableValue(200),
			"words": metricspkg.AvailableValue(30),
		},
	}
	var buf bytes.Buffer
	require.NoError(t, writeMetricsGetText(&buf, row, defs))
	out := buf.String()
	assert.Contains(t, out, "bytes")
	assert.Contains(t, out, "200")
	assert.Contains(t, out, "words")
	assert.Contains(t, out, "30")
}

func TestWriteMetricsGetText_WriteError(t *testing.T) {
	defs := []metricspkg.Definition{{ID: "bytes", Name: "bytes"}}
	row := metricspkg.Row{
		Path:    "test.md",
		Metrics: map[string]metricspkg.Value{"bytes": metricspkg.AvailableValue(100)},
	}
	err := writeMetricsGetText(&errWriter{err: errors.New("disk full")}, row, defs)
	require.Error(t, err)
}

func TestWriteMetricsGetJSON_ValidJSONObject(t *testing.T) {
	defs := []metricspkg.Definition{{ID: "bytes", Name: "bytes", Kind: metricspkg.KindInteger}}
	row := metricspkg.Row{
		Path:    "test.md",
		Metrics: map[string]metricspkg.Value{"bytes": metricspkg.AvailableValue(42)},
	}
	var buf bytes.Buffer
	require.NoError(t, writeMetricsGetJSON(&buf, row, defs))
	out := buf.String()
	assert.Contains(t, out, `"bytes"`)
	assert.Contains(t, out, "42")
	// Must be a JSON object, not an array.
	assert.True(t, strings.HasPrefix(strings.TrimSpace(out), "{"), "expected JSON object")
}

func TestWriteMetricsGetYAML_ValidYAMLObject(t *testing.T) {
	defs := []metricspkg.Definition{{ID: "bytes", Name: "bytes", Kind: metricspkg.KindInteger}}
	row := metricspkg.Row{
		Path:    "test.md",
		Metrics: map[string]metricspkg.Value{"bytes": metricspkg.AvailableValue(42)},
	}
	var buf bytes.Buffer
	require.NoError(t, writeMetricsGetYAML(&buf, row, defs))
	out := buf.String()
	assert.Contains(t, out, "bytes:")
	assert.Contains(t, out, "42")
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
	var buf bytes.Buffer
	require.NoError(t, writeMetricsRankText(&buf, rows, defs))
	out := buf.String()
	assert.True(t, strings.Contains(out, "a.md"))
	assert.True(t, strings.Contains(out, "b.md"))
}
