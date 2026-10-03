//go:build unix || windows || plan9

package build

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckProcTestFile_UnixWindowsTagFails(t *testing.T) {
	src := "//go:build unix || windows\n\npackage build\n"
	errs := checkProcTestFile("x_proc_test.go", []byte(src))
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "plan9")
}

func TestCheckProcTestFile_ProcTagPasses(t *testing.T) {
	src := "//go:build unix || windows || plan9\n\npackage build\n"
	assert.Empty(t, checkProcTestFile("x_proc_test.go", []byte(src)))
}

func TestCheckProcTestFile_OtherTagsIgnored(t *testing.T) {
	for _, tag := range []string{"", "//go:build unix\n\n", "//go:build plan9\n\n", "//go:build js && wasm\n\n"} {
		src := tag + "package build\n\nfunc TestX(t *testing.T) { writeScript(t, \"\", \"a.sh\", \"\") }\n"
		assert.Empty(t, checkProcTestFile("x_test.go", []byte(src)), "tag %q", tag)
	}
}

func TestCheckProcTestFile_ShTestWithoutSkipFails(t *testing.T) {
	src := `//go:build unix || windows || plan9

package build

func TestScript(t *testing.T) { writeScript(t, "", "a.sh", "") }
func TestSh(t *testing.T) { runHook(nil, []string{"sh", "-c", "x"}, "") }
func TestCp(t *testing.T) { recipeCmd("cp {inputs} {outputs}") }
func TestNoSpawn(t *testing.T) { _ = 1 }
`
	errs := checkProcTestFile("x_proc_test.go", []byte(src))
	require.Len(t, errs, 3)
	joined := strings.Join(errs, "\n")
	for _, name := range []string{"TestScript", "TestSh", "TestCp"} {
		assert.Contains(t, joined, name)
	}
}

func TestCheckProcTestFile_ShTestWithSkipPasses(t *testing.T) {
	src := `//go:build unix || windows || plan9

package build

func TestA(t *testing.T) { skipOnPlan9(t); writeScript(t, "", "a.sh", "") }
func TestB(t *testing.T) { skipWithoutPOSIXTools(t, "sh"); runHook(nil, []string{"sh"}, "") }
`
	assert.Empty(t, checkProcTestFile("x_proc_test.go", []byte(src)))
}

func TestCheckProcTestFile_ParseErrorReported(t *testing.T) {
	src := "//go:build unix || windows || plan9\n\npackage build\n\nfunc {\n"
	errs := checkProcTestFile("x_proc_test.go", []byte(src))
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "x_proc_test.go")
}

// TestProcTestFilesCoverPlan9 is the CI guard for plan 2610030243: a
// spawn test file tagged `unix || windows` again would drop out of the
// GOOS=plan9 vet, and an sh test without a plan9 skip would fail on
// plan9.
func TestProcTestFilesCoverPlan9(t *testing.T) {
	assert.Empty(t, checkProcTestDir(t, "."))
}
