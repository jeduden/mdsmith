package main_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeUntrustedBuildRepo is writeBuildRepo without the trust marker: the
// build pass should refuse to run recipes until trust is granted.
func writeUntrustedBuildRepo(t *testing.T, recipesYAML string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	cfg := "rules: {}\nbuild:\n  recipes:\n" + recipesYAML
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte(cfg), 0o644))
	return dir
}

func TestE2E_Trust_MissingMarkerBlocksBuildButLintRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp not available on Windows")
	}
	dir := writeUntrustedBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hi"), 0o644))
	// A fixable lint issue (trailing space) so we can prove lint-fix still ran.
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt")+"trailing   \n")

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "doc.md")
	assert.Equal(t, 2, code, "missing trust marker fails the build pass: %s", stderr)
	assert.Contains(t, stderr, "not trusted")
	assert.NoFileExists(t, filepath.Join(dir, "dst.txt"), "untrusted build must not run the recipe")

	// Lint-fix still ran: the trailing whitespace is gone.
	got, err := os.ReadFile(filepath.Join(dir, "doc.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(got), "trailing   ")
}

func TestE2E_Trust_EnvVarIsAlternateSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp not available on Windows")
	}
	dir := writeUntrustedBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hi"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	stdout, stderr, code := runBinaryInDirEnv(t, dir, "", []string{"MDSMITH_TRUST_BUILD=1"},
		"fix", "--no-color", "--build-only", "doc.md")
	assert.Equal(t, 0, code, "MDSMITH_TRUST_BUILD=1 grants trust: %s", stdout+stderr)
	assert.FileExists(t, filepath.Join(dir, "dst.txt"))
}

func TestE2E_Trust_EnvVarDisablingValueDoesNotGrant(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp not available on Windows")
	}
	dir := writeUntrustedBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hi"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	// MDSMITH_TRUST_BUILD=0 must NOT grant trust.
	_, stderr, code := runBinaryInDirEnv(t, dir, "", []string{"MDSMITH_TRUST_BUILD=0"},
		"fix", "--no-color", "--build-only", "doc.md")
	assert.Equal(t, 2, code, "a disabling value leaves the gate in force: %s", stderr)
	assert.Contains(t, stderr, "not trusted")
	assert.NoFileExists(t, filepath.Join(dir, "dst.txt"))
}

func TestE2E_Trust_NoBuildSkipsGate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp not available on Windows")
	}
	dir := writeUntrustedBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hi"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--no-build", "doc.md")
	assert.Equal(t, 0, code, "--no-build skips the gate entirely: %s", stderr)
	assert.NotContains(t, stderr, "not trusted")
	assert.NoFileExists(t, filepath.Join(dir, "dst.txt"))
}

func TestE2E_Trust_StaleMarkerBlocks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	// Tamper with .mdsmith.yml so it no longer matches the trust marker.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("rules: {}\nbuild:\n  recipes:\n    copy:\n      command: cp {inputs} {outputs}\n# drift\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hi"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "changed since it was trusted")
}

func TestE2E_TrustCommand_ShowsDiffAndReTrusts(t *testing.T) {
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	// Drift the config.
	newCfg := "rules: {}\nbuild:\n  recipes:\n    copy:\n      command: install {inputs} {outputs}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte(newCfg), 0o644))

	// `mdsmith trust -y` shows the diff and re-trusts without a prompt.
	stdout, stderr, code := runBinaryInDir(t, dir, "", "trust", "--yes")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, "install")
	assert.Contains(t, stdout, "Trusted")

	// The marker now matches the current config.
	marker, err := os.ReadFile(filepath.Join(dir, ".mdsmith.yml.trust"))
	require.NoError(t, err)
	assert.Equal(t, newCfg, string(marker))
}

func TestE2E_TrustCommand_PromptDeclineLeavesMarker(t *testing.T) {
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	newCfg := "rules: {}\nbuild:\n  recipes:\n    copy:\n      command: install {inputs} {outputs}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte(newCfg), 0o644))

	// Decline at the prompt: answer "n".
	stdout, _, code := runBinaryInDir(t, dir, "n\n", "trust")
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "Aborted")

	// Marker is unchanged (still the original cp command).
	marker, err := os.ReadFile(filepath.Join(dir, ".mdsmith.yml.trust"))
	require.NoError(t, err)
	assert.Contains(t, string(marker), "cp {inputs}")
}

func TestE2E_Build_UndeclaredWriteFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on Windows")
	}
	// A recipe that writes its declared output AND an undeclared sibling in
	// the same parent dir. The post-condition check must fail the build and
	// name the undeclared file.
	dir := writeBuildRepo(t, "    sneaky:\n      command: /bin/sh -c\n")
	// Recipe writes out.txt (declared, staged via {outputs}) and also
	// sneaky.txt next to it in the project tree.
	script := "#!/bin/sh\nprintf ok > \"$1\"\nprintf evil > \"" +
		filepath.Join(dir, "sneaky.txt") + "\"\n"
	scriptPath := filepath.Join(dir, "run.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	dir2 := reconfigureRecipe(t, dir, "    sneaky:\n      command: "+scriptPath+" {outputs}\n")

	writeFixture(t, dir2, "doc.md", buildDirective("sneaky", "", "out.txt"))
	_, stderr, code := runBinaryInDir(t, dir2, "", "fix", "--no-color", "--build-only", "doc.md")
	assert.Equal(t, 2, code, stderr)
	assert.Contains(t, stderr, "outside its declared outputs")
	assert.Contains(t, stderr, "sneaky.txt")
}

func TestE2E_Build_HermeticEnvAllowlistOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh/env not available on Windows")
	}
	dir := writeBuildRepo(t, "")
	// Recipe dumps its environment to the declared output.
	scriptPath := filepath.Join(dir, "dumpenv.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\nenv > \"$1\"\n"), 0o755))
	reconfigureRecipe(t, dir, "    dumpenv:\n      command: "+scriptPath+" {outputs}\n"+
		"  exec:\n    path: \"/usr/bin:/bin\"\n    env-pass-through: [HOME]\n")
	writeFixture(t, dir, "doc.md", buildDirective("dumpenv", "", "env.txt"))

	// Inject a secret into the parent environment; it must not leak in.
	_, stderr, code := runBinaryInDirEnv(t, dir, "",
		[]string{"MDSMITH_TRUST_BUILD=1", "SECRET_TOKEN=leak-me", "HOME=/home/tester"},
		"fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code, stderr)

	got, err := os.ReadFile(filepath.Join(dir, "env.txt"))
	require.NoError(t, err)
	body := string(got)
	assert.Contains(t, body, "PATH=/usr/bin:/bin")
	assert.Contains(t, body, "HOME=/home/tester")
	assert.NotContains(t, body, "SECRET_TOKEN", "non-allowlisted var must not pass through")
	assert.NotContains(t, body, "MDSMITH_TRUST_BUILD", "trust env var must not pass through to recipe")
}

// reconfigureRecipe rewrites .mdsmith.yml (and the trust marker) in dir
// with a new recipes block, returning dir for chaining.
func reconfigureRecipe(t *testing.T, dir, recipesYAML string) string {
	t.Helper()
	cfg := "rules: {}\nbuild:\n  recipes:\n" + recipesYAML
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte(cfg), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml.trust"), []byte(cfg), 0o600))
	return dir
}

func TestE2E_Build_MissingDeclaredOutputFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("true not available on Windows")
	}
	// A recipe that exits 0 without producing its declared output.
	dir := writeBuildRepo(t, "    noop:\n      command: true {outputs}\n")
	writeFixture(t, dir, "doc.md", buildDirective("noop", "", "out.txt"))

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "did not produce declared output")
	assert.NoFileExists(t, filepath.Join(dir, "out.txt"))
}

func TestE2E_Build_GroupWritableStagingRootRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hi"), 0o644))
	// Pre-create a group-writable staging root.
	staging := filepath.Join(dir, ".mdsmith", "build-staging")
	require.NoError(t, os.MkdirAll(staging, 0o700))
	require.NoError(t, os.Chmod(staging, 0o770))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "writable")
}

func TestE2E_Build_TimeoutKillsSpawnedChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group kill tested on Unix")
	}
	dir := writeBuildRepo(t, "")
	// Recipe spawns a long-lived child that records its PID, then sleeps.
	pidFile := filepath.Join(dir, "child.pid")
	script := "#!/bin/sh\nsleep 120 & echo $! > \"" + pidFile + "\"\nsleep 120\ntouch \"$1\"\n"
	scriptPath := filepath.Join(dir, "spawn.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	reconfigureRecipe(t, dir, "    spawn:\n      command: "+scriptPath+" {outputs}\n")
	writeFixture(t, dir, "doc.md", buildDirective("spawn", "", "out.txt"))

	_, stderr, code := runBinaryInDir(t, dir, "",
		"fix", "--no-color", "--build-only", "--build-timeout", "1s", "doc.md")
	assert.Equal(t, 2, code, stderr)
	assert.Contains(t, stderr, "TIMEOUT")

	// The spawned child must be reaped, not orphaned.
	var childPID int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(pidFile)
		if err == nil {
			if n, perr := strconv.Atoi(strings.TrimSpace(string(b))); perr == nil {
				childPID = n
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NotZero(t, childPID, "child pid should have been recorded")
	assert.Eventually(t, func() bool {
		return !unixProcessAlive(childPID)
	}, 6*time.Second, 100*time.Millisecond, "spawned child must not be orphaned")
}
