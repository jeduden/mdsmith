package release

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleChecksums is a minimal checksums.txt for tests: one Windows
// exe and one Linux binary so parsers can prove they select the right
// line.
const sampleChecksums = `abc123def456abc123def456abc123def456abc123def456abc123def456abc1  mdsmith-linux-amd64
deadbeef1234deadbeef1234deadbeef1234deadbeef1234deadbeef1234dead  mdsmith-windows-amd64.exe
`

func TestParseChecksums_WindowsExe(t *testing.T) {
	hash, err := ParseChecksumFor(strings.NewReader(sampleChecksums), "mdsmith-windows-amd64.exe")
	require.NoError(t, err)
	assert.Equal(t, "deadbeef1234deadbeef1234deadbeef1234deadbeef1234deadbeef1234dead", hash)
}

func TestParseChecksums_LinuxBinary(t *testing.T) {
	hash, err := ParseChecksumFor(strings.NewReader(sampleChecksums), "mdsmith-linux-amd64")
	require.NoError(t, err)
	assert.Equal(t, "abc123def456abc123def456abc123def456abc123def456abc123def456abc1", hash)
}

func TestParseChecksums_Missing(t *testing.T) {
	_, err := ParseChecksumFor(strings.NewReader(sampleChecksums), "mdsmith-darwin-amd64")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mdsmith-darwin-amd64")
	assert.Contains(t, err.Error(), "not found")
}

func TestParseChecksums_EmptyInput(t *testing.T) {
	_, err := ParseChecksumFor(strings.NewReader(""), "mdsmith-windows-amd64.exe")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRenderScoopManifest_SubstitutesVersionURLHash(t *testing.T) {
	manifest := RenderScoopManifest("1.2.3", "deadbeef1234deadbeef1234deadbeef1234deadbeef1234deadbeef1234dead")

	// Version field
	assert.Contains(t, manifest, `"version": "1.2.3"`)

	// URL must point at the release asset for the version.
	wantURL := "https://github.com/jeduden/mdsmith/releases/download/" +
		"v1.2.3/mdsmith-windows-amd64.exe"
	assert.Contains(t, manifest, wantURL)

	// SHA-256 hash
	assert.Contains(t, manifest, "deadbeef1234deadbeef1234deadbeef1234deadbeef1234deadbeef1234dead")

	// bin must expose the mdsmith.exe command.
	assert.Contains(t, manifest, `"bin": "mdsmith.exe"`)

	// autoupdate block so the bucket can self-bump.
	assert.Contains(t, manifest, "autoupdate")
}

func TestRenderScoopManifest_DifferentVersion(t *testing.T) {
	// A second version proves the URL and version fields are
	// parameterised, not hard-coded.
	m1 := RenderScoopManifest("0.13.0", "aaaaaa1111111111111111111111111111111111111111111111111111111111")
	m2 := RenderScoopManifest("0.14.0", "bbbbbb2222222222222222222222222222222222222222222222222222222222")

	assert.Contains(t, m1, "v0.13.0")
	assert.NotContains(t, m1, "v0.14.0")
	assert.Contains(t, m2, "v0.14.0")
	assert.NotContains(t, m2, "v0.13.0")
}

func TestRenderWingetManifest_SubstitutesVersionURLHash(t *testing.T) {
	manifests := RenderWingetManifests("1.2.3", "deadbeef1234deadbeef1234deadbeef1234deadbeef1234deadbeef1234dead")

	// RenderWingetManifests returns the three YAML files WinGet needs:
	// version, installer, and locale. At minimum the combined output
	// must contain the package identifier, version, installer URL,
	// and hash.
	combined := strings.Join(manifests, "\n")

	assert.Contains(t, combined, "jeduden.mdsmith")
	assert.Contains(t, combined, "1.2.3")
	wantURL := "https://github.com/jeduden/mdsmith/releases/download/" +
		"v1.2.3/mdsmith-windows-amd64.exe"
	assert.Contains(t, combined, wantURL)
	assert.Contains(t, combined, "deadbeef1234deadbeef1234deadbeef1234deadbeef1234deadbeef1234dead")
}

func TestRenderWingetManifest_InstallerFields(t *testing.T) {
	manifests := RenderWingetManifests("0.13.0", "cafe0000cafe0000cafe0000cafe0000cafe0000cafe0000cafe0000cafe0000")
	combined := strings.Join(manifests, "\n")

	// Installer type must be exe or inno — Windows exe installs
	// unconventionally for a CLI so we use exe.
	assert.Contains(t, combined, "InstallerType: exe")

	// The architecture for the single Windows asset.
	assert.Contains(t, combined, "x64")
}

func TestRenderWingetManifest_DifferentVersion(t *testing.T) {
	m1 := RenderWingetManifests("0.13.0", "1111111111111111111111111111111111111111111111111111111111111111")
	m2 := RenderWingetManifests("0.14.0", "2222222222222222222222222222222222222222222222222222222222222222")

	c1 := strings.Join(m1, "\n")
	c2 := strings.Join(m2, "\n")
	assert.Contains(t, c1, "0.13.0")
	assert.NotContains(t, c1, "0.14.0")
	assert.Contains(t, c2, "0.14.0")
	assert.NotContains(t, c2, "0.13.0")
}

// failingReader returns an error on its first Read so the
// bufio.Scanner inside ParseChecksumFor stops and surfaces
// scanner.Err() — the read-error branch that a well-formed file or
// an empty reader never reaches.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("simulated read failure")
}

// TestParseChecksums_SkipsBlankCommentAndShortLines exercises the
// three "keep scanning" branches: a blank line, a comment line, and
// a line with fewer than two whitespace-separated fields. The target
// hash sits after all three, so the parser must skip past them.
func TestParseChecksums_SkipsBlankCommentAndShortLines(t *testing.T) {
	input := "\n" +
		"# checksums for v1.2.3\n" +
		"onefield\n" +
		"deadbeef1234deadbeef1234deadbeef1234deadbeef1234deadbeef1234dead  mdsmith-windows-amd64.exe\n"
	hash, err := ParseChecksumFor(strings.NewReader(input), "mdsmith-windows-amd64.exe")
	require.NoError(t, err)
	assert.Equal(t, "deadbeef1234deadbeef1234deadbeef1234deadbeef1234deadbeef1234dead", hash)
}

// TestParseChecksums_ScannerError covers the scanner.Err() branch: a
// reader that fails mid-scan makes ParseChecksumFor return a wrapped
// "read checksums" error rather than a not-found error.
func TestParseChecksums_ScannerError(t *testing.T) {
	_, err := ParseChecksumFor(failingReader{}, "mdsmith-windows-amd64.exe")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read checksums")
}
