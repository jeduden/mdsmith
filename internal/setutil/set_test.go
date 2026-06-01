package setutil_test

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/setutil"
	"github.com/stretchr/testify/assert"
)

func TestContains_Present(t *testing.T) {
	m := map[string]struct{}{"a": {}, "b": {}}
	assert.True(t, setutil.Contains(m, "a"))
	assert.True(t, setutil.Contains(m, "b"))
}

func TestContains_Absent(t *testing.T) {
	m := map[string]struct{}{"a": {}}
	assert.False(t, setutil.Contains(m, "x"))
}

func TestContains_Nil(t *testing.T) {
	assert.False(t, setutil.Contains(nil, "x"))
}

func TestContains_Empty(t *testing.T) {
	assert.False(t, setutil.Contains(map[string]struct{}{}, "x"))
}

func TestFromStrings_Basic(t *testing.T) {
	got := setutil.FromStrings([]string{"x", "y", "z"})
	assert.Equal(t, map[string]struct{}{"x": {}, "y": {}, "z": {}}, got)
}

func TestFromStrings_Empty(t *testing.T) {
	got := setutil.FromStrings([]string{})
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestFromStrings_Nil(t *testing.T) {
	got := setutil.FromStrings(nil)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestFromStrings_Deduplication(t *testing.T) {
	got := setutil.FromStrings([]string{"a", "a", "b"})
	assert.Len(t, got, 2)
	assert.Contains(t, got, "a")
	assert.Contains(t, got, "b")
}
