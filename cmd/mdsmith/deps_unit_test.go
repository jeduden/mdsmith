package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
