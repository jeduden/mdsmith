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

// --config accepts any .toml file and reads its [tool.mdsmith] table.
func TestE2E_Config_TOMLFlagReadsToolMdsmith(t *testing.T) {
	for _, name := range []string{"pyproject.toml", "foo.toml"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			md := writeFixture(t, dir, "test.md", "# Title\n\nSome text. \n")
			cfg := writeFixture(t, dir, name, "[project]\nname = \"x\"\n\n[tool.mdsmith.rules]\nno-trailing-spaces = false\n")

			_, _, exitCode := runBinary(t, "", "check", md)
			assert.Equal(t, 1, exitCode, "default config flags the trailing space")
			stdout, stderr, exitCode := runBinary(t, "", "check", "--config", cfg, md)
			assert.Equal(t, 0, exitCode, "stdout: %s stderr: %s", stdout, stderr)
		})
	}
}
