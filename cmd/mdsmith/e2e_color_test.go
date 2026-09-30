package main_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCheckOutput_E2EColorFlagsAndEnv runs the real binary with its
// report on a pipe (stderr) or in a file, neither of them a terminal,
// and pins the color precedence: an explicit --color=always or never
// (--no-color is never; the last one given wins) beats the
// environment, --color=auto asks the terminal alone, and without a
// flag NO_COLOR beats FORCE_COLOR. The terminal side is in
// e2e_color_linux_test.go.
func TestCheckOutput_E2EColorFlagsAndEnv(t *testing.T) {
	dir := outputWorkspace(t)
	for _, tc := range []struct {
		name string
		env  []string
		args []string
		want bool
	}{
		{name: "pipe"},
		{name: "FORCE_COLOR=1", env: []string{"FORCE_COLOR=1"}, want: true},
		{name: "FORCE_COLOR=0", env: []string{"FORCE_COLOR=0"}},
		{name: "NO_COLOR beats FORCE_COLOR", env: []string{"NO_COLOR=1", "FORCE_COLOR=1"}},
		{name: "--color=always", args: []string{"--color=always"}, want: true},
		{
			name: "--color always beats NO_COLOR", want: true,
			env: []string{"NO_COLOR=1"}, args: []string{"--color", "always"},
		},
		{name: "--color=never beats FORCE_COLOR", env: []string{"FORCE_COLOR=1"}, args: []string{"--color=never"}},
		{name: "--color=auto ignores FORCE_COLOR", env: []string{"FORCE_COLOR=1"}, args: []string{"--color=auto"}},
		{name: "--no-color beats FORCE_COLOR", env: []string{"FORCE_COLOR=1"}, args: []string{"--no-color"}},
		{name: "last flag wins: always", args: []string{"--no-color", "--color=always"}, want: true},
		{name: "last flag wins: never", args: []string{"--color=always", "--no-color"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{"check"}, tc.args...), "long.md")
			_, stderr, code := runBinaryInDirEnv(t, dir, "", tc.env, args...)
			assert.Equal(t, 1, code)
			assert.Contains(t, stderr, "MDS001")
			assert.Equal(t, tc.want, strings.Contains(stderr, "\033["), "stderr=%q", stderr)
		})
	}

	t.Run("--color=always colors an -o file", func(t *testing.T) {
		_, _, code := runBinaryInDir(t, dir, "", "check", "--color=always", "-o", "color.txt", "long.md")
		assert.Equal(t, 1, code)
		assert.Contains(t, readReport(t, filepath.Join(dir, "color.txt")), "\033[")
	})
	t.Run("FORCE_COLOR leaves an -o file plain", func(t *testing.T) {
		_, _, code := runBinaryInDirEnv(t, dir, "", []string{"FORCE_COLOR=1"}, "check", "-o", "plain.txt", "long.md")
		assert.Equal(t, 1, code)
		report := readReport(t, filepath.Join(dir, "plain.txt"))
		assert.Contains(t, report, "MDS001")
		assert.NotContains(t, report, "\033[")
	})
	t.Run("FORCE_COLOR colors -o -", func(t *testing.T) {
		stdout, _, code := runBinaryInDirEnv(t, dir, "", []string{"FORCE_COLOR=1"}, "check", "-o", "-", "long.md")
		assert.Equal(t, 1, code)
		assert.Contains(t, stdout, "\033[")
	})
	t.Run("bad --color value", func(t *testing.T) {
		stdout, stderr, code := runBinaryInDir(t, dir, "", "check", "--color=sometimes", "long.md")
		assert.Equal(t, 2, code)
		assert.Empty(t, stdout)
		assert.Contains(t, stderr, `invalid argument "sometimes" for "--color" flag: must be auto, always, or never`)
	})
	t.Run("fix --color=always", func(t *testing.T) {
		_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--color=always", "long.md")
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, "\033[")
	})
}
