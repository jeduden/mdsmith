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

// The tag is matched the way go build matches it, so release and
// compiler tags, file-name GOOS suffixes, and +build lines count.
func TestCheckProcTestFile_GoBuildMatchingRules(t *testing.T) {
	for _, src := range []string{
		"//go:build go1.21 && (unix || windows)\n\npackage build\n",
		"//go:build gc && (unix || windows)\n\npackage build\n",
		"// +build linux windows\n\npackage build\n",
	} {
		errs := checkProcTestFile("x_proc_test.go", []byte(src))
		require.Len(t, errs, 1, "src %q", src)
		assert.Contains(t, errs[0], "plan9")
	}
	// A _windows file never builds on linux, so it is not a spawn file.
	src := "package build\n\nfunc TestX(t *testing.T) { writeScript(t, \"\", \"a.sh\", \"\") }\n"
	assert.Empty(t, checkProcTestFile("x_windows_test.go", []byte(src)))
}

// A spawn file must build exactly where exec_other.go's stubs do not:
// a tag that also builds on wasip1 or on a future port fails.
func TestCheckProcTestFile_NotComplementOfStubsFails(t *testing.T) {
	for _, tc := range []struct{ tag, want string }{
		{"!js", "wasip1"},
		{"!js && !wasip1", "a future port"},
	} {
		src := "//go:build " + tc.tag + "\n\npackage build\n"
		errs := checkProcTestFile("x_proc_test.go", []byte(src))
		require.Len(t, errs, 1, "tag %q", tc.tag)
		assert.Contains(t, errs[0], tc.want, "tag %q", tc.tag)
	}
}

func TestCheckProcTestFile_MalformedTagReported(t *testing.T) {
	src := "//go:build unix ||\n\npackage build\n"
	errs := checkProcTestFile("x_proc_test.go", []byte(src))
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "x_proc_test.go")
}

// The skip must come first, unconditionally, at the top of the test:
// one after the sh use, in a branch, or deferred leaves plan9 running
// the script.
func TestCheckProcTestFile_LateOrConditionalSkipFails(t *testing.T) {
	src := `//go:build unix || windows || plan9

package build

func TestLate(t *testing.T) { writeScript(t, "", "a.sh", ""); skipOnPlan9(t) }
func TestBranch(t *testing.T) { if ok { skipOnPlan9(t) }; writeScript(t, "", "a.sh", "") }
func TestDefer(t *testing.T) { defer skipOnPlan9(t); writeScript(t, "", "a.sh", "") }
func TestBranchMsg(t *testing.T) { if ok { skipWithoutPOSIXTools(t, "sh") } }
`
	errs := checkProcTestFile("x_proc_test.go", []byte(src))
	joined := strings.Join(errs, "\n")
	require.Len(t, errs, 3, joined)
	for _, name := range []string{"TestLate", "TestBranch", "TestDefer"} {
		assert.Contains(t, joined, name+" ")
	}
}

// A subtest that skips first covers its own sh use.
func TestCheckProcTestFile_SubtestSkipPasses(t *testing.T) {
	src := `//go:build unix || windows || plan9

package build

func TestSub(t *testing.T) {
	t.Run("a", func(t *testing.T) { skipOnPlan9(t); writeScript(t, "", "a.sh", "") })
}
func TestSubNoSkip(t *testing.T) {
	t.Run("a", func(t *testing.T) { writeScript(t, "", "a.sh", "") })
}
`
	errs := checkProcTestFile("x_proc_test.go", []byte(src))
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "TestSubNoSkip")
}

// A helper, in any test file of the package, that reaches sh before a
// skip makes its callers need a skip; one that skips first is a skip.
func TestCheckProcTestFiles_HelpersFollowed(t *testing.T) {
	helpers := `package build

func scriptRecipe(t *testing.T) RecipeSpec { return recipeCmd(writeScript(t, "", "a.sh", "")) }
func shArgv() []string { return []string{"sh", "-c", "x"} }
func viaHelper(t *testing.T) RecipeSpec { return scriptRecipe(t) }
func skipNoSh(t *testing.T) { t.Helper(); skipOnPlan9(t) }
`
	proc := `//go:build unix || windows || plan9

package build

func TestScriptRecipe(t *testing.T) { scriptRecipe(t) }
func TestShArgv(t *testing.T) { runHook(nil, shArgv(), "") }
func TestViaHelper(t *testing.T) { viaHelper(t) }
func TestCustomSkip(t *testing.T) { skipNoSh(t); scriptRecipe(t) }
`
	errs := checkProcTestFiles(map[string][]byte{
		"helpers_test.go": []byte(helpers),
		"x_proc_test.go":  []byte(proc),
	})
	joined := strings.Join(errs, "\n")
	require.Len(t, errs, 3, joined)
	for _, name := range []string{"TestScriptRecipe", "TestShArgv", "TestViaHelper"} {
		assert.Contains(t, joined, name)
	}
}

// Only what go test runs as a test is checked: not a TestXxx method,
// not a Testlower helper, and not a body-less declaration.
func TestCheckProcTestFile_OnlyTestFuncsChecked(t *testing.T) {
	src := `//go:build unix || windows || plan9

package build

type s struct{}

func (s) TestMethod(t *testing.T) { _ = recipeCmd("x") }
func Testable(t *testing.T) { _ = recipeCmd("x") }
func TestExtern(t *testing.T)
`
	assert.Empty(t, checkProcTestFile("x_proc_test.go", []byte(src)))
}

// TestProcTestFilesCoverPlan9 is the CI guard for plan 2610030243: a
// spawn test file tagged `unix || windows` again would drop out of the
// GOOS=plan9 vet, and an sh test without a plan9 skip would fail on
// plan9. The release-tooling packages carry the same spawn tag, so
// their test files are checked here too.
func TestProcTestFilesCoverPlan9(t *testing.T) {
	for _, dir := range []string{".", "../release", "../../cmd/mdsmith-release"} {
		assert.Empty(t, checkProcTestDir(t, dir), "dir %s", dir)
	}
}
