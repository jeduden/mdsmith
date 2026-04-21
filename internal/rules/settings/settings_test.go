package settings_test

import (
	"math"
	"testing"

	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/stretchr/testify/assert"
)

func TestToInt(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int
		ok   bool
	}{
		{"int", 7, 7, true},
		{"int-zero", 0, 0, true},
		{"int-negative", -3, -3, true},
		{"int64", int64(42), 42, true},
		{"float64-whole", 5.0, 5, true},
		{"float64-truncates", 3.9, 3, true},
		{"float64-negative-truncates", -2.7, -2, true},
		{"float64-nan-rejected", math.NaN(), 0, false},
		{"float64-posinf-rejected", math.Inf(1), 0, false},
		{"float64-neginf-rejected", math.Inf(-1), 0, false},
		{"float64-overflow-rejected", 1e20, 0, false},
		{"float64-underflow-rejected", -1e20, 0, false},
		{"string-rejected", "5", 0, false},
		{"bool-rejected", true, 0, false},
		{"nil-rejected", nil, 0, false},
		{"slice-rejected", []int{1}, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := settings.ToInt(tc.in)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestToFloat(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want float64
		ok   bool
	}{
		{"float64", 1.5, 1.5, true},
		{"float64-zero", 0.0, 0.0, true},
		{"int", 4, 4.0, true},
		{"int64", int64(9), 9.0, true},
		{"float64-nan-rejected", math.NaN(), 0, false},
		{"float64-posinf-rejected", math.Inf(1), 0, false},
		{"float64-neginf-rejected", math.Inf(-1), 0, false},
		{"string-rejected", "1.5", 0, false},
		{"bool-rejected", false, 0, false},
		{"nil-rejected", nil, 0, false},
		{"slice-rejected", []any{1}, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := settings.ToFloat(tc.in)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestToStringSlice(t *testing.T) {
	t.Run("[]string returns copy", func(t *testing.T) {
		in := []string{"a", "b"}
		got, ok := settings.ToStringSlice(in)
		assert.True(t, ok)
		assert.Equal(t, []string{"a", "b"}, got)
		got[0] = "mutated"
		assert.Equal(t, "a", in[0], "caller mutation must not affect original")
	})

	t.Run("[]any of strings", func(t *testing.T) {
		got, ok := settings.ToStringSlice([]any{"x", "y"})
		assert.True(t, ok)
		assert.Equal(t, []string{"x", "y"}, got)
	})

	t.Run("empty []any", func(t *testing.T) {
		got, ok := settings.ToStringSlice([]any{})
		assert.True(t, ok)
		assert.Equal(t, []string{}, got)
	})

	t.Run("[]any with non-string rejected", func(t *testing.T) {
		got, ok := settings.ToStringSlice([]any{"a", 1})
		assert.False(t, ok)
		assert.Nil(t, got)
	})

	t.Run("string rejected", func(t *testing.T) {
		got, ok := settings.ToStringSlice("a,b")
		assert.False(t, ok)
		assert.Nil(t, got)
	})

	t.Run("nil rejected", func(t *testing.T) {
		got, ok := settings.ToStringSlice(nil)
		assert.False(t, ok)
		assert.Nil(t, got)
	})
}
