package tableformat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- GetPad coverage ---

func TestGetPad(t *testing.T) {
	r := &Rule{Pad: 1}
	assert.Equal(t, 1, r.GetPad())
}

func TestGetPad_Zero(t *testing.T) {
	r := &Rule{Pad: 0}
	assert.Equal(t, 0, r.GetPad())
}

func TestGetPad_Custom(t *testing.T) {
	r := &Rule{Pad: 3}
	assert.Equal(t, 3, r.GetPad())
}

// --- ApplySettings with float64 pad ---

func TestApplySettings_Float64Pad(t *testing.T) {
	r := &Rule{Pad: 1}
	err := r.ApplySettings(map[string]any{"pad": float64(2)})
	require.NoError(t, err)
	assert.Equal(t, 2, r.Pad)
}

func TestApplySettings_Int64Pad(t *testing.T) {
	r := &Rule{Pad: 1}
	err := r.ApplySettings(map[string]any{"pad": int64(3)})
	require.NoError(t, err)
	assert.Equal(t, 3, r.Pad)
}

// --- Fix with negative pad defaults ---

func TestFix_NegativePad_DefaultsTo1(t *testing.T) {
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	r := &Rule{Pad: -1}
	f := newTestFile(t, src)
	result := string(r.Fix(f))
	// Should use pad=1, producing padded output
	assert.Contains(t, result, "| a   | b   |")
}

// --- Check with negative pad ---

func TestCheck_NegativePad_DefaultsTo1(t *testing.T) {
	src := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	r := &Rule{Pad: -1}
	f := newTestFile(t, src)
	diags := r.Check(f)
	// Negative pad defaults to 1; the table needs reformatting to match pad=1.
	assert.NotEmpty(t, diags, "negative pad should default to 1 and produce diagnostics")
}
