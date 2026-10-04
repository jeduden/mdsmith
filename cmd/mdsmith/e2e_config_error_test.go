package main_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A bad value in the discovered .mdsmith.yml is reported as a
// `file:line:col config message` diagnostic on stderr, exit 2.
func TestE2E_ConfigError_IsPositionedDiagnostic(t *testing.T) {
	for _, cmd := range []string{"check", "fix"} {
		t.Run(cmd, func(t *testing.T) {
			dir := t.TempDir()
			writeFixture(t, dir, "test.md", "# Title\n\nContent here.\n")
			writeFixture(t, dir, ".mdsmith.yml", `rules:
  line-length: true
foreign-regions:
  - start: "<!-- a -->"
    end: ""
`)
			_, stderr, exitCode := runBinaryInDir(t, dir, "", cmd, ".")
			assert.Equal(t, 2, exitCode)
			assert.Contains(t, stderr,
				".mdsmith.yml:5:5 config foreign-regions[0]: end marker must not be empty\n")
			assert.NotContains(t, stderr, "mdsmith: validating config")
		})
	}
}

// A YAML syntax error points at the line the parser reported.
func TestE2E_ConfigError_SyntaxErrorPointsAtLine(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "test.md", "# Title\n\nContent here.\n")
	writeFixture(t, dir, ".mdsmith.yml", "rules: {}\nfiles: [a.md]\n\tbad: x\n")
	_, stderr, exitCode := runBinaryInDir(t, dir, "", "check", ".")
	assert.Equal(t, 2, exitCode)
	assert.Contains(t, stderr, ".mdsmith.yml:3:1 config ")
}
