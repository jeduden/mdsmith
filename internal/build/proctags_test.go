//go:build unix || windows || plan9

package build

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// procPrelude mirrors the package's real sh and skip helpers, so the
// checker resolves them from their bodies as it does in the package.
const procPrelude = `package build

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(name, []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	return name
}
func skipOnPlan9(t testing.TB) {
	t.Helper()
	if runtime.GOOS == "plan9" {
		t.Skip("plan9 has no sh")
	}
}
func skipWithoutPOSIXTools(t testing.TB, tools string) {
	t.Helper()
	switch runtime.GOOS {
	case "plan9":
		t.Skip(tools + " needs POSIX tools")
	case "windows":
		t.Skip(tools + " is not available on Windows")
	}
}
`

// checkProcTestFile checks one file of a package that also holds
// procPrelude.
func checkProcTestFile(name string, src []byte) []string {
	return checkProcTestPkg(map[string][]byte{name: src})
}

// checkProcTestPkg checks files as one package that also holds
// procPrelude.
func checkProcTestPkg(files map[string][]byte) []string {
	files["prelude_test.go"] = []byte(procPrelude)
	return checkProcTestFiles(files)
}

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
	for _, tag := range []string{"", "//go:build unix\n\n", "//go:build js && wasm\n\n"} {
		src := tag + "package build\n\nfunc TestX(t *testing.T) { writeScript(t, \"\", \"a.sh\", \"\") }\n"
		assert.Empty(t, checkProcTestFile("x_test.go", []byte(src)), "tag %q", tag)
	}
}

// An sh script, an sh argv, or an sh recipe needs a plan9 skip; a
// recipe that runs no sh, such as cp, does not, since plan9 has cp.
func TestCheckProcTestFile_ShTestWithoutSkipFails(t *testing.T) {
	src := `//go:build unix || windows || plan9

package build

func TestScript(t *testing.T) { writeScript(t, "", "a.sh", "") }
func TestSh(t *testing.T) { runHook(nil, []string{"sh", "-c", "x"}, "") }
func TestShRecipe(t *testing.T) { recipeCmd("sh -c x") }
func TestCp(t *testing.T) { recipeCmd("cp {inputs} {outputs}") }
func TestNoSpawn(t *testing.T) { _ = 1 }
`
	errs := checkProcTestFile("x_proc_test.go", []byte(src))
	joined := strings.Join(errs, "\n")
	require.Len(t, errs, 3, joined)
	for _, name := range []string{"TestScript ", "TestSh ", "TestShRecipe "} {
		assert.Contains(t, joined, name)
	}
}

// A file that builds on plan9 but not js runs its tests only where a
// process can start, plan9 among them, so its sh tests need the skip
// even when it is not a spawn file. One that also builds on js is left
// to the js/wasm gate.
func TestCheckProcTestFile_Plan9NonJSFilesChecked(t *testing.T) {
	for _, tag := range []string{"//go:build unix || plan9\n\n", "//go:build plan9\n\n"} {
		src := tag + "package build\n\nfunc TestX(t *testing.T) { writeScript(t, \"\", \"a.sh\", \"\") }\n"
		errs := checkProcTestFile("x_test.go", []byte(src))
		require.Len(t, errs, 1, "tag %q", tag)
		assert.Contains(t, errs[0], "TestX ", "tag %q", tag)
	}
}

// A plan9 skip written inline, as an if or a switch on runtime.GOOS,
// counts as the helpers do, so a package without them can comply. A
// skip under any other condition does not.
func TestCheckProcTestFile_InlinePlan9SkipPasses(t *testing.T) {
	src := `//go:build unix || windows || plan9

package build

func TestIf(t *testing.T) {
	if runtime.GOOS == "plan9" {
		t.Skip("no sh")
	}
	runHook(nil, []string{"sh"}, "")
}
func TestSwitch(t *testing.T) {
	switch runtime.GOOS {
	case "windows", "plan9":
		t.Skipf("no %s", "sh")
	}
	runHook(nil, []string{"sh"}, "")
}
func TestWrongOS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available")
	}
	runHook(nil, []string{"sh"}, "")
}
`
	errs := checkProcTestFile("x_proc_test.go", []byte(src))
	require.Len(t, errs, 1, strings.Join(errs, "\n"))
	assert.Contains(t, errs[0], "TestWrongOS ")
}

// Comparing a name with "sh" runs nothing, so it is no sh use.
func TestCheckProcTestFile_ShComparisonIsNoUse(t *testing.T) {
	src := `//go:build unix || windows || plan9

package build

func isSh(name string) bool { return name == "sh" || "/bin/sh" != name }
func TestCompare(t *testing.T) { _ = isSh("x") }
`
	assert.Empty(t, checkProcTestFile("x_proc_test.go", []byte(src)))
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
	errs := checkProcTestPkg(map[string][]byte{
		"helpers_test.go": []byte(helpers),
		"x_proc_test.go":  []byte(proc),
	})
	joined := strings.Join(errs, "\n")
	require.Len(t, errs, 3, joined)
	for _, name := range []string{"TestScriptRecipe", "TestShArgv", "TestViaHelper"} {
		assert.Contains(t, joined, name)
	}
}

// A helper's verdict does not depend on declaration order: one that
// calls a later-declared sh helper before its own skip needs sh, and
// one that calls a later-declared skip helper before sh is a skip.
func TestCheckProcTestFiles_HelperOrderIndependent(t *testing.T) {
	helpers := `package build

func shThenSkip(t *testing.T) { mkScript(t); skipOnPlan9(t) }
func skipThenSh(t *testing.T) { skipNoSh(t); writeScript(t, "", "a.sh", "") }
func mkScript(t *testing.T) { writeScript(t, "", "a.sh", "") }
func skipNoSh(t *testing.T) { skipOnPlan9(t) }
`
	proc := `//go:build unix || windows || plan9

package build

func TestShThenSkip(t *testing.T) { shThenSkip(t) }
func TestSkipThenSh(t *testing.T) { skipThenSh(t) }
`
	errs := checkProcTestPkg(map[string][]byte{
		"helpers_test.go": []byte(helpers),
		"x_proc_test.go":  []byte(proc),
	})
	joined := strings.Join(errs, "\n")
	require.Len(t, errs, 1, joined)
	assert.Contains(t, joined, "TestShThenSkip ")
}

// A recursive helper resolves: its call to itself reaches neither, so
// what follows decides it.
func TestCheckProcTestFiles_RecursiveHelperResolves(t *testing.T) {
	src := `//go:build unix || windows || plan9

package build

func again(t *testing.T) { again(t); writeScript(t, "", "a.sh", "") }
func TestAgain(t *testing.T) { again(t) }
`
	errs := checkProcTestFile("x_proc_test.go", []byte(src))
	require.Len(t, errs, 1, strings.Join(errs, "\n"))
	assert.Contains(t, errs[0], "TestAgain ")
}

// Only plan9's build of the package runs a spawn test there, so only a
// helper that builds on plan9 decides whether a call to it needs sh.
func TestCheckProcTestFiles_OnlyPlan9HelpersCount(t *testing.T) {
	files := map[string][]byte{
		"a_unix_test.go":  []byte("package build\n\nfunc mk(t *testing.T) { skipOnPlan9(t) }\n"),
		"b_plan9_test.go": []byte("package build\n\nfunc mk(t *testing.T) { writeScript(t, \"\", \"a.sh\", \"\") }\n"),
		"x_proc_test.go": []byte("//go:build unix || windows || plan9\n\npackage build\n\n" +
			"func TestX(t *testing.T) { mk(t) }\n"),
	}
	errs := checkProcTestPkg(files)
	require.Len(t, errs, 1, strings.Join(errs, "\n"))
	assert.Contains(t, errs[0], "TestX ")
}

// A helper passed by name runs as surely as one called directly, a
// /bin/sh argv needs sh as "sh" does, a method or a func-valued var is
// a helper too, and go test runs a fuzz target's seeds and, under
// -bench, a benchmark, so each of these needs a skip first.
func TestCheckProcTestFile_IndirectShUsesFail(t *testing.T) {
	src := `//go:build unix || windows || plan9

package build

type fx struct{}

func (fx) script(t *testing.T) { writeScript(t, "", "a.sh", "") }

var mkScript = func(t *testing.T) { writeScript(t, "", "a.sh", "") }

func scriptSub(t *testing.T) { writeScript(t, "", "a.sh", "") }
func TestFuncRef(t *testing.T) { t.Run("a", scriptSub) }
func TestBinSh(t *testing.T) { runHook(nil, []string{"/bin/sh", "-c", "x"}, "") }
func TestMethod(t *testing.T) { fx{}.script(t) }
func TestMethodRef(t *testing.T) { t.Run("a", fx{}.script) }
func TestFuncVar(t *testing.T) { mkScript(t) }
func FuzzScript(f *testing.F) { f.Fuzz(func(t *testing.T, s string) { writeScript(t, "", "a.sh", s) }) }
func FuzzSkipped(f *testing.F) {
	f.Fuzz(func(t *testing.T, s string) { skipOnPlan9(t); writeScript(t, "", "a.sh", s) })
}
func BenchmarkScript(b *testing.B) { runHook(nil, []string{"sh"}, "") }
func BenchmarkSkipped(b *testing.B) { skipOnPlan9(b); runHook(nil, []string{"sh"}, "") }
`
	errs := checkProcTestFile("x_proc_test.go", []byte(src))
	joined := strings.Join(errs, "\n")
	want := []string{
		"TestFuncRef", "TestBinSh", "TestMethod", "TestMethodRef",
		"TestFuncVar", "FuzzScript", "BenchmarkScript",
	}
	require.Len(t, errs, len(want), joined)
	for _, name := range want {
		assert.Contains(t, joined, name+" ")
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
// their test files are checked here too. Plan 2610031920 moves this
// guard to the module level.
func TestProcTestFilesCoverPlan9(t *testing.T) {
	for _, dir := range []string{".", "../release", "../../cmd/mdsmith-release"} {
		assert.Empty(t, checkProcTestDir(t, dir), "dir %s", dir)
	}
}
