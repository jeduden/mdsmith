package release

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// relKey is the slash path LoadChannels passes to the extractor.
func relKey(name string) string {
	return filepath.ToSlash(filepath.Join(ChannelDir, name))
}

// mkChannelDoc builds a conformant extract envelope for one channel.
func mkChannelDoc(title, mech, art, cmd, aud string, weight int) channelDoc {
	var d channelDoc
	d.Frontmatter.Title = title
	d.Frontmatter.Summary = "summary of " + title
	d.Frontmatter.Mechanism = mech
	d.Frontmatter.Artifact = art
	d.Frontmatter.Command = cmd
	d.Frontmatter.Audience = aud
	d.Frontmatter.ChannelURL = "https://example.test/" + title
	d.Frontmatter.Weight = weight
	return d
}

// stubExtractAll swaps the all-files extract seam for canned JSON
// keyed by rel path and restores it when the test ends.
func stubExtractAll(t *testing.T, byRel map[string]channelDoc) {
	t.Helper()
	prev := channelsExtractAll
	t.Cleanup(func() { channelsExtractAll = prev })
	channelsExtractAll = func(_ string, rels []string) (map[string][]byte, error) {
		out := make(map[string][]byte, len(rels))
		for _, rel := range rels {
			doc, ok := byRel[rel]
			require.Truef(t, ok, "no extractor stub for %s", rel)
			b, err := json.Marshal(doc)
			require.NoError(t, err)
			out[rel] = b
		}
		return out, nil
	}
}

// seedChannelDir creates a repo root with the given channel files.
func seedChannelDir(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, filepath.FromSlash(ChannelDir))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, f := range files {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, f), []byte("# x\n"), 0o644))
	}
	return root
}

func TestLoadChannelsSortsByWeightAndSkipsProto(t *testing.T) {
	root := seedChannelDir(t, "a.md", "b.md", "proto.md")
	stubExtractAll(t, map[string]channelDoc{
		relKey("a.md"): mkChannelDoc("A", "push", "cli", "cmd a", "aud a", 5),
		relKey("b.md"): mkChannelDoc("B", "pull", "cli", "cmd b", "aud b", 1),
		// no proto.md stub: channelFiles must exclude it.
	})
	chs, err := LoadChannels(root)
	require.NoError(t, err)
	require.Len(t, chs, 2)
	assert.Equal(t, "B", chs[0].Title, "weight 1 sorts first")
	assert.Equal(t, "A", chs[1].Title)
}

func TestLoadChannels_MissingDirErrors(t *testing.T) {
	_, err := LoadChannels(t.TempDir()) // no channel dir at all
	require.Error(t, err)
}

func TestLoadChannels_EmptyDirErrors(t *testing.T) {
	root := seedChannelDir(t) // dir exists, but no channel files
	_, err := LoadChannels(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no channel files")
}

func TestLoadChannels_ExtractorError(t *testing.T) {
	root := seedChannelDir(t, "a.md")
	prev := channelsExtractAll
	t.Cleanup(func() { channelsExtractAll = prev })
	channelsExtractAll = func(string, []string) (map[string][]byte, error) {
		return nil, errors.New("boom")
	}
	_, err := LoadChannels(root)
	require.Error(t, err)
}

func TestLoadChannels_BadJSON(t *testing.T) {
	root := seedChannelDir(t, "a.md")
	prev := channelsExtractAll
	t.Cleanup(func() { channelsExtractAll = prev })
	channelsExtractAll = func(_ string, rels []string) (map[string][]byte, error) {
		return map[string][]byte{rels[0]: []byte("not json")}, nil
	}
	_, err := LoadChannels(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestChannelValidate_RequiredFields(t *testing.T) {
	base := Channel{
		Title: "T", Summary: "S", Mechanism: "push", Artifact: "cli",
		Command: "c", Audience: "a", URL: "https://x", Weight: 1,
	}
	require.NoError(t, base.validate("x"))

	for _, tc := range []struct {
		name string
		mut  func(*Channel)
	}{
		{"title", func(c *Channel) { c.Title = "" }},
		{"summary", func(c *Channel) { c.Summary = "" }},
		{"mechanism", func(c *Channel) { c.Mechanism = "" }},
		{"artifact", func(c *Channel) { c.Artifact = "" }},
		{"command", func(c *Channel) { c.Command = "" }},
		{"audience", func(c *Channel) { c.Audience = "" }},
		{"url", func(c *Channel) { c.URL = "" }},
	} {
		c := base
		tc.mut(&c)
		err := c.validate("x")
		require.Error(t, err, tc.name)
		assert.Contains(t, err.Error(), tc.name)
	}

	zeroWeight := base
	zeroWeight.Weight = 0
	err := zeroWeight.validate("x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "weight")
}

func TestRenderChannelsYAML(t *testing.T) {
	out := RenderChannelsYAML([]Channel{{
		Title: "Go", Command: "go install", Mechanism: "toolchain",
		Artifact: "cli", Audience: "devs", Platforms: []string{"go"},
		URL: "https://example.test", Weight: 1,
	}})
	s := string(out)
	assert.Contains(t, s, "do not edit by hand")
	assert.Contains(t, s, "- title: Go")
	assert.Contains(t, s, "command: go install")
	assert.Contains(t, s, "platforms:")
}

func fixtureChannels() []Channel {
	return []Channel{{
		Title: "A", Summary: "s", Mechanism: "push", Artifact: "cli",
		Command: "cmd", Audience: "aud", Platforms: []string{"go"},
		URL: "https://example.test/a", Weight: 1,
	}}
}

func TestWriteAndCheckChannelsData(t *testing.T) {
	root := t.TempDir()
	chs := fixtureChannels()
	dataPath := filepath.Join(root, filepath.FromSlash(ChannelsDataFile))

	// Missing data file counts as drift.
	drift, err := CheckChannelsData(root, chs)
	require.NoError(t, err)
	assert.True(t, drift)

	changed, err := WriteChannelsData(root, chs)
	require.NoError(t, err)
	assert.True(t, changed)

	data, err := os.ReadFile(dataPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "title: A")

	// Re-write is a byte-stable no-op; check now passes.
	changed, err = WriteChannelsData(root, chs)
	require.NoError(t, err)
	assert.False(t, changed)
	drift, err = CheckChannelsData(root, chs)
	require.NoError(t, err)
	assert.False(t, drift)

	// Hand-editing the data file is drift again.
	require.NoError(t, os.WriteFile(dataPath, []byte("# tampered\n"), 0o644))
	drift, err = CheckChannelsData(root, chs)
	require.NoError(t, err)
	assert.True(t, drift)
}

func TestCheckChannelsData_ReadError(t *testing.T) {
	root := t.TempDir()
	// Make the data file a DIRECTORY so ReadFile returns a
	// non-NotExist error.
	require.NoError(t, os.MkdirAll(
		filepath.Join(root, filepath.FromSlash(ChannelsDataFile)), 0o755))
	_, err := CheckChannelsData(root, fixtureChannels())
	require.Error(t, err)
}

// repoRootForChannels resolves the repo root from this test file.
func repoRootForChannels(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestLoadChannels_RealExtract(t *testing.T) {
	if testing.Short() {
		t.Skip("builds cmd/mdsmith; skipped under -short")
	}
	chs, err := LoadChannels(repoRootForChannels(t))
	require.NoError(t, err)
	require.NotEmpty(t, chs)
	// The real extract+decode path must populate the projected
	// fields — catches a channelDoc json-tag or proto.md field drift
	// the stubbed tests cannot see.
	for _, c := range chs {
		assert.NotEmpty(t, c.Title)
		assert.NotEmpty(t, c.Command)
		assert.NotEmpty(t, c.URL, "channelurl must map to URL")
		assert.Greater(t, c.Weight, 0)
	}
}

func TestLoadChannels_MissingExtractOutput(t *testing.T) {
	root := seedChannelDir(t, "a.md")
	prev := channelsExtractAll
	t.Cleanup(func() { channelsExtractAll = prev })
	channelsExtractAll = func(string, []string) (map[string][]byte, error) {
		return map[string][]byte{}, nil // no output for a.md
	}
	_, err := LoadChannels(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no output")
}

func TestExtractAllChannels_BuildFails(t *testing.T) {
	// t.TempDir() has no ./cmd/mdsmith package, so `go build
	// ./cmd/mdsmith` (with cmd.Dir set to the temp dir) fails —
	// covering buildMdsmith's error path.
	_, err := extractAllChannels(t.TempDir(), []string{relKey("a.md")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build mdsmith")
}

func TestRunExtract_BadBinary(t *testing.T) {
	_, err := runExtract(
		filepath.Join(t.TempDir(), "nope"), t.TempDir(), "x.md")
	require.Error(t, err)
}

func TestRunExtract_ExitErrorCapturesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell stub")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "fake")
	require.NoError(t, os.WriteFile(stub,
		[]byte("#!/bin/sh\necho boom >&2\nexit 3\n"), 0o755))
	_, err := runExtract(stub, dir, "x.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom", "stderr is captured")
}

func TestWriteChannelsData_ReadError(t *testing.T) {
	root := t.TempDir()
	// The data file is a directory, so ReadFile returns a
	// non-NotExist error that WriteChannelsData must surface
	// instead of masking it with a write.
	require.NoError(t, os.MkdirAll(
		filepath.Join(root, filepath.FromSlash(ChannelsDataFile)), 0o755))
	_, err := WriteChannelsData(root, fixtureChannels())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read")
}

func TestMdsmithBinName(t *testing.T) {
	assert.Equal(t, "mdsmith.exe", mdsmithBinName("windows"))
	assert.Equal(t, "mdsmith", mdsmithBinName("linux"))
	assert.Equal(t, "mdsmith", mdsmithBinName("darwin"))
}

func TestBuildMdsmith_TempDirError(t *testing.T) {
	if runtime.GOOS == "windows" {
		// os.TempDir reads TEMP/TMP on Windows, not TMPDIR, so this
		// env trick can't force MkdirTemp to fail there.
		t.Skip("TMPDIR is not honored by os.TempDir on Windows")
	}
	base := t.TempDir()
	// Point TMPDIR at a non-existent directory so os.MkdirTemp fails.
	t.Setenv("TMPDIR", filepath.Join(base, "missing"))
	_, _, err := buildMdsmith(base)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tempdir")
}

func TestLoadChannels_ValidateError(t *testing.T) {
	root := seedChannelDir(t, "a.md")
	stubExtractAll(t, map[string]channelDoc{
		// empty command fails validate inside LoadChannels.
		relKey("a.md"): mkChannelDoc("A", "push", "cli", "", "aud", 1),
	})
	_, err := LoadChannels(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command")
}

func TestExtractAllChannels_RunExtractError(t *testing.T) {
	if testing.Short() {
		t.Skip("builds cmd/mdsmith; skipped under -short")
	}
	// The build succeeds against the real repo, but extracting a
	// non-existent channel file makes runExtract fail mid-loop.
	_, err := extractAllChannels(repoRootForChannels(t),
		[]string{"docs/development/release-channels/does-not-exist.md"})
	require.Error(t, err)
}
