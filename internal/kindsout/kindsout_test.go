package kindsout

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingWriter returns the configured error from every Write so we
// can exercise the error-handling branches in WriteBodyText / etc.
type failingWriter struct {
	err   error
	after int // number of successful writes before erroring
	calls int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls > w.after {
		return 0, w.err
	}
	return len(p), nil
}

func TestRuleCfgValue_AllForms(t *testing.T) {
	assert.Equal(t, false, RuleCfgValue(config.RuleCfg{Enabled: false}))
	assert.Equal(t, true, RuleCfgValue(config.RuleCfg{Enabled: true}))
	v := RuleCfgValue(config.RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"max": 30},
	})
	m, ok := v.(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 30, m["max"])

	// Deep-merge can leave Enabled=false with non-nil Settings (a
	// bool-only layer toggling Enabled while inheriting Settings from
	// an earlier layer). The output value must report `false` so it
	// cannot contradict the `enabled` leaf.
	assert.Equal(t, false, RuleCfgValue(config.RuleCfg{
		Enabled:  false,
		Settings: map[string]any{"max": 30},
	}))
}

func TestRuleCfgJSON_Marshal(t *testing.T) {
	r := RuleCfgJSON{v: false}
	data, err := r.MarshalJSON()
	require.NoError(t, err)
	assert.Equal(t, "false", string(data))
}

func TestMakeBodyJSON_PreservesShape(t *testing.T) {
	body := config.KindBody{
		Rules: map[string]config.RuleCfg{
			"line-length":           {Enabled: true, Settings: map[string]any{"max": 30}},
			"paragraph-readability": {Enabled: false},
		},
		Categories: map[string]bool{"meta": true},
	}
	out := MakeBodyJSON("plan", body)
	assert.Equal(t, "plan", out.Name)
	require.Contains(t, out.Rules, "line-length")
	require.Contains(t, out.Rules, "paragraph-readability")

	enc, err := json.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(enc), `"line-length":{"max":30}`)
	assert.Contains(t, string(enc), `"paragraph-readability":false`)
}

func TestWriteBodyText_RendersYAMLBody(t *testing.T) {
	body := config.KindBody{
		Rules: map[string]config.RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 30}},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, WriteBodyText(&buf, "plan", body))
	out := buf.String()
	assert.Contains(t, out, "plan:")
	assert.Contains(t, out, "rules:")
	assert.Contains(t, out, "max: 30")
}

func TestWriteBodyText_EmptyBodyRendersPlaceholder(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteBodyText(&buf, "ghost", config.KindBody{}))
	out := buf.String()
	assert.Contains(t, out, "ghost:")
	assert.Contains(t, out, "(empty)")
}

func TestWriteBodyText_HeaderWriteError(t *testing.T) {
	w := &failingWriter{err: errors.New("disk full"), after: 0}
	err := WriteBodyText(w, "plan", config.KindBody{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}

func TestWriteBodyText_BodyWriteError(t *testing.T) {
	w := &failingWriter{err: errors.New("nope"), after: 1}
	body := config.KindBody{
		Rules: map[string]config.RuleCfg{"x": {Enabled: true}},
	}
	err := WriteBodyText(w, "plan", body)
	require.Error(t, err)
}

func TestWriteBodyText_EmptyPlaceholderWriteError(t *testing.T) {
	w := &failingWriter{err: errors.New("nope"), after: 1}
	err := WriteBodyText(w, "ghost", config.KindBody{})
	require.Error(t, err)
}

// makeFileResolution builds a minimal FileResolution with one rule and
// two layers so writers exercise both the kinds and rules branches.
func makeFileResolution(t *testing.T) *config.FileResolution {
	t.Helper()
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Kinds: map[string]config.KindBody{
			"short": {Rules: map[string]config.RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 30}},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Files: []string{"x.md"}, Kinds: []string{"short"}},
		},
	}
	return config.ResolveFile(cfg, "x.md", nil)
}

func TestWriteFileResolutionText_Full(t *testing.T) {
	res := makeFileResolution(t)
	var buf bytes.Buffer
	require.NoError(t, WriteFileResolutionText(&buf, res))
	out := buf.String()
	assert.Contains(t, out, "file: x.md")
	assert.Contains(t, out, "short (from kind-assignment[0])")
	assert.Contains(t, out, "line-length")
	assert.Contains(t, out, "settings.max = 30")
	assert.Contains(t, out, "(from kinds.short)")
}

func TestWriteFileResolutionText_NoKinds(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{"line-length": {Enabled: true}},
	}
	res := config.ResolveFile(cfg, "x.md", nil)
	var buf bytes.Buffer
	require.NoError(t, WriteFileResolutionText(&buf, res))
	assert.Contains(t, buf.String(), "(none)")
}

func TestWriteFileResolutionText_WriteErrorPropagates(t *testing.T) {
	res := makeFileResolution(t)
	for after := 0; after < 6; after++ {
		w := &failingWriter{err: errors.New("io"), after: after}
		err := WriteFileResolutionText(w, res)
		assert.Error(t, err, "expected error for write #%d", after)
	}
}

func TestWriteRuleResolutionText_FullAndNoOpLayers(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length":           {Enabled: true, Settings: map[string]any{"max": 80}},
			"paragraph-readability": {Enabled: true},
		},
		Kinds: map[string]config.KindBody{
			// short does not touch line-length, so it appears as a no-op layer.
			"short": {Rules: map[string]config.RuleCfg{
				"paragraph-readability": {Enabled: false},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Files: []string{"x.md"}, Kinds: []string{"short"}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil)
	rr := res.Rules["line-length"]
	var buf bytes.Buffer
	require.NoError(t, WriteRuleResolutionText(&buf, "x.md", rr))
	out := buf.String()
	assert.Contains(t, out, "rule: line-length")
	assert.Contains(t, out, "default")
	assert.Contains(t, out, "no-op")
	assert.Contains(t, out, "kinds.short")
	assert.Contains(t, out, "winning source: default")
}

func TestWriteRuleResolutionText_WriteErrorPropagates(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"r": {Enabled: true, Settings: map[string]any{"v": 1}},
		},
		Kinds: map[string]config.KindBody{
			"k": {Rules: map[string]config.RuleCfg{
				"r": {Enabled: true, Settings: map[string]any{"v": 2}},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Files: []string{"x.md"}, Kinds: []string{"k"}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil)
	rr := res.Rules["r"]
	for after := 0; after < 8; after++ {
		w := &failingWriter{err: errors.New("io"), after: after}
		err := WriteRuleResolutionText(w, "x.md", rr)
		assert.Error(t, err, "expected error for write #%d", after)
	}
}

func TestWriteJSON_RendersIndentedJSON(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteJSON(&buf, map[string]int{"a": 1}))
	assert.Contains(t, buf.String(), "\"a\": 1")
	assert.Equal(t, "\n", buf.String()[len(buf.String())-1:])
}

func TestWriteJSON_EncodingError(t *testing.T) {
	// channels are not encodable
	err := WriteJSON(&bytes.Buffer{}, make(chan int))
	require.Error(t, err)
}

func TestWriteJSON_WriteError(t *testing.T) {
	w := &failingWriter{err: errors.New("io"), after: 0}
	err := WriteJSON(w, map[string]int{"a": 1})
	require.Error(t, err)
}

func TestFormatValue_Scalars(t *testing.T) {
	assert.Equal(t, "30", FormatValue(30))
	assert.Equal(t, "true", FormatValue(true))
	assert.Equal(t, "\"hi\"", FormatValue("hi"))
	assert.Equal(t, "null", FormatValue(nil))
}

func TestFormatValue_FallbackForUnmarshalable(t *testing.T) {
	// channel is not JSON-encodable; FormatValue falls back to %v,
	// which prints the channel's pointer address.
	out := FormatValue(make(chan int))
	assert.NotEmpty(t, out)
	assert.True(t, strings.HasPrefix(out, "0x"),
		"expected pointer-like fallback, got %q", out)
}

func TestFileResolutionJSON_Shape(t *testing.T) {
	res := makeFileResolution(t)
	out := FileResolution(res)
	assert.Equal(t, "x.md", out.File)
	require.Len(t, out.Kinds, 1)
	assert.Equal(t, "short", out.Kinds[0].Name)
	assert.Equal(t, "kind-assignment[0]", out.Kinds[0].Source)
	rr, ok := out.Rules["line-length"]
	require.True(t, ok)
	var sawMax bool
	for _, l := range rr.Leaves {
		if l.Path == "settings.max" {
			sawMax = true
			assert.Equal(t, "kinds.short", l.Source)
		}
	}
	assert.True(t, sawMax)
}

func TestRuleResolutionJSON_IncludesNoOpLayers(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length":           {Enabled: true, Settings: map[string]any{"max": 80}},
			"paragraph-readability": {Enabled: true},
		},
		Kinds: map[string]config.KindBody{
			"short": {Rules: map[string]config.RuleCfg{
				"paragraph-readability": {Enabled: false},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Files: []string{"x.md"}, Kinds: []string{"short"}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil)
	rr := res.Rules["line-length"]
	out := RuleResolution("x.md", rr)
	require.Len(t, out.Layers, 2)
	assert.Equal(t, "default", out.Layers[0].Source)
	assert.True(t, out.Layers[0].Set)
	assert.Equal(t, "kinds.short", out.Layers[1].Source)
	assert.False(t, out.Layers[1].Set, "no-op layer for line-length")
}

func TestLeavesJSON_PreservesChain(t *testing.T) {
	res := makeFileResolution(t)
	rr := res.Rules["line-length"]
	out := RuleResolution("x.md", rr)
	for _, l := range out.Leaves {
		if l.Path == "settings.max" {
			require.Len(t, l.Chain, 2)
			assert.Equal(t, "default", l.Chain[0].Source)
			assert.Equal(t, "kinds.short", l.Chain[1].Source)
			return
		}
	}
	t.Fatalf("settings.max leaf missing from %v", out.Leaves)
}

// erroringYAMLMarshaler implements yaml.Marshaler with a method that
// always returns an error so we can drive yaml.Marshal down its
// error-return path.
type erroringYAMLMarshaler struct{}

func (erroringYAMLMarshaler) MarshalYAML() (any, error) {
	return nil, errors.New("synthetic marshal error")
}

// TestWriteBodyText_YAMLMarshalError covers the yaml.Marshal failure
// branch: a setting whose value implements yaml.Marshaler and returns
// an error makes yaml.Marshal surface that error rather than panicking.
func TestWriteBodyText_YAMLMarshalError(t *testing.T) {
	var buf bytes.Buffer
	body := config.KindBody{
		Rules: map[string]config.RuleCfg{
			"x": {
				Enabled:  true,
				Settings: map[string]any{"bad": erroringYAMLMarshaler{}},
			},
		},
	}
	err := WriteBodyText(&buf, "k", body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "synthetic marshal error")
}

// TestWriteFileResolutionText_NoneWriteError covers the (none) Fprintln
// error branch: an empty kinds list combined with a writer that fails
// after the first two writes.
func TestWriteFileResolutionText_NoneWriteError(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{"r": {Enabled: true}},
	}
	res := config.ResolveFile(cfg, "x.md", nil)
	w := &failingWriter{err: errors.New("io"), after: 2}
	err := WriteFileResolutionText(w, res)
	require.Error(t, err)
}

// TestSanitizeControl_StripsCtrls covers the sanitizeControl helper.
func TestSanitizeControl_StripsCtrls(t *testing.T) {
	assert.Equal(t, "ab", sanitizeControl("a\nb"))     // C0: LF
	assert.Equal(t, "ab", sanitizeControl("a\x07b"))   // C0: BEL
	assert.Equal(t, "ab", sanitizeControl("a\x1bb"))   // C0: ESC
	assert.Equal(t, "ab", sanitizeControl("a\u009fb")) // C1: U+009F
	assert.Equal(t, "hello", sanitizeControl("hello"))
}

// TestWriteBodyText_SanitizesKindName ensures control chars in the kind
// name are stripped from the header line.
func TestWriteBodyText_SanitizesKindName(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteBodyText(&buf, "evil\nkind", config.KindBody{}))
	assert.NotContains(t, buf.String(), "\n\n") // no extra blank line from injected newline
	assert.Contains(t, buf.String(), "evilkind:")
}

// TestWriteFileResolutionText_SanitizesKindName ensures control chars
// in kind names (from user YAML) are stripped from the text output.
func TestWriteFileResolutionText_SanitizesKindName(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {Enabled: true},
		},
		Kinds: map[string]config.KindBody{
			"evil\x1bkind": {Rules: map[string]config.RuleCfg{
				"line-length": {Enabled: true},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Files: []string{"x.md"}, Kinds: []string{"evil\x1bkind"}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil)
	var buf bytes.Buffer
	require.NoError(t, WriteFileResolutionText(&buf, res))
	assert.NotContains(t, buf.String(), "\x1b")
	assert.Contains(t, buf.String(), "evilkind")
}

// TestWriteRuleResolutionText_SanitizesFields ensures control chars in
// file/rule/source fields are stripped from the text output.
func TestWriteRuleResolutionText_SanitizesFields(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line\x07length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
	}
	res := config.ResolveFile(cfg, "evil\x07file.md", nil)
	rr := res.Rules["line\x07length"]
	var buf bytes.Buffer
	require.NoError(t, WriteRuleResolutionText(&buf, "evil\x07file.md", rr))
	out := buf.String()
	assert.NotContains(t, out, "\x07")
	assert.Contains(t, out, "linelength")  // BEL stripped from rule name
	assert.Contains(t, out, "evilfile.md") // BEL stripped from file name
}

// Ensure WriteBodyText output is sorted deterministically.
func TestWriteBodyText_DeterministicOutput(t *testing.T) {
	body := config.KindBody{
		Rules: map[string]config.RuleCfg{
			"a": {Enabled: false},
			"b": {Enabled: false},
			"c": {Enabled: false},
		},
	}
	var first, second bytes.Buffer
	require.NoError(t, WriteBodyText(&first, "plan", body))
	require.NoError(t, WriteBodyText(&second, "plan", body))
	assert.Equal(t, first.String(), second.String())
	// All three names appear.
	for _, name := range []string{"a", "b", "c"} {
		assert.True(t, strings.Contains(first.String(), name))
	}
}

func TestWriteFileResolutionText_ShowsConventionLayer(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 120}},
		},
		Convention: "portable",
		ConventionPreset: map[string]config.RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil)
	var buf bytes.Buffer
	require.NoError(t, WriteFileResolutionText(&buf, res))
	out := buf.String()
	// User's max=120 wins; provenance shows default as winning source
	// because cfg.Rules sits above the convention layer.
	assert.Contains(t, out, "settings.max = 120")
	assert.Contains(t, out, "(from default)")
}

func TestWriteRuleResolutionText_ShowsConventionLayer(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 120}},
		},
		Convention: "portable",
		ConventionPreset: map[string]config.RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
	}
	res := config.ResolveFile(cfg, "x.md", nil)
	rr := res.Rules["line-length"]
	var buf bytes.Buffer
	require.NoError(t, WriteRuleResolutionText(&buf, "x.md", rr))
	out := buf.String()
	assert.Contains(t, out, "convention.portable",
		"convention layer must appear in chain")
	assert.Contains(t, out, "winning source: default",
		"user value wins over convention preset")
}
