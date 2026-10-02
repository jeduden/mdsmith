package release

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopLevelFuncs(t *testing.T) {
	src := "package p\n" +
		"import \"testing\"\n" +
		"type r struct{}\n" +
		"func (r) Method() {}\n" +
		"// func Commented() {}\n" +
		"func B(t *testing.T) {}\n" +
		"var v = func() {}\n" +
		"func A() {}\n"
	f, funcs, err := topLevelFuncs([]byte(src))
	require.NoError(t, err)
	require.NotNil(t, f)
	assert.Equal(t, "p", f.Name.Name)
	names := make([]string, 0, len(funcs))
	for _, fn := range funcs {
		names = append(names, fn.Name.Name)
	}
	assert.Equal(t, []string{"B", "A"}, names)

	_, funcs, err = topLevelFuncs([]byte("package p\n"))
	require.NoError(t, err)
	assert.Nil(t, funcs)

	_, _, err = topLevelFuncs([]byte("package p\nfunc X(\n"))
	assert.Error(t, err)
}
