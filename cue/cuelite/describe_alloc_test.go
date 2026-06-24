package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDescribe_IntBool_Values verifies that describe() produces the correct
// CUE literal for concrete integer and boolean values.
func TestDescribe_IntBool_Values(t *testing.T) {
	cases := []struct {
		name string
		v    *engineValue
		want string
	}{
		{"kInt zero", &engineValue{kind: kInt, i: 0}, "0"},
		{"kInt positive", &engineValue{kind: kInt, i: 42}, "42"},
		{"kInt negative", &engineValue{kind: kInt, i: -7}, "-7"},
		{"kBool true", &engineValue{kind: kBool, b: true}, "true"},
		{"kBool false", &engineValue{kind: kBool, b: false}, "false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.v.describe())
		})
	}
}

// TestDescribe_Bool_ZeroAlloc pins that describe() on a kBool value must not
// allocate. strconv.FormatBool returns a string literal ("true"/"false"),
// which is zero allocations; fmt.Sprintf("%t", v.b) allocates a new string.
//
// This test is RED before the fix (fmt.Sprintf path allocates ≥1) and GREEN
// after (strconv.FormatBool path allocates 0).
func TestDescribe_Bool_ZeroAlloc(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	v := &engineValue{kind: kBool, b: true}
	// Warm run to prime any internal state.
	_ = v.describe()

	allocs := testing.AllocsPerRun(100, func() {
		_ = v.describe()
	})
	require.LessOrEqualf(t, allocs, float64(0),
		"describe() kBool allocated %.0f/call; want 0 "+
			"(strconv.FormatBool returns a string literal, not fmt.Sprintf)", allocs)
}

// TestDescribe_Int_OneAlloc pins that describe() on a kInt value allocates
// exactly one string. Both fmt.Sprintf and strconv.FormatInt allocate one
// string for the result, but strconv avoids the reflection overhead.
func TestDescribe_Int_OneAlloc(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	v := &engineValue{kind: kInt, i: 12345}
	_ = v.describe()

	allocs := testing.AllocsPerRun(100, func() {
		_ = v.describe()
	})
	require.LessOrEqualf(t, allocs, float64(1),
		"describe() kInt allocated %.0f/call; want ≤1 (strconv.FormatInt)", allocs)
}
