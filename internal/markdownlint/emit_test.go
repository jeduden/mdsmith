package markdownlint

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/yamlutil"
)

func TestEmitConfig(t *testing.T) {
	conv := &Conversion{
		Rules: map[string]config.RuleCfg{
			"line-length":        {Enabled: true, Settings: map[string]any{"max": 100}},
			"first-line-heading": {Enabled: false},
			"single-h1":          {Enabled: true},
		},
		Notes: []string{
			`MD024 option "siblings_only": no mdsmith equivalent`,
		},
	}

	out, err := EmitConfig(conv, ".markdownlint.yaml")
	require.NoError(t, err)

	want := `# Converted from .markdownlint.yaml by mdsmith init --from-markdownlint.
# Rules not listed here keep their mdsmith defaults.
#
# Not converted:
# - MD024 option "siblings_only": no mdsmith equivalent

front-matter: true
rules:
    first-line-heading: false
    line-length:
        max: 100
    single-h1: true
`
	assert.Equal(t, want, string(out))
}

func TestEmitConfig_NoNotes(t *testing.T) {
	conv := &Conversion{
		Rules: map[string]config.RuleCfg{"no-hard-tabs": {Enabled: false}},
	}

	out, err := EmitConfig(conv, ".markdownlint.json")
	require.NoError(t, err)

	s := string(out)
	assert.NotContains(t, s, "Not converted")
	assert.Contains(t, s, "no-hard-tabs: false\n")
}

// TestEmitConfig_RoundTrips loads the emitted YAML back through the
// config parser: the converted file must be a valid .mdsmith.yml.
func TestEmitConfig_RoundTrips(t *testing.T) {
	conv := mustConvert(t, `
MD013: {line_length: 100}
MD033: {allowed_elements: [kbd]}
MD041: false
`)
	out, err := EmitConfig(conv, ".markdownlint.yaml")
	require.NoError(t, err)

	var cfg config.Config
	require.NoError(t, yamlutil.UnmarshalSafe(out, &cfg))
	assert.Equal(t, map[string]any{"max": 100}, cfg.Rules["line-length"].Settings)
	assert.False(t, cfg.Rules["first-line-heading"].Enabled)
	assert.Equal(t, map[string]any{"allow": []any{"kbd"}}, cfg.Rules["no-inline-html"].Settings)
}

// failingMarshaler forces yaml.Marshal down its error return, the only
// way to drive EmitConfig's marshal-failure branch.
type failingMarshaler struct{}

func (failingMarshaler) MarshalYAML() (any, error) {
	return nil, errors.New("boom")
}

func TestEmitConfig_MarshalError(t *testing.T) {
	conv := &Conversion{Rules: map[string]config.RuleCfg{
		"line-length": {Enabled: true, Settings: map[string]any{"bad": failingMarshaler{}}},
	}}

	_, err := EmitConfig(conv, "x.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshalling converted config")
}

func TestWrapComment(t *testing.T) {
	assert.Equal(t, []string{"# - short"}, wrapComment("short", 72))

	long := "one two three four five six seven eight nine ten"
	got := wrapComment(long, 20)
	assert.Equal(t, []string{
		"# - one two three",
		"#   four five six",
		"#   seven eight nine",
		"#   ten",
	}, got)

	// A single overlong word is not split.
	assert.Equal(t, []string{"# - aaaaaaaaaaaaaaaaaaaaaaaa"},
		wrapComment("aaaaaaaaaaaaaaaaaaaaaaaa", 10))
}

// TestWrapComment_AllocsPerCall verifies that wrapComment uses strings.Builder
// rather than += in a loop, keeping allocations low regardless of word count.
// Per the high-performance Go guidelines: "strings.Builder over +".
// Before the fix, each iteration allocates a new backing array for the string.
func TestWrapComment_AllocsPerCall(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc count skipped in short mode")
	}
	// 20 words all fitting within width=200 — one long line.
	// With +=, each of the 20 words triggers a string copy (≥20 allocs).
	// With strings.Builder, the entire call uses ≤3 allocs.
	text := "one two three four five six seven eight nine ten " +
		"eleven twelve thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty"
	allocs := testing.AllocsPerRun(100, func() {
		_ = wrapComment(text, 200)
	})
	assert.LessOrEqualf(t, allocs, float64(5),
		"wrapComment allocates %.0f/call for 20-word input; want ≤5 "+
			"(guideline: use strings.Builder instead of += in a loop)", allocs)
}
