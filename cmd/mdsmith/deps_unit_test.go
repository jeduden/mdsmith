package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// depsWorkspace creates a minimal project (.git + .mdsmith.yml + two
// linked docs) and chdirs into it so runDeps's discovery + config
// walk resolve against it, not the repo root.
func depsWorkspace(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	wf := func(rel, body string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	wf(".mdsmith.yml", "files:\n  - \"**/*.md\"\nrules:\n  cross-file-reference-integrity: false\n")
	wf("a.md", "# A\n\nSee [b](b.md).\n")
	wf("b.md", "# B\n")
	t.Chdir(dir)
}

func TestEdgeKindString(t *testing.T) {
	cases := []struct {
		k    index.EdgeKind
		want string
	}{
		{index.EdgeAnchorLink, "anchor-link"},
		{index.EdgeFileLink, "file-link"},
		{index.EdgeRefLink, "ref-link"},
		{index.EdgeInclude, "include"},
		{index.EdgeCatalog, "catalog"},
		{index.EdgeBuild, "build"},
		{index.EdgeKind(999), "unknown"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, edgeKindString(tc.k))
	}
}

func TestEdgeTargetString(t *testing.T) {
	t.Run("file link with anchor", func(t *testing.T) {
		e := index.Edge{Kind: index.EdgeFileLink, TargetFile: "docs/api.md", TargetAnchor: "auth"}
		assert.Equal(t, "docs/api.md#auth", edgeTargetString(e, "docs/index.md"))
	})
	t.Run("file link no anchor", func(t *testing.T) {
		e := index.Edge{Kind: index.EdgeFileLink, TargetFile: "docs/api.md"}
		assert.Equal(t, "docs/api.md", edgeTargetString(e, "docs/index.md"))
	})
	t.Run("same-file anchor", func(t *testing.T) {
		e := index.Edge{Kind: index.EdgeAnchorLink, TargetAnchor: "setup"}
		assert.Equal(t, "#setup", edgeTargetString(e, "docs/index.md"))
	})
	t.Run("ref link", func(t *testing.T) {
		e := index.Edge{Kind: index.EdgeRefLink, TargetLabel: "spec"}
		assert.Equal(t, "[spec]", edgeTargetString(e, "docs/index.md"))
	})
	t.Run("unresolved catalog", func(t *testing.T) {
		e := index.Edge{Kind: index.EdgeCatalog, Unresolved: true}
		assert.Equal(t, "(glob)", edgeTargetString(e, "docs/index.md"))
	})
}

func TestCollectDeps_Outgoing(t *testing.T) {
	idx := index.New("/ws")
	src := map[string][]byte{
		"a.md": []byte("# A\n\nSee [b](b.md#sec).\n"),
		"b.md": []byte("# B\n\n## Sec\n"),
	}
	idx.BuildSerial([]string{"a.md", "b.md"}, func(p string) ([]byte, error) {
		return src[p], nil
	})
	recs := collectDeps(idx, "a.md", false)
	require.Len(t, recs, 1)
	assert.Equal(t, "a.md", recs[0].Source)
	assert.Equal(t, "file-link", recs[0].Kind)
	assert.Equal(t, "b.md#sec", recs[0].Target)
}

func TestCollectDeps_Incoming(t *testing.T) {
	idx := index.New("/ws")
	src := map[string][]byte{
		"a.md": []byte("# A\n\nSee [b](b.md).\n"),
		"b.md": []byte("# B\n"),
	}
	idx.BuildSerial([]string{"a.md", "b.md"}, func(p string) ([]byte, error) {
		return src[p], nil
	})
	recs := collectDeps(idx, "b.md", true)
	require.Len(t, recs, 1)
	assert.Equal(t, "a.md", recs[0].Source)
	assert.Equal(t, "b.md", recs[0].Target)
}

func TestEmitDeps_Text(t *testing.T) {
	var buf bytes.Buffer
	code := emitDeps(&buf, []depRecord{
		{Source: "a.md", Line: 3, Kind: "file-link", Target: "b.md#sec"},
	}, "text")
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.md:3: file-link b.md#sec\n", buf.String())
}

func TestEmitDeps_JSON(t *testing.T) {
	var buf bytes.Buffer
	code := emitDeps(&buf, []depRecord{
		{Source: "a.md", Line: 3, Kind: "include", Target: "frag.md"},
	}, "json")
	assert.Equal(t, 0, code)
	var got []depRecord
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "frag.md", got[0].Target)
}

func TestEmitDeps_JSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	code := emitDeps(&buf, nil, "json")
	assert.Equal(t, 1, code)
	assert.Equal(t, "[]\n", strings.TrimSpace(buf.String())+"\n")
}

func TestEmitDeps_NoneText(t *testing.T) {
	var buf bytes.Buffer
	code := emitDeps(&buf, nil, "text")
	assert.Equal(t, 1, code)
	assert.Empty(t, buf.String())
}

func TestEmitDeps_UnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	code := emitDeps(&buf, []depRecord{{Source: "a.md"}}, "yaml")
	assert.Equal(t, 2, code)
}

func TestEdgeTargetString_WholeSelfFile(t *testing.T) {
	// No target file and no anchor: a whole-file same-file edge
	// renders as the originating file.
	e := index.Edge{Kind: index.EdgeFileLink}
	assert.Equal(t, "docs/index.md", edgeTargetString(e, "docs/index.md"))
}

func TestCollectDeps_OutgoingSorted(t *testing.T) {
	idx := index.New("/ws")
	// Two links on the same line (line 3) plus one on a later line,
	// so collectDeps must sort by line then target.
	src := map[string][]byte{
		"a.md": []byte("# A\n\n[z](z.md) and [m](m.md)\n\n[q](q.md)\n"),
		"z.md": []byte("# Z\n"),
		"m.md": []byte("# M\n"),
		"q.md": []byte("# Q\n"),
	}
	idx.BuildSerial([]string{"a.md", "z.md", "m.md", "q.md"}, func(p string) ([]byte, error) {
		return src[p], nil
	})
	recs := collectDeps(idx, "a.md", false)
	require.Len(t, recs, 3)
	assert.Equal(t, "m.md", recs[0].Target)
	assert.Equal(t, "z.md", recs[1].Target)
	assert.Equal(t, 3, recs[0].Line)
	assert.Equal(t, "q.md", recs[2].Target)
}

func TestEmitDeps_TextWriteError(t *testing.T) {
	code := emitDeps(failingWriter{}, []depRecord{
		{Source: "a.md", Line: 1, Kind: "file-link", Target: "b.md"},
	}, "text")
	assert.Equal(t, 2, code)
}

func TestEmitDeps_JSONWriteError(t *testing.T) {
	code := emitDeps(failingWriter{}, []depRecord{{Source: "a.md"}}, "json")
	assert.Equal(t, 2, code)
}

func TestParseDepsFlags(t *testing.T) {
	t.Run("valid flags", func(t *testing.T) {
		opts, pos, err := parseDepsFlags([]string{"--incoming", "--format", "json", "docs/api.md"})
		require.NoError(t, err)
		assert.True(t, opts.incoming)
		assert.Equal(t, "json", opts.format)
		assert.Equal(t, []string{"docs/api.md"}, pos)
	})
	t.Run("unknown flag errors", func(t *testing.T) {
		_, _, err := parseDepsFlags([]string{"--nope"})
		assert.Error(t, err)
	})
	t.Run("help flag", func(t *testing.T) {
		_, _, err := parseDepsFlags([]string{"--help"})
		assert.Error(t, err)
	})
}

func TestRunDeps_Success(t *testing.T) {
	t.Run("outgoing", func(t *testing.T) {
		depsWorkspace(t)
		var code int
		out := captureStdout(func() { code = runDeps([]string{"a.md"}) })
		assert.Equal(t, 0, code)
		assert.Contains(t, out, "a.md:")
		assert.Contains(t, out, "file-link b.md")
	})
	t.Run("incoming", func(t *testing.T) {
		depsWorkspace(t)
		var code int
		out := captureStdout(func() { code = runDeps([]string{"b.md", "--incoming"}) })
		assert.Equal(t, 0, code)
		assert.Contains(t, out, "a.md:")
	})
	t.Run("no edges exits 1", func(t *testing.T) {
		depsWorkspace(t)
		var code int
		_ = captureStdout(func() { code = runDeps([]string{"b.md"}) })
		assert.Equal(t, 1, code)
	})
}

func TestRunDeps_ArgErrors(t *testing.T) {
	cases := map[string][]string{
		"no target":     nil,
		"too many args": {"a.md", "b.md"},
		"absolute":      {"/etc/passwd"},
		"bad flag":      {"--bogus"},
		"bad max-input": {"--max-input-size", "bogus", "a.md"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			depsWorkspace(t)
			assert.Equal(t, 2, runDeps(args))
		})
	}
	t.Run("help exits 0", func(t *testing.T) {
		depsWorkspace(t)
		assert.Equal(t, 0, runDeps([]string{"--help"}))
	})
	t.Run("unknown format exits 2", func(t *testing.T) {
		depsWorkspace(t)
		var code int
		_ = captureStdout(func() { code = runDeps([]string{"--format", "yaml", "a.md"}) })
		assert.Equal(t, 2, code)
	})
}

func TestRunDeps_Discovery(t *testing.T) {
	t.Run("missing config exits 2", func(t *testing.T) {
		depsWorkspace(t)
		assert.Equal(t, 2, runDeps([]string{"--config", "/no/such/.mdsmith.yml", "a.md"}))
	})
	t.Run("empty discovery exits 1", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
			[]byte("files:\n  - \"nope/*.md\"\n"), 0o644))
		t.Chdir(dir)
		var code int
		_ = captureStdout(func() { code = runDeps([]string{"a.md"}) })
		assert.Equal(t, 1, code)
	})
}
